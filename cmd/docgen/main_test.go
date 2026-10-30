package main

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var golden = flag.Bool("golden", false, "rewrite testdata/want from the output of the fixture")

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func docgen(t *testing.T, args ...string) (int, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rc := run(args, f)
	b, _ := os.ReadFile(f.Name())
	return rc, string(b)
}

func TestFixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyTree(t, "testdata/repo", root)
	if rc, log := docgen(t, "-check", "-root", root); rc != 1 || !strings.Contains(log, "stale: docs/machines/machine-lamp.md:5: dmrgo:config") {
		t.Fatalf("-check on empty markers: rc %d, log:\n%s", rc, log)
	}
	if rc, log := docgen(t, "-update", "-root", root); rc != 0 {
		t.Fatalf("-update: rc %d, log:\n%s", rc, log)
	}
	for _, f := range []string{"machine-lamp.md", "states/machine-lamp.svg", "states/machine-counter.svg", "states/machine-pair-sidestate.svg"} {
		got, err := os.ReadFile(filepath.Join(root, "docs/machines", f))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join("testdata/want", f)
		if *golden {
			if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(want, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		w, err := os.ReadFile(want)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, w) {
			t.Errorf("%s differs from %s:\n%s", f, want, got)
		}
	}
	if rc, log := docgen(t, "-check", "-root", root); rc != 0 {
		t.Fatalf("-check after -update: rc %d, log:\n%s", rc, log)
	}

	page := filepath.Join(root, "docs/machines/machine-lamp.md")
	text, _ := os.ReadFile(page)
	edited := strings.Replace(string(text), "| LampOff | `Start` | LampOn |", "| LampOff | `Start` | LampBroken |", 1)
	if err := os.WriteFile(page, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, log := docgen(t, "-check", "-root", root); rc != 1 || !strings.Contains(log, "machine-lamp.md:15: dmrgo:states pkg=machine type=Lamp") {
		t.Fatalf("-check on an edited body: rc %d, log:\n%s", rc, log)
	}
	if err := os.WriteFile(page, text, 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(root, "v2/machine/machine.go")
	code, _ := os.ReadFile(src)
	moved := strings.Replace(string(code), "// Stop turns", "// Lamps stop.\n\n// Stop turns", 1)
	if err := os.WriteFile(src, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, log := docgen(t, "-check", "-root", root); rc != 1 || !strings.Contains(log, "range drifted: the excerpt in the page is now at lines=98-104") {
		t.Fatalf("-check after the code moved: rc %d, log:\n%s", rc, log)
	}
}

func TestMarkerErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ text, want string }{
		{"<!-- dmrgo:nope a=b -->\n<!-- /dmrgo:nope -->", "unknown marker kind"},
		{"<!-- dmrgo:config pkg=x -->\n<!-- /dmrgo:config -->", "needs type="},
		{"<!-- dmrgo:output example=a test=b.C -->\n<!-- /dmrgo:output -->", "exactly one of"},
		{"<!-- dmrgo:code file=a lines=1-2 -->\n", "has no <!-- /dmrgo:code -->"},
		{"<!-- dmrgo:const pkg=x name=Y extra=1 --><!-- /dmrgo:const -->", "does not take extra="},
	} {
		if _, err := parseMarkers(tc.text); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.text, err, tc.want)
		}
	}
}

func TestMarkerInFence(t *testing.T) {
	t.Parallel()
	text := "a <!-- dmrgo:gomod field=go --><!-- /dmrgo:gomod -->\n\n```text\n<!-- dmrgo:config pkg=a type=B -->\n<!-- /dmrgo:config -->\n```\n\n<!-- dmrgo:const pkg=a name=C --><!-- /dmrgo:const -->\n"
	ms, err := parseMarkers(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].kind != kindGoMod || ms[1].kind != kindConst {
		t.Fatalf("got %d markers, want only the two outside the fence", len(ms))
	}
}

func TestCodeRange(t *testing.T) {
	t.Parallel()
	src := []byte("package p\n\nfunc f() {\n\tx := 1\n\t_ = x\n}\n")
	if err := checkGoRange("p.go", src, 3, 6); err != nil {
		t.Errorf("whole func: %v", err)
	}
	if err := checkGoRange("p.go", src, 4, 5); err != nil {
		t.Errorf("statements: %v", err)
	}
	if err := checkGoRange("p.go", src, 2, 6); err == nil {
		t.Error("a range from a blank line passed")
	}
	if err := checkGoRange("p.go", src, 3, 5); err == nil {
		t.Error("a range that ends inside the func passed")
	}
}

func TestYAMLSymbol(t *testing.T) {
	t.Parallel()
	src := []byte("on: push\njobs:\n  a:\n    steps:\n      - run: x\n\n  # b runs y\n  b:\n    steps:\n      - run: y\n")
	got, err := yamlExcerpt(src, "jobs.a")
	if err != nil {
		t.Fatal(err)
	}
	if want := "a:\n  steps:\n    - run: x"; got != want {
		t.Errorf("jobs.a = %q, want %q", got, want)
	}
	got, err = yamlExcerpt(src, "jobs.b.steps")
	if err != nil {
		t.Fatal(err)
	}
	if want := "steps:\n  - run: y"; got != want {
		t.Errorf("jobs.b.steps = %q, want %q", got, want)
	}
	if _, err := yamlExcerpt(src, "jobs.c"); err == nil {
		t.Error("a missing key passed")
	}
	if _, err := yamlExcerpt(src, "steps"); err == nil {
		t.Error("a key below the top level passed as a top-level key")
	}
}
