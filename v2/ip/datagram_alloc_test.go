//go:build !tinygo

package ip_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

func TestAllocs(t *testing.T) {
	data := unhex(t, appcapture.TMS)
	gps := unhex(t, appcapture.GPS)
	buf := make([]byte, 0, 256)
	n := testing.AllocsPerRun(50, func() {
		d, err := ip.Decode(enums.SAPIPBasedPacketData, data)
		if err != nil {
			panic(err)
		}
		if _, err := ip.AppendIPv4(buf[:0], &d); err != nil {
			panic(err)
		}
		c, err := ip.Decode(enums.SAPUDPIPHeaderCompression, gps)
		if err != nil {
			panic(err)
		}
		if _, err := ip.AppendCompressed(buf[:0], &c); err != nil {
			panic(err)
		}
		e := ip.ICMPPacket{Message: ip.ICMPHostUnreachable, Original: data}
		out, err := ip.AppendICMPPacket(buf[:0], &e)
		if err != nil {
			panic(err)
		}
		if _, err := ip.DecodeICMPPacket(out); err != nil {
			panic(err)
		}
		ip.CheckIPv4(data, 576)
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
