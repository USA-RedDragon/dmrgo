//go:build tinygo

package enums_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	enumSeeds(func(v uint64, i int) { fuzzEnums(t, v, i) })
	fuzzEnums(t, 200, 0)
}
