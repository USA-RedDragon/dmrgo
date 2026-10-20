//go:build tinygo

package layer3_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	layer3Seeds(func(data []byte) { fuzzLayer3(t, data) })
}
