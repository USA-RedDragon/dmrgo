//go:build tinygo

package dvsi_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range parseSeeds(t) {
		checkParse(t, s)
	}
}
