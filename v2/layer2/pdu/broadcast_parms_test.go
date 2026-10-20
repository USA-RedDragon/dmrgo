package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var capMaxAdjacentSite = [12]byte{0xA8, 0x00, 0x34, 0xC0, 0x76, 0x98, 0x27, 0xC0, 0x01, 0xF9, 0x18, 0x46}

func TestBroadcastPDU_AsAdjacentSite_CapacityMax(t *testing.T) {
	t.Parallel()
	c := pdu.DecodeCSBK(capMaxAdjacentSite)
	if c.BroadcastPDU == nil {
		t.Fatalf("no C_BCAST payload: %v", c.Opcode)
	}
	got, ok := c.BroadcastPDU.AsAdjacentSite()
	if !ok {
		t.Fatal("AsAdjacentSite: ok false")
	}
	want := pdu.AdjacentSiteParms{
		AnnouncementType: enums.AnnouncementAdjacentSite,
		AdjacentSysCode:  0x2603,
		Reg:              true,
		Backoff:          6,
		SysIdentCode:     0x9827,
		HasActiveConn:    true,
		ActiveConn:       true,
		Channel:          505,
	}
	if got != want {
		t.Errorf("AsAdjacentSite = %+v, want %+v", got, want)
	}
	var sc pdu.SysCode
	if err := sc.Unpack14(got.AdjacentSysCode); err != nil || sc.Model != pdu.NetworkModelLarge || sc.Net != 6 || sc.Site != 3 {
		t.Errorf("adjacent SYScode %+v (%v), want Large net 6 site 3", sc, err)
	}
	if _, ok := c.BroadcastPDU.AsAnnWDTSCC(); ok {
		t.Error("AsAnnWDTSCC: ok for Adjacent_Site")
	}
	if _, ok := c.BroadcastPDU.AsChanFreq(); ok {
		t.Error("AsChanFreq: ok for Adjacent_Site")
	}
}

func TestBroadcastPDU_AsAnnWDTSCC(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementAnnWDTSCC, BroadcastParms1: 0b0000_0111_0100_01, Backoff: 3, SysIdentCode: 0x4015, BroadcastParms2: 17<<12 | 4095}
	got, ok := b.AsAnnWDTSCC()
	want := pdu.AnnWDTSCCParms{
		AnnouncementType: enums.AnnouncementAnnWDTSCC, ColorCode1: 7, ColorCode2: 4, Withdraw2: true,
		Backoff: 3, SysIdentCode: 0x4015, Channel1: 17, Channel2: 4095,
	}
	if !ok || got != want {
		t.Errorf("AsAnnWDTSCC = %+v, %v, want %+v", got, ok, want)
	}
	b.BroadcastParms1 = 1 << 14
	if _, ok := b.AsAnnWDTSCC(); ok {
		t.Error("AsAnnWDTSCC: ok for a BroadcastParms1 over 14 bits")
	}
}

func TestBroadcastPDU_AsChanFreq(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementChanFreq, Reg: true, Backoff: 1, SysIdentCode: 0x1234, BroadcastParms2: 300}
	got, ok := b.AsChanFreq()
	want := pdu.ChanFreqParms{AnnouncementType: enums.AnnouncementChanFreq, Reg: true, Backoff: 1, SysIdentCode: 0x1234, Channel: 300}
	if !ok || got != want {
		t.Errorf("AsChanFreq = %+v, %v, want %+v", got, ok, want)
	}
}
