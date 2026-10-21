//go:build tinygo

package lrrp_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	messageSeeds(func(data []byte) { fuzzDecodeMessage(t, data) })
}
