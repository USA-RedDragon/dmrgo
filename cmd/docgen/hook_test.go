package main

import (
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
