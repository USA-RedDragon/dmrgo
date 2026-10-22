//go:build tinygo

package ipscsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	pool := fuzzPool()
	for _, s := range fuzzSeeds() {
		checkSession(t, pool, s)
	}
}

func tooLong(p []byte) bool {
	return len(p) > ipscsession.MaxPacket
}

func controlTooLong(p []byte) bool {
	return len(p) > ipscsession.MaxControl
}
