//go:build tinygo

package hytera_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(data []byte) { fuzzDecode(t, data) })
}
