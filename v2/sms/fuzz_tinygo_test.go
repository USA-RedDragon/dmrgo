//go:build tinygo

package sms_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(defined bool, code uint8, data []byte, from, to uint8, repeat uint16) {
		fuzzDecode(t, defined, code, data, from, to, repeat)
	})
}
