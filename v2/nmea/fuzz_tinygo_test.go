//go:build tinygo

package nmea_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	rmcSeeds(func(data []byte) { fuzzParseRMC(t, data) })
}
