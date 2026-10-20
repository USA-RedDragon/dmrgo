package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type specMatrix struct {
	IgnoreFiles []string   `yaml:"ignore_files"`
	Documents   []document `yaml:"documents"`
}

type document struct {
	ID       string    `yaml:"id"`
	Title    string    `yaml:"title"`
	Sections []section `yaml:"sections"`
}

type section struct {
	Section       string          `yaml:"section"`
	Title         string          `yaml:"title"`
	SourceFiles   []string        `yaml:"source_files"`
	TestFunctions []testFuncGroup `yaml:"test_functions"`
}

type testFuncGroup struct {
	Package string   `yaml:"package"`
	Names   []string `yaml:"names"`
}

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

const (
	statusVerified      = "verified"
	statusFailing       = "failing"
	statusUntested      = "untested"
	statusUnimplemented = "unimplemented"
)

type sectionKey struct {
	docID   string
	section string
}

type sectionVerification struct {
	total  int
	passed int
	failed []string
	notRun []string
}

type testResult struct {
	passed bool
	ran    bool
}

type packageRun struct {
	results    map[string]*testResult
	testFailed bool
	output     strings.Builder
}

func deriveStatus(sec section, sv *sectionVerification) string {
	hasSource := len(sec.SourceFiles) > 0
	hasTests := len(sec.TestFunctions) > 0

	if !hasSource && !hasTests {
		return statusUnimplemented
	}
	if !hasTests {
		return statusUntested
	}
	if sv != nil && sv.total > 0 && len(sv.failed) == 0 && len(sv.notRun) == 0 {
		return statusVerified
	}
	return statusFailing
}

func main() {
	os.Exit(run())
}

func run() int {
	updateReadme := flag.Bool("update-readme", false, "Inject Markdown coverage report into README.md between ETSI COVERAGE markers")
	checkReadme := flag.Bool("check-readme", false, "Fail if the coverage section in README.md is not up to date")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "error: cannot find the speccheck source directory")
		return 1
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "spec_matrix.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read spec_matrix.yaml: %v\n", err)
		return 1
	}

	var matrix specMatrix
	if err := yaml.Unmarshal(data, &matrix); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot parse spec_matrix.yaml: %v\n", err)
		return 1
	}

	errCount := checkListedFiles(repoRoot, matrix)
	pkgRuns, runErrs := runAllTests(ctx, filepath.Join(repoRoot, "v2"), collectPackageTests(matrix))
	errCount += runErrs

	sectionResults, verifyErrs := verifySections(matrix, pkgRuns)
	errCount += verifyErrs

	untracked, err := findUntracked(repoRoot, matrix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot scan source files: %v\n", err)
		errCount++
	}
	for _, f := range untracked {
		fmt.Fprintf(os.Stderr, "ERROR: file not tracked in spec_matrix.yaml (add it to a section's source_files or to ignore_files): %s\n", f)
		errCount++
	}

	md := generateMarkdown(matrix.Documents, sectionResults)
	readmePath := filepath.Join(repoRoot, "README.md")

	if *checkReadme {
		errCount += checkReadmeFresh(readmePath, md)
	}

	printSummary(matrix.Documents, sectionResults, errCount)

	if *updateReadme {
		if errCount > 0 {
			fmt.Fprintf(os.Stderr, "error: not updating README.md because of %d error(s)\n", errCount)
			return 1
		}
		if err := writeReadme(readmePath, md); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot update README.md: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Updated ETSI coverage section in %s\n", readmePath)
	}

	if errCount > 0 {
		return 1
	}
	return 0
}

func checkListedFiles(repoRoot string, matrix specMatrix) int {
	errCount := 0
	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			for _, f := range sec.SourceFiles {
				if _, err := os.Stat(filepath.Join(repoRoot, f)); errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(os.Stderr, "ERROR: [%s §%s] source file missing: %s\n", doc.ID, sec.Section, f)
					errCount++
				}
			}
		}
	}
	for _, f := range matrix.IgnoreFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, f)); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "ERROR: ignore_files entry missing: %s\n", f)
			errCount++
		}
	}
	return errCount
}

