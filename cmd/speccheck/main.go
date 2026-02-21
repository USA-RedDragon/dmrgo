// Command speccheck validates spec_matrix.yaml against the codebase by
// running the Go tests listed for each spec section.
//
// Status is derived from test execution:
//   - verified:      has tests and all pass
//   - failing:       has tests but some fail or were not found
//   - untested:      has source files but no tests listed
//   - unimplemented: no source files and no tests
//
// Checks:
//  1. Every source_file listed actually exists on disk.
//  2. For each section with test_functions, run `go test -v -run <regex>`
//     and verify all named tests pass.
//  3. Prints a per-document summary and overall completion percentage.
//  4. Exits non-zero on any test failure.
//
// Flags:
//
//	-update-readme   Inject the Markdown coverage report into README.md
//	                 between <!-- BEGIN ETSI COVERAGE --> and <!-- END ETSI COVERAGE --> markers.
//
// Usage:
//
//	go run -C cmd/speccheck .
//	go run -C cmd/speccheck . --update-readme
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── YAML schema ──────────────────────────────────────────────────────────────

type specMatrix struct {
	Documents []document `yaml:"documents"`
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

// testEvent is the JSON output from `go test -json`.
type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// Derived status values (not from YAML - computed from test execution).
const (
	statusVerified      = "verified"      // has tests, all pass
	statusFailing       = "failing"       // has tests, some fail or not found
	statusUntested      = "untested"      // has source files but no tests
	statusUnimplemented = "unimplemented" // no source files, no tests
)

// sectionKey uniquely identifies a section within a document.
type sectionKey struct {
	docID   string
	section string
}

// sectionVerification holds the test execution results for one spec section.
type sectionVerification struct {
	total  int
	passed int
	failed []string
	notRun []string
}

// deriveStatus computes the status of a section from its YAML shape and test results.
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
	updateReadme := flag.Bool("update-readme", false, "Inject Markdown coverage report into README.md between ETSI COVERAGE markers")
	flag.Parse()

	// Resolve repo root relative to this source file.
	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "spec_matrix.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read spec_matrix.yaml: %v\n", err)
		os.Exit(1)
	}

	var matrix specMatrix
	if err := yaml.Unmarshal(data, &matrix); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot parse spec_matrix.yaml: %v\n", err)
		os.Exit(1)
	}

	var errors, warnings int

	// ── Phase 1: validate schema & source files ──────────────────────────

	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			prefix := fmt.Sprintf("[%s §%s]", doc.ID, sec.Section)

			for _, f := range sec.SourceFiles {
				if _, err := os.Stat(filepath.Join(repoRoot, f)); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "ERROR: %s source file missing: %s\n", prefix, f)
					errors++
				}
			}
		}
	}

	// ── Phase 2: collect all tests to run per package ────────────────────

	// Map package → set of test names we need
	type sectionRef struct {
		docID   string
		section string
	}

	type testReq struct {
		names    map[string]bool
		sections []sectionRef
	}

	packageTests := make(map[string]*testReq)

	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			for _, tfg := range sec.TestFunctions {
				req, ok := packageTests[tfg.Package]
				if !ok {
					req = &testReq{names: make(map[string]bool)}
					packageTests[tfg.Package] = req
				}
				for _, name := range tfg.Names {
					req.names[name] = true
				}
				req.sections = append(req.sections, sectionRef{doc.ID, sec.Section})
			}
		}
	}

	// ── Phase 3: run tests per package ───────────────────────────────────

	// Results: package → test name → pass/fail
	type testResult struct {
		passed bool
		ran    bool
	}
	pkgResults := make(map[string]map[string]*testResult)

	for pkg, req := range packageTests {
		// Build a regex that matches exactly these test names
		nameList := make([]string, 0, len(req.names))
		for name := range req.names {
			nameList = append(nameList, name)
		}
		regex := "^(" + strings.Join(nameList, "|") + ")$"

		fmt.Fprintf(os.Stderr, "Running tests for %s ...\n", pkg)
		cmd := exec.Command("go", "test", "-v", "-json", "-count=1", "-run", regex, pkg)
		cmd.Dir = filepath.Join(repoRoot, "v2")
		cmd.Env = os.Environ()

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to create pipe for %s: %v\n", pkg, err)
			errors++
			continue
		}

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to start tests for %s: %v\n", pkg, err)
			errors++
			continue
		}

		results := make(map[string]*testResult)
		// Pre-populate expected tests
		for name := range req.names {
			results[name] = &testResult{}
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var ev testEvent
			if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Test == "" {
				continue // package-level event
			}

			// Handle sub-tests: "TestFoo/bar" → base is "TestFoo"
			baseName := ev.Test
			if idx := strings.Index(ev.Test, "/"); idx >= 0 {
				baseName = ev.Test[:idx]
			}

			res, ok := results[baseName]
			if !ok {
				continue // not one of ours
			}

			switch ev.Action {
			case "run":
				res.ran = true
			case "pass":
				if ev.Test == baseName { // only count top-level pass
					res.passed = true
				}
			case "fail":
				if ev.Test == baseName {
					res.passed = false
					res.ran = true
				}
			}
		}

		_ = cmd.Wait()
		pkgResults[pkg] = results
	}

	// ── Phase 4: check section-level results ─────────────────────────────

	sectionResults := make(map[sectionKey]*sectionVerification)

	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			key := sectionKey{doc.ID, sec.Section}
			sv := &sectionVerification{}

			if len(sec.TestFunctions) == 0 {
				sectionResults[key] = sv
				continue
			}

			prefix := fmt.Sprintf("[%s §%s]", doc.ID, sec.Section)

			for _, tfg := range sec.TestFunctions {
				results, ok := pkgResults[tfg.Package]
				if !ok {
					for _, name := range tfg.Names {
						sv.total++
						sv.notRun = append(sv.notRun, tfg.Package+"."+name)
					}
					continue
				}

				for _, name := range tfg.Names {
					sv.total++
					res, ok := results[name]
					if !ok || !res.ran {
						sv.notRun = append(sv.notRun, tfg.Package+"."+name)
						continue
					}
					if res.passed {
						sv.passed++
					} else {
						sv.failed = append(sv.failed, tfg.Package+"."+name)
					}
				}
			}

			sectionResults[key] = sv

			if len(sv.failed) > 0 {
				fmt.Fprintf(os.Stderr, "FAIL: %s %s - %d/%d passed, failed: %s\n",
					prefix, sec.Title, sv.passed, sv.total, strings.Join(sv.failed, ", "))
				errors++
			}
			if len(sv.notRun) > 0 {
				fmt.Fprintf(os.Stderr, "WARN: %s %s - tests not found/not run: %s\n",
					prefix, sec.Title, strings.Join(sv.notRun, ", "))
				warnings++
			}
			if len(sv.failed) == 0 && len(sv.notRun) == 0 {
				fmt.Fprintf(os.Stderr, "  PASS: %s %s (%d tests)\n", prefix, sec.Title, sv.passed)
			}
		}
	}

	// ── Phase 5: warn about untracked source files ───────────────────────

	referenced := make(map[string]bool)
	for _, doc := range matrix.Documents {
		for _, sec := range doc.Sections {
			for _, f := range sec.SourceFiles {
				referenced[f] = true
			}
		}
	}

	codeRoot := filepath.Join(repoRoot, "v2")
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if !referenced[rel] {
			fmt.Fprintf(os.Stderr, "WARN: file not tracked in spec_matrix.yaml: %s\n", rel)
			warnings++
		}
		return nil
	})

	// ── Summary ──────────────────────────────────────────────────────────

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(os.Stderr, "  ETSI TS 102 361 - Specification Coverage")
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")

	var totalN, totalVerified, totalFailing, totalUntested, totalUnimp int

	for _, doc := range matrix.Documents {
		c := map[string]int{}
		for _, sec := range doc.Sections {
			key := sectionKey{doc.ID, sec.Section}
			status := deriveStatus(sec, sectionResults[key])
			c[status]++
		}
		n := len(doc.Sections)
		totalN += n
		totalVerified += c[statusVerified]
		totalFailing += c[statusFailing]
		totalUntested += c[statusUntested]
		totalUnimp += c[statusUnimplemented]

		fmt.Fprintf(os.Stderr, "\n  %s\n", doc.Title)
		fmt.Fprintf(os.Stderr, "    verified:      %3d / %d\n", c[statusVerified], n)
		fmt.Fprintf(os.Stderr, "    failing:       %3d / %d\n", c[statusFailing], n)
		fmt.Fprintf(os.Stderr, "    untested:      %3d / %d\n", c[statusUntested], n)
		fmt.Fprintf(os.Stderr, "    unimplemented: %3d / %d\n", c[statusUnimplemented], n)
	}

	pct := 0.0
	if totalN > 0 {
		pct = float64(totalVerified) / float64(totalN) * 100
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "───────────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr, "  Overall: %d/%d verified (%.1f%%)\n", totalVerified, totalN, pct)
	fmt.Fprintf(os.Stderr, "           %d failing, %d untested, %d unimplemented\n", totalFailing, totalUntested, totalUnimp)
	fmt.Fprintf(os.Stderr, "  Errors: %d   Warnings: %d\n", errors, warnings)
	fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(os.Stderr)

	// ── Markdown report ──────────────────────────────────────────────────

	if *updateReadme {
		md := generateMarkdown(matrix.Documents, sectionResults, totalVerified, totalFailing, totalUntested, totalUnimp, totalN)
		readmePath := filepath.Join(repoRoot, "README.md")
		if err := injectMarkdownIntoReadme(readmePath, md); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot update README.md: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Updated ETSI coverage section in %s\n", readmePath)
	}

	if errors > 0 {
		os.Exit(1)
	}
}

