package dmrdauth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
)

func TestAllocsAuth(t *testing.T) {
	b := dmrdauth.NewBridgeKey([]byte("passw0rd"))
	v1, v5 := packet("DMRD", 53, 73), packet("DMRE", 73, 89)
	n := testing.AllocsPerRun(100, func() {
		tok, _ := dmrdauth.Token(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"))
		_ = dmrdauth.VerifyToken(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"), tok[:])
		_ = b.SignInPlace(v1)
		_, _ = b.Verify(v1)
		_ = b.SignInPlace(v5)
		_, _ = b.Verify(v5)
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
