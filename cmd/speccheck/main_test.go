package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseTestEvents(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`{"Action":"start","Package":"p"}`,
		`{"Action":"run","Package":"p","Test":"TestPass"}`,
		`{"Action":"run","Package":"p","Test":"TestPass/sub"}`,
		`{"Action":"pass","Package":"p","Test":"TestPass/sub"}`,
		`{"Action":"pass","Package":"p","Test":"TestPass"}`,
		`{"Action":"run","Package":"p","Test":"TestFail"}`,
		`{"Action":"run","Package":"p","Test":"TestFail/sub"}`,
		`{"Action":"fail","Package":"p","Test":"TestFail/sub"}`,
		`{"Action":"fail","Package":"p","Test":"TestFail"}`,
		`{"Action":"output","Package":"p","Output":"FAIL\n"}`,
		`not json`,
	}, "\n")
	names := map[string]bool{"TestPass": true, "TestFail": true, "TestMissing": true}

	pr, err := parseTestEvents(strings.NewReader(input), names)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]testResult{
		"TestPass":    {passed: true, ran: true},
		"TestFail":    {passed: false, ran: true},
		"TestMissing": {},
	}
	for name, w := range want {
		if got := *pr.results[name]; got != w {
			t.Errorf("%s: got %+v, want %+v", name, got, w)
		}
	}
	if !pr.testFailed {
		t.Error("testFailed = false, want true")
	}
	if out := pr.output.String(); !strings.Contains(out, "FAIL\n") || !strings.Contains(out, "not json") {
		t.Errorf("output = %q, want package output and non-JSON lines", out)
	}
}

func TestParseTestEvents_SubtestFailMarksFailed(t *testing.T) {
	t.Parallel()

	input := `{"Action":"fail","Package":"p","Test":"TestOther/sub"}`
	pr, err := parseTestEvents(strings.NewReader(input), map[string]bool{"TestX": true})
	if err != nil {
		t.Fatal(err)
	}
	if !pr.testFailed {
		t.Error("testFailed = false, want true")
	}
}

func TestVerifySection(t *testing.T) {
	t.Parallel()

	pkgRuns := map[string]*packageRun{
		"a": {results: map[string]*testResult{
			"TestPass":   {passed: true, ran: true},
			"TestFail":   {ran: true},
			"TestNotRun": {},
		}},
	}
	sec := section{TestFunctions: []testFuncGroup{
		{Package: "a", Names: []string{"TestPass", "TestFail", "TestNotRun", "TestUnknown"}},
		{Package: "missing", Names: []string{"TestB"}},
	}}

	sv := verifySection(sec, pkgRuns)
	want := &sectionVerification{
		total:  5,
		passed: 1,
		failed: []string{"a.TestFail"},
		notRun: []string{"a.TestNotRun", "a.TestUnknown", "missing.TestB"},
	}
	if !reflect.DeepEqual(sv, want) {
		t.Errorf("got %+v, want %+v", sv, want)
	}
	if got := deriveStatus(section{SourceFiles: []string{"x"}, TestFunctions: sec.TestFunctions}, sv); got != statusFailing {
		t.Errorf("deriveStatus = %s, want %s", got, statusFailing)
	}
}

