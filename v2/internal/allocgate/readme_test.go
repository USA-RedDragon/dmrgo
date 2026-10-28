//go:build !tinygo

package allocgate_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	readmeBegin = "<!-- allocgate:begin -->"
	readmeEnd   = "<!-- allocgate:end -->"
)

func readmeTable(rules []*rule, cuts []*cut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n| What may allocate | When | Functions |\n| --- | --- | ---: |\n", readmeBegin)
	for _, r := range rules {
		fmt.Fprintf(&b, "| %s | %s | %d |\n", r.what, r.when, r.used)
	}
	for _, c := range cuts {
		fmt.Fprintf(&b, "| %s | %s | %d |\n", c.what, c.when, c.used)
	}
	fmt.Fprintf(&b, "\nThe counts are of exported functions and methods, for %s/%s with cgo. `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table.\n\n%s", runtime.GOOS, runtime.GOARCH, readmeEnd)
	return b.String()
}

func checkReadme(t *testing.T, rules []*rule, cuts []*cut) {
	t.Helper()
	mode := os.Getenv("DMRGO_README")
	if mode == "" {
		return
	}
	path := filepath.Join(moduleRoot, "..", "docs", "allocation.md")
	readme, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i, j := bytes.Index(readme, []byte(readmeBegin)), bytes.Index(readme, []byte(readmeEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", path, readmeBegin, readmeEnd)
	}
	want := readmeTable(rules, cuts)
	got := string(readme[i : j+len(readmeEnd)])
	switch mode {
	case "check":
		if got != want {
			t.Errorf("the allocation table of docs/allocation.md differs from the rules of the gate; run DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate in v2. Want:\n%s", want)
		}
	case "write":
		out := append(append(append([]byte{}, readme[:i]...), want...), readme[j+len(readmeEnd):]...)
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("DMRGO_README is %q, want check or write", mode)
	}
}
