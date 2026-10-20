//go:build tinygo

package trellis

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range trellisSeeds(t) {
		checkTrellis(t, s.data, s.mask)
	}
}