func TestDeriveStatus(t *testing.T) {
	t.Parallel()

	tests := []testFuncGroup{{Package: "a", Names: []string{"T"}}}
	cases := []struct {
		name string
		sec  section
		sv   *sectionVerification
		want string
	}{
		{"unimplemented", section{}, nil, statusUnimplemented},
		{"untested", section{SourceFiles: []string{"x"}}, nil, statusUntested},
		{"verified", section{TestFunctions: tests}, &sectionVerification{total: 1, passed: 1}, statusVerified},
		{"not run", section{TestFunctions: tests}, &sectionVerification{total: 1, notRun: []string{"a.T"}}, statusFailing},
		{"no results", section{TestFunctions: tests}, nil, statusFailing},
	}
	for _, c := range cases {
		if got := deriveStatus(c.sec, c.sv); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFindUntracked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := []string{
		"v2/a/listed.go",
		"v2/a/ignored.go",
		"v2/a/untracked.go",
		"v2/a/untracked_test.go",
		"v2/a/tables_gen.go",
		"v2/a/notes.txt",
	}
	for _, f := range files {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	matrix := specMatrix{
		IgnoreFiles: []string{"v2/a/ignored.go"},
		Documents: []document{{Sections: []section{
			{SourceFiles: []string{"v2/a/listed.go"}},
		}}},
	}

	got, err := findUntracked(root, matrix)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v2/a/untracked.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReplaceCoverage(t *testing.T) {
	t.Parallel()

	content := "head\n" + beginMarker + "\nold\n" + endMarker + "\ntail\n"
	got, err := replaceCoverage(content, "new\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "head\n" + beginMarker + "\nnew\n" + endMarker + "\ntail\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	again, err := replaceCoverage(got, "new\n")
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Error("replaceCoverage is not idempotent")
	}

	for _, bad := range []string{"no markers", beginMarker, endMarker, endMarker + beginMarker} {
		if _, err := replaceCoverage(bad, "x"); err == nil {
			t.Errorf("replaceCoverage(%q) succeeded, want error", bad)
		}
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/m\n\ngo 1.21\n"
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunPackageTests(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
	const pkgSrc = "package m\n"
	names := map[string]bool{"TestPass": true, "TestFail": true}

	t.Run("test failure", func(t *testing.T) {
		t.Parallel()

		dir := writeModule(t, map[string]string{
			"m.go": pkgSrc,
			"m_test.go": `package m

import "testing"

func TestPass(t *testing.T) {}
func TestFail(t *testing.T) { t.Fatal("boom") }
`,
		})
		pr, err := runPackageTests(context.Background(), dir, ".", names)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r := pr.results["TestPass"]; !r.ran || !r.passed {
			t.Errorf("TestPass = %+v, want ran and passed", *r)
		}
		if r := pr.results["TestFail"]; !r.ran || r.passed {
			t.Errorf("TestFail = %+v, want ran and failed", *r)
		}
	})

	t.Run("build failure", func(t *testing.T) {
		t.Parallel()

		dir := writeModule(t, map[string]string{
			"m.go": pkgSrc,
			"m_test.go": `package m

import "testing"

func TestPass(t *testing.T) { undefinedFunc() }
`,
		})
		pr, err := runPackageTests(context.Background(), dir, ".", names)
		if err == nil {
			t.Fatal("expected error for build failure")
		}
		if !strings.Contains(err.Error(), "undefinedFunc") {
			t.Errorf("error does not include build output: %v", err)
		}
		if pr == nil || pr.results["TestPass"].ran {
			t.Error("TestPass should not have run")
		}
	})
}

func TestGenerateMarkdown(t *testing.T) {
	t.Parallel()

	docs := []document{{
		ID:    "D",
		Title: "ETSI TS 102 361-1: Air Interface Protocol",
		Sections: []section{
			{Section: "6.1", Title: "Vocoder socket", SourceFiles: []string{"v2/a.go"}, TestFunctions: []testFuncGroup{{Package: "./a", Names: []string{"TestA"}}}},
			{Section: "6.2", Title: "Untested", SourceFiles: []string{"v2/b.go"}},
		},
	}}
	results := map[sectionKey]*sectionVerification{
		{"D", "6.1"}: {total: 1, passed: 1},
	}
	got := generateMarkdown(docs, results)
	for _, want := range []string{
		"| Verified (tests pass) | 1 |\n",
		"| Untested (has source, no tests) | 1 |\n",
		"**Overall: 1/2 verified (50.0%)**\n",
		"### ETSI TS 102 361-1: Air Interface Protocol\n\n2 sections: 1 verified, 0 failing, 1 untested, 0 unimplemented.\n",
		"| 6.1 | Vocoder socket | verified | 1/1 passed | `v2/a.go` |\n",
		"| 6.2 | Untested | untested | - | `v2/b.go` |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown does not contain %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"\u00a7", "\u2014", "\u2705", "\u2588"} {
		if strings.Contains(got, bad) {
			t.Errorf("markdown contains %q", bad)
		}
	}
}
