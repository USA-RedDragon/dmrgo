package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateHookAllocatesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyTree(t, "testdata/repo", root)
	rc, log := docgen(t, "-update", "-root", root)
	if rc != 0 || strings.Contains(log, "FAIL") {
		t.Fatalf("-update: rc %d, log:\n%s", rc, log)
	}
}

func TestStateHookTestFailureFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyTree(t, "testdata/repo", root)
	if rc, log := docgen(t, "-update", "-root", root); rc != 0 {
		t.Fatalf("-update: rc %d, log:\n%s", rc, log)
	}
	f := filepath.Join(root, "v2/machine/machine_test.go")
	src, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	src = append(src, []byte("\nfunc TestBroken(t *testing.T) { t.Fatal(\"broken\") }\n")...)
	if err := os.WriteFile(f, src, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"-check", "-update"} {
		if rc, log := docgen(t, mode, "-root", root); rc != 1 || !strings.Contains(log, "TestBroken") {
			t.Errorf("%s with a failing test: rc %d, log:\n%s", mode, rc, log)
		}
	}
}