func collectPackageTests(matrix specMatrix) map[string]map[string]bool {
	packageTests := make(map[string]map[string]bool)
	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			for _, tfg := range sec.TestFunctions {
				names, ok := packageTests[tfg.Package]
				if !ok {
					names = make(map[string]bool)
					packageTests[tfg.Package] = names
				}
				for _, name := range tfg.Names {
					names[name] = true
				}
			}
		}
	}
	return packageTests
}

func runAllTests(ctx context.Context, dir string, packageTests map[string]map[string]bool) (map[string]*packageRun, int) {
	pkgs := make([]string, 0, len(packageTests))
	for pkg := range packageTests {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)

	errCount := 0
	pkgRuns := make(map[string]*packageRun, len(pkgs))
	for _, pkg := range pkgs {
		fmt.Fprintf(os.Stderr, "Running tests for %s ...\n", pkg)
		pr, err := runPackageTests(ctx, dir, pkg, packageTests[pkg])
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			errCount++
		}
		if pr != nil {
			pkgRuns[pkg] = pr
		}
	}
	return pkgRuns, errCount
}

func testRegex(names map[string]bool) string {
	nameList := make([]string, 0, len(names))
	for name := range names {
		nameList = append(nameList, name)
	}
	sort.Strings(nameList)
	return "^(" + strings.Join(nameList, "|") + ")$"
}

func runPackageTests(ctx context.Context, dir, pkg string, names map[string]bool) (*packageRun, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-v", "-json", "-count=1", "-run", testRegex(names), pkg)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe for %s: %w", pkg, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tests for %s: %w", pkg, err)
	}

	pr, scanErr := parseTestEvents(stdout, names)
	waitErr := cmd.Wait()

	if scanErr != nil {
		return pr, fmt.Errorf("reading go test output for %s: %w", pkg, scanErr)
	}
	if waitErr == nil {
		return pr, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) && pr.testFailed {
		return pr, nil
	}
	return pr, fmt.Errorf("go test %s failed without a test failure (build error?): %w\n%s%s",
		pkg, waitErr, pr.output.String(), stderr.String())
}

func parseTestEvents(r io.Reader, names map[string]bool) (*packageRun, error) {
	pr := &packageRun{results: make(map[string]*testResult, len(names))}
	for name := range names {
		pr.results[name] = &testResult{}
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var ev testEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			pr.output.Write(scanner.Bytes())
			pr.output.WriteByte('\n')
			continue
		}
		if ev.Test == "" {
			pr.output.WriteString(ev.Output)
			continue
		}
		if ev.Action == "fail" {
			pr.testFailed = true
		}

		baseName, _, _ := strings.Cut(ev.Test, "/")
		res, ok := pr.results[baseName]
		if !ok {
			continue
		}

		switch ev.Action {
		case "run":
			res.ran = true
		case "pass":
			if ev.Test == baseName {
				res.passed = true
			}
		case "fail":
			if ev.Test == baseName {
				res.passed = false
				res.ran = true
			}
		}
	}
	return pr, scanner.Err()
}

func verifySection(sec section, pkgRuns map[string]*packageRun) *sectionVerification {
	sv := &sectionVerification{}
	for _, tfg := range sec.TestFunctions {
		pr := pkgRuns[tfg.Package]
		for _, name := range tfg.Names {
			sv.total++
			var res *testResult
			if pr != nil {
				res = pr.results[name]
			}
			switch {
			case res == nil || !res.ran:
				sv.notRun = append(sv.notRun, tfg.Package+"."+name)
			case res.passed:
				sv.passed++
			default:
				sv.failed = append(sv.failed, tfg.Package+"."+name)
			}
		}
	}
	return sv
}

