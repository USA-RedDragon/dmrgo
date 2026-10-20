package auth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

func TestAllocsAuth(t *testing.T) {
	b := auth.NewBridge([]byte("passw0rd"))
	v1, v5 := packet("DMRD", 53, 73), packet("DMRE", 73, 89)
	n := testing.AllocsPerRun(100, func() {
		_, _ = auth.Token(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"))
		_ = b.Sign(v1)
		_ = b.Verify(v1)
		_ = b.Sign(v5)
		_ = b.Verify(v5)
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
