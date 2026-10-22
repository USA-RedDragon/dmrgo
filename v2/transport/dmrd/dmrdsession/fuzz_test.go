//go:build !tinygo

package dmrdsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func FuzzSession(f *testing.F) {
	pool := fuzzPool()
	for _, s := range fuzzSeeds() {
		f.Add(s.ops, s.raw, s.from, s.to, s.reps)
	}
	f.Fuzz(func(t *testing.T, ops, raw []byte, from, to, reps uint8) {
		checkSession(t, pool, fuzzInput{ops: ops, raw: raw, from: from, to: to, reps: reps})
	})
}

func tooLong(p []byte) bool {
	return len(p) > dmrdsession.MaxPacket
}
