package trunkauth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/trunkauth"
)

func TestAllocs(t *testing.T) {
	if n := allocstest.PerRun(t, 100, func() { _ = trunkauth.Response(0x123456, testKey, 0) }); n != 0 {
		t.Errorf("%.1f allocations, want 0", n)
	}
}
