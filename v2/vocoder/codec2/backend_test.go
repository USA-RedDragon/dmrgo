//go:build !nocodec2 && !tinygo && !codec2native

package codec2

import "testing"

func TestDefaultBackendIsWASM(t *testing.T) {
	t.Parallel()

	if Backend != "wasm" {
		t.Errorf("Backend = %q without -tags codec2native, want \"wasm\"", Backend)
	}
}
