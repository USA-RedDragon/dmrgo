//go:build !tinygo

package ars_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
)

func TestAllocs(t *testing.T) {
	data := unhex(t, registration)
	buf := make([]byte, 0, 64)
	n := testing.AllocsPerRun(50, func() {
		m, err := ars.DecodeMessage(data)
		if err != nil {
			panic(err)
		}
		id, _ := m.DeviceID()
		r := ars.NewRegistration(ars.EventRefresh)
		b, err := r.SetDeviceID(buf[:0], id)
		if err != nil {
			panic(err)
		}
		if _, err := r.AppendBinary(b[len(b):]); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
