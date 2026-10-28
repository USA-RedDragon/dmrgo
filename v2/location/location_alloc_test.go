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

func TestAllocsAppendPacket(t *testing.T) {
	ms := aprsMessages(t)
	d := new(location.Decoder)
	f, err := d.Decode(&ms[0].Header, ms[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	h := new(location.Header)
	buf := make([]byte, 0, 256)
	var out []byte
	n := testing.AllocsPerRun(50, func() {
		out, err = location.AppendPacket(buf[:0], &f, h)
	})
	if n != 0 || err != nil || string(out) != string(ms[0].Data) {
		t.Errorf("%.1f allocations per run, %q, %v, want 0 and %q", n, out, err, ms[0].Data)
	}
}