func verifySections(matrix specMatrix, pkgRuns map[string]*packageRun) (map[sectionKey]*sectionVerification, int) {
	errCount := 0
	sectionResults := make(map[sectionKey]*sectionVerification)
	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			sv := verifySection(sec, pkgRuns)
			sectionResults[sectionKey{doc.ID, sec.Section}] = sv
			if sv.total == 0 {
				continue
			}

			prefix := fmt.Sprintf("[%s §%s]", doc.ID, sec.Section)
			if len(sv.failed) > 0 {
				fmt.Fprintf(os.Stderr, "FAIL: %s %s - %d/%d passed, failed: %s\n",
					prefix, sec.Title, sv.passed, sv.total, strings.Join(sv.failed, ", "))
				errCount++
			}
			if len(sv.notRun) > 0 {
				fmt.Fprintf(os.Stderr, "FAIL: %s %s - tests not found/not run: %s\n",
					prefix, sec.Title, strings.Join(sv.notRun, ", "))
				errCount++
			}
			if len(sv.failed) == 0 && len(sv.notRun) == 0 {
				fmt.Fprintf(os.Stderr, "  PASS: %s %s (%d tests)\n", prefix, sec.Title, sv.passed)
			}
		}
	}
	return sectionResults, errCount
}

func findUntracked(repoRoot string, matrix specMatrix) ([]string, error) {
	known := make(map[string]bool)
	for _, f := range matrix.IgnoreFiles {
		known[filepath.Clean(f)] = true
	}
	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			for _, f := range sec.SourceFiles {
				known[filepath.Clean(f)] = true
			}
		}
	}

	var untracked []string
	err := filepath.WalkDir(filepath.Join(repoRoot, "v2"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") ||
			filepath.Base(path) == "doc.go" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		if !known[filepath.ToSlash(rel)] {
			untracked = append(untracked, filepath.ToSlash(rel))
		}
		return nil
	})
	return untracked, err
}

func countStatuses(doc document, results map[sectionKey]*sectionVerification) map[string]int {
	c := map[string]int{}
	for _, sec := range doc.Sections {
		c[deriveStatus(sec, results[sectionKey{doc.ID, sec.Section}])]++
	}
	return c
}

func countTotals(docs []document, results map[sectionKey]*sectionVerification) (map[string]int, int) {
	totals := map[string]int{}
	n := 0
	for _, doc := range docs {
		for status, count := range countStatuses(doc, results) {
			totals[status] += count
		}
		n += len(doc.Sections)
	}
	return totals, n
}

func printSummary(docs []document, results map[sectionKey]*sectionVerification, errCount int) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(os.Stderr, "  ETSI TS 102 361 - Specification Coverage")
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")

	for _, doc := range docs {
		c := countStatuses(doc, results)
		n := len(doc.Sections)
		fmt.Fprintf(os.Stderr, "\n  %s\n", doc.Title)
		fmt.Fprintf(os.Stderr, "    verified:      %3d / %d\n", c[statusVerified], n)
		fmt.Fprintf(os.Stderr, "    failing:       %3d / %d\n", c[statusFailing], n)
		fmt.Fprintf(os.Stderr, "    untested:      %3d / %d\n", c[statusUntested], n)
		fmt.Fprintf(os.Stderr, "    unimplemented: %3d / %d\n", c[statusUnimplemented], n)
	}

	totals, totalN := countTotals(docs, results)
	pct := 0.0
	if totalN > 0 {
		pct = float64(totals[statusVerified]) / float64(totalN) * 100
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "───────────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  Overall: %d/%d verified (%.1f%%)\n", totals[statusVerified], totalN, pct)
	fmt.Fprintf(os.Stderr, "           %d failing, %d untested, %d unimplemented\n",
		totals[statusFailing], totals[statusUntested], totals[statusUnimplemented])
	fmt.Fprintf(os.Stderr, "  Errors: %d\n", errCount)
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(os.Stderr)
}

func testResultCell(sv *sectionVerification) string {
	if sv == nil || sv.total == 0 {
		return "-"
	}
	if len(sv.failed) == 0 && len(sv.notRun) == 0 {
		return fmt.Sprintf("%d/%d passed", sv.passed, sv.total)
	}
	parts := []string{}
	if sv.passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", sv.passed))
	}
	if len(sv.failed) > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", len(sv.failed)))
	}
	if len(sv.notRun) > 0 {
		parts = append(parts, fmt.Sprintf("%d not run", len(sv.notRun)))
	}
	return strings.Join(parts, ", ")
}