// statusEmoji returns a visual indicator for each derived status.
func statusEmoji(status string) string {
	switch status {
	case statusVerified:
		return "✅"
	case statusFailing:
		return "❌"
	case statusUntested:
		return "🔶"
	case statusUnimplemented:
		return "⬜"
	default:
		return "❓"
	}
}

// progressBar renders a text-based progress bar: [████████░░░░░░░░] 50%
func progressBar(filled, total, width int) string {
	if total == 0 {
		return "[" + strings.Repeat("░", width) + "] 0.0%"
	}
	pct := float64(filled) / float64(total)
	full := int(pct * float64(width))
	if full > width {
		full = width
	}
	return fmt.Sprintf("[%s%s] %.1f%%", strings.Repeat("█", full), strings.Repeat("░", width-full), pct*100)
}

// testResultCell formats the test verification column for a section.
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

func generateMarkdown(docs []document, results map[sectionKey]*sectionVerification, totalVerified, totalFailing, totalUntested, totalUnimp, totalN int) string {
	var b strings.Builder

	b.WriteString("\n*Auto-generated by `cmd/speccheck --update-readme`. Status derived from test execution.*\n\n")

	// ── Overall summary ──
	b.WriteString("### Summary\n\n")
	b.WriteString("| Metric | Count |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Total sections | %d |\n", totalN))
	b.WriteString(fmt.Sprintf("| ✅ Verified (tests pass) | %d |\n", totalVerified))
	b.WriteString(fmt.Sprintf("| ❌ Failing (tests fail) | %d |\n", totalFailing))
	b.WriteString(fmt.Sprintf("| 🔶 Untested (has source, no tests) | %d |\n", totalUntested))
	b.WriteString(fmt.Sprintf("| ⬜ Unimplemented | %d |\n", totalUnimp))
	b.WriteString("\n")

	overallPct := 0.0
	if totalN > 0 {
		overallPct = float64(totalVerified) / float64(totalN) * 100
	}
	b.WriteString(fmt.Sprintf("**Overall: %d/%d verified (%.1f%%)**\n\n", totalVerified, totalN, overallPct))

	// ── Per-document tables ──
	for _, doc := range docs {
		c := map[string]int{}
		for _, sec := range doc.Sections {
			key := sectionKey{doc.ID, sec.Section}
			status := deriveStatus(sec, results[key])
			c[status]++
		}
		n := len(doc.Sections)

		b.WriteString(fmt.Sprintf("### %s\n\n", doc.Title))

		// Mini stats line with progress bar
		b.WriteString(fmt.Sprintf("**%d** sections - ", n))
		b.WriteString(fmt.Sprintf("✅ %d  ", c[statusVerified]))
		b.WriteString(fmt.Sprintf("❌ %d  ", c[statusFailing]))
		b.WriteString(fmt.Sprintf("🔶 %d  ", c[statusUntested]))
		b.WriteString(fmt.Sprintf("⬜ %d\n\n", c[statusUnimplemented]))

		b.WriteString(fmt.Sprintf("`%s`\n\n", progressBar(c[statusVerified], n, 30)))

		// Section table
		b.WriteString("| Section | Title | Status | Tests | Source Files |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")

		for _, sec := range doc.Sections {
			key := sectionKey{doc.ID, sec.Section}
			sv := results[key]
			status := deriveStatus(sec, sv)
			emoji := statusEmoji(status)
			testsCol := testResultCell(sv)

			// Source files as inline code list
			var srcFiles string
			if len(sec.SourceFiles) == 0 {
				srcFiles = "-"
			} else {
				parts := make([]string, len(sec.SourceFiles))
				for i, f := range sec.SourceFiles {
					parts[i] = fmt.Sprintf("`%s`", f)
				}
				srcFiles = strings.Join(parts, ", ")
			}

			b.WriteString(fmt.Sprintf("| §%s | %s | %s %s | %s | %s |\n",
				sec.Section, sec.Title, emoji, status, testsCol, srcFiles))
		}

		b.WriteString("\n")
	}

	return b.String()
}

const (
	beginMarker = "<!-- BEGIN ETSI COVERAGE -->"
	endMarker   = "<!-- END ETSI COVERAGE -->"
)

// injectMarkdownIntoReadme replaces content between the ETSI COVERAGE
// sentinel comments in README.md with the generated markdown.
func injectMarkdownIntoReadme(readmePath, md string) error {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmePath, err)
	}

	content := string(data)

	beginIdx := strings.Index(content, beginMarker)
	if beginIdx == -1 {
		return fmt.Errorf("%s not found in %s", beginMarker, readmePath)
	}

	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 {
		return fmt.Errorf("%s not found in %s", endMarker, readmePath)
	}

	if endIdx < beginIdx {
		return fmt.Errorf("end marker appears before begin marker in %s", readmePath)
	}

	var out strings.Builder
	out.WriteString(content[:beginIdx+len(beginMarker)])
	out.WriteString("\n")
	out.WriteString(md)
	out.WriteString(content[endIdx:])

	return os.WriteFile(readmePath, []byte(out.String()), 0o600)
}
