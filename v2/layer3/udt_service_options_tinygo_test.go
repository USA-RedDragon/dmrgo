//go:build tinygo

package layer3_test

import "testing"

func TestUDTOptionsFuzzSeeds(t *testing.T) {
	t.Parallel()
	udtOptionsSeeds(func(b byte) { fuzzUDTOptions(t, b) })
}
