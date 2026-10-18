//go:build tinygo

package enums_test

import "testing"

// TestFuzzSeeds runs FuzzEnums on its seeds and on the value of its
// regression input in testdata/fuzz, which TinyGo does not do because its
// F.Fuzz does not run the fuzz function.
func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	enumSeeds(func(v uint64, i int) { fuzzEnums(t, v, i) })
	fuzzEnums(t, 200, 0)
}