func generateMarkdown(docs []document, results map[sectionKey]*sectionVerification) string {
	var b strings.Builder
	totals, totalN := countTotals(docs, results)

	b.WriteString("\n*Generated by `cmd/speccheck -update-readme`. The status of each section comes from its tests.*\n\n")

	b.WriteString("### Summary\n\n")
	b.WriteString("| Metric | Count |\n")
	b.WriteString("| --- | --- |\n")
	fmt.Fprintf(&b, "| Total sections | %d |\n", totalN)
	fmt.Fprintf(&b, "| Verified (tests pass) | %d |\n", totals[statusVerified])
	fmt.Fprintf(&b, "| Failing (tests fail) | %d |\n", totals[statusFailing])
	fmt.Fprintf(&b, "| Untested (has source, no tests) | %d |\n", totals[statusUntested])
	fmt.Fprintf(&b, "| Unimplemented | %d |\n", totals[statusUnimplemented])
	b.WriteString("\n")

	overallPct := 0.0
	if totalN > 0 {
		overallPct = float64(totals[statusVerified]) / float64(totalN) * 100
	}
	fmt.Fprintf(&b, "**Overall: %d/%d verified (%.1f%%)**\n\n", totals[statusVerified], totalN, overallPct)

	for _, doc := range docs {
		c := countStatuses(doc, results)
		n := len(doc.Sections)

		fmt.Fprintf(&b, "### %s\n\n", doc.Title)

		fmt.Fprintf(&b, "%d sections: %d verified, %d failing, %d untested, %d unimplemented.\n\n",
			n, c[statusVerified], c[statusFailing], c[statusUntested], c[statusUnimplemented])

		b.WriteString("| Section | Title | Status | Tests | Source Files |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")

		for _, sec := range doc.Sections {
			sv := results[sectionKey{doc.ID, sec.Section}]
			status := deriveStatus(sec, sv)

			srcFiles := "-"
			if len(sec.SourceFiles) > 0 {
				parts := make([]string, len(sec.SourceFiles))
				for i, f := range sec.SourceFiles {
					parts[i] = fmt.Sprintf("`%s`", f)
				}
				srcFiles = strings.Join(parts, ", ")
			}

			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
				sec.Section, sec.Title, status, testResultCell(sv), srcFiles)
		}

		b.WriteString("\n")
	}

	return b.String()
}

const (
	beginMarker = "<!-- BEGIN ETSI COVERAGE -->"
	endMarker   = "<!-- END ETSI COVERAGE -->"
)

func replaceCoverage(content, md string) (string, error) {
	beginIdx := strings.Index(content, beginMarker)
	if beginIdx == -1 {
		return "", fmt.Errorf("%s not found", beginMarker)
	}

	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 {
		return "", fmt.Errorf("%s not found", endMarker)
	}

	if endIdx < beginIdx {
		return "", errors.New("end marker appears before begin marker")
	}

	var out strings.Builder
	out.WriteString(content[:beginIdx+len(beginMarker)])
	out.WriteString("\n")
	out.WriteString(md)
	out.WriteString(content[endIdx:])
	return out.String(), nil
}

func writeReadme(readmePath, md string) error {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmePath, err)
	}
	out, err := replaceCoverage(string(data), md)
	if err != nil {
		return fmt.Errorf("%s: %w", readmePath, err)
	}
	return os.WriteFile(readmePath, []byte(out), 0o600)
}

func checkReadmeFresh(readmePath, md string) int {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read %s: %v\n", readmePath, err)
		return 1
	}
	want, err := replaceCoverage(string(data), md)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s: %v\n", readmePath, err)
		return 1
	}
	if want != string(data) {
		fmt.Fprintln(os.Stderr, "ERROR: README.md coverage section is out of date; run `go run -C cmd/speccheck . -update-readme` and commit the result")
		return 1
	}
	return 0
}
