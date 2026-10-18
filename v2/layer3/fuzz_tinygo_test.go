//go:build tinygo

package layer3_test

import "testing"

// TestFuzzSeeds runs FuzzLayer3 on its seeds, which TinyGo does not do
// because its F.Fuzz does not run the fuzz function.
func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	layer3Seeds(func(data []byte) { fuzzLayer3(t, data) })
}
