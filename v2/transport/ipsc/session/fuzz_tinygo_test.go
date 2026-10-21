//go:build tinygo

package session_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
)

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkSession(t, s)
	}
}

func tooLong(p []byte) bool {
	return len(p) > session.MaxPacket
}

func controlTooLong(p []byte) bool {
	return len(p) > session.MaxControl
}
