//go:build !tinygo

package lrrp_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

func TestAllocs(t *testing.T) {
	data := unhex(t, specReport)
	m := new(lrrp.Message)
	buf := make([]byte, 0, 64)
	n := testing.AllocsPerRun(50, func() {
		if err := lrrp.DecodeMessageInto(m, data); err != nil {
			panic(err)
		}
		if _, err := m.AppendBinary(buf[:0]); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
