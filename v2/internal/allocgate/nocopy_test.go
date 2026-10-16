//go:build !tinygo

package allocgate_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVetReportsCopies runs go vet on testdata/copies, which copies each
// library type that keeps pointers into its own storage, and fails unless
// the copylocks check reports each copy.
func TestVetReportsCopies(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "vet", "-copylocks", "./internal/allocgate/testdata/copies")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go vet reports no copy:\n%s", out)
	}
	for _, v := range []string{"burst", "assembler", "builder", "receiver"} {
		if !strings.Contains(string(out), "assignment copies lock value to "+v) {
			t.Errorf("go vet does not report the copy to %s:\n%s", v, out)
		}
	}
}
