package auth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
)

func TestAllocsAuth(t *testing.T) {
	key := auth.NewKey([20]byte{1})
	buf := make([]byte, 0, 128)
	n := testing.AllocsPerRun(100, func() {
		buf = append(buf[:0], 0x96, 0, 0, 0, 1)
		buf = key.Sign(buf)
		_, _ = key.Verify(buf)
		_, _ = auth.ParseKey("1234abcd")
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
