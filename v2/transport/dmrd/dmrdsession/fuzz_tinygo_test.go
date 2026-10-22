//go:build tinygo

package dmrdsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	pool := fuzzPool()
	for _, s := range fuzzSeeds() {
		checkSession(t, pool, s)
	}
}

func tooLong(p []byte) bool {
	return len(p) > dmrdsession.MaxPacket
}
