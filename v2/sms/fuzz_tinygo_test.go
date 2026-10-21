//go:build tinygo

package sms_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(defined bool, code uint8, data []byte) { fuzzDecode(t, defined, code, data) })
}
