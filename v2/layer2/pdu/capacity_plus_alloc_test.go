//go:build !tinygo

package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestAllocsCapacityPlus(t *testing.T) {
	raw := [12]byte{0xBE, 0x10, 0xC2, 0x80, 0x1E, 0, 0, 0, 0, 0, 0xCC, 0xD6}
	var a pdu.CapacityPlusSiteStatusAssembler
	var sink uint16
	n := testing.AllocsPerRun(100, func() {
		c := pdu.DecodeCSBK(raw)
		p, _ := c.AsCapacityPlusSiteStatus()
		_, _ = a.Add(&p)
		act, _ := a.Activity()
		nb, _ := c.AsCapacityPlusNeighbors()
		_, r, _ := nb.Neighbor(0)
		pr, _ := c.AsCapacityPlusPreamble()
		s := pdu.ShortLC{SLCO: pdu.SLCOCapacityPlusRestChannel, RawPayload: [3]byte{0x10, 0x06, 0x18}}
		rc, _ := s.AsCapacityPlusRestChannel()
		lc := pdu.FullLinkControl{FLCO: pdu.FLCOCapacityPlusGroupVoiceGrant, FID: enums.FIDMotorola}
		g, _ := lc.AsCapacityPlusGroupVoiceGrant()
		sink += act.Voice + uint16(r) + pr.SourceAddress + uint16(rc.RestLSN) + g.SourceAddress
	})
	if n != 0 {
		t.Errorf("%v allocations", n)
	}
	_ = sink
}
