package auth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

func TestAllocs(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() { _ = auth.Response(0x123456, testKey, 0) }); n != 0 {
		t.Errorf("%.1f allocations, want 0", n)
	}
}
