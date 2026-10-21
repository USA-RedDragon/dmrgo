//go:build tinygo

package location_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(sap uint8, data []byte) { fuzzDecode(t, sap, data) })
}
