//go:build !tinygo

package location_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

func TestAllocs(t *testing.T) {
	ms := aprsMessages(t)
	l := lrrpPacket(t, "071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006")
	h := hdapPacket(t, hdap.ServiceLP, hdap.LPConditionReport, append([]byte{0, 0, 0, 9, 10, 0, 0, 80}, hdapGPS...))
	d := new(location.Decoder)
	n := testing.AllocsPerRun(50, func() {
		if _, err := d.Decode(&ms[0].Header, ms[0].Data); err != nil {
			panic(err)
		}
		if _, err := d.Decode(&l.Header, l.Data); err != nil {
			panic(err)
		}
		if _, err := d.Decode(&h.Header, h.Data); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
