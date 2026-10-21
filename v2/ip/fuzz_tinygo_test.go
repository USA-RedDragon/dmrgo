//go:build tinygo

package ip_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	decodeSeeds(func(sap uint8, data []byte) { fuzzDecode(t, sap, data) })
}

func TestFuzzICMPSeeds(t *testing.T) {
	t.Parallel()

	icmpSeeds(func(data []byte) { fuzzDecodeICMPPacket(t, data) })
}
