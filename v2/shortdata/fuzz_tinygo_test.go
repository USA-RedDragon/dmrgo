//go:build tinygo

package shortdata_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	textSeeds(func(format uint8, data []byte) { fuzzText(t, format, data) })
}
