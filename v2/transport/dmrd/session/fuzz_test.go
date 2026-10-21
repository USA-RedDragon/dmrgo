//go:build !tinygo

package session_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func FuzzSession(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	f.Fuzz(checkSession)
}

func tooLong(p []byte) bool {
	return len(p) > session.MaxPacket
}
