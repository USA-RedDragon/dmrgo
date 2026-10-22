//go:build tinygo

package dmrdsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkSession(t, s)
	}
}

func tooLong(p []byte) bool {
	return len(p) > dmrdsession.MaxPacket
}
