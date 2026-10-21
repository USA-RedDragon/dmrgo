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

func TestBroadcastPDU_AsCallTimer(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementCallTimer, BroadcastParms1: 510<<5 | 31, Backoff: 2, SysIdentCode: 0x4015, BroadcastParms2: 4095<<12 | 1}
	got, ok := b.AsCallTimer()
	want := pdu.CallTimerParms{AnnouncementType: enums.AnnouncementCallTimer, EmergencyTimer: 510, PacketTimer: 31, Backoff: 2, SysIdentCode: 0x4015, MSMSTimer: 4095, LineTimer: 1}
	if !ok || got != want {
		t.Errorf("AsCallTimer = %+v, %v, want %+v", got, ok, want)
	}
	checkBroadcastEncode(t, &b, must(got.Encode()))
}

func TestBroadcastPDU_AsVoteNow(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementVoteNow, BroadcastParms1: 0x2603, Reg: true, Backoff: 6, SysIdentCode: 0x9827, BroadcastParms2: 1<<23 | 2<<19 | 5<<16 | 0xFFF}
	got, ok := b.AsVoteNow()
	want := pdu.VoteNowParms{
		AnnouncementType: enums.AnnouncementVoteNow, VoteSysCode: 0x2603, Reg: true, Backoff: 6, SysIdentCode: 0x9827,
		HasActiveConn: true, ConfirmedPriority: 2, AdjacentPriority: 5, Channel: 0xFFF,
	}
	if !ok || got != want {
		t.Errorf("AsVoteNow = %+v, %v, want %+v", got, ok, want)
	}
	checkBroadcastEncode(t, &b, must(got.Encode()))
}

func TestBroadcastPDU_AsLocalTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p1   uint16
		p2   uint32
		want pdu.LocalTimeParms
	}{
		{"UTC-5 Wednesday", 21<<9 | 10<<5 | 5<<1 | 1, 11<<19 | 49<<13 | 50<<7 | 4<<4, pdu.LocalTimeParms{Day: 21, Month: 10, UTCOffset: 5, UTCOffsetLags: true, Hours: 11, Minutes: 49, Seconds: 50, DayOfWeek: 4}},
		{"UTC+5:30", 1<<9 | 1<<5 | 5<<1, 23<<19 | 59<<13 | 59<<7 | 1<<4 | 2<<2, pdu.LocalTimeParms{Day: 1, Month: 1, UTCOffset: 5, Hours: 23, Minutes: 59, Seconds: 59, DayOfWeek: 1, UTCOffsetQuarter: 2}},
		{"no date", 15 << 1, 0, pdu.LocalTimeParms{UTCOffset: 15}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementLocalTime, BroadcastParms1: tt.p1, Backoff: 1, SysIdentCode: 7, BroadcastParms2: tt.p2}
			got, ok := b.AsLocalTime()
			want := tt.want
			want.AnnouncementType, want.Backoff, want.SysIdentCode = enums.AnnouncementLocalTime, 1, 7
			if !ok || got != want {
				t.Errorf("AsLocalTime = %+v, %v, want %+v", got, ok, want)
			}
			checkBroadcastEncode(t, &b, must(got.Encode()))
		})
	}
}

func TestBroadcastPDU_AsMassReg(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementMassReg, BroadcastParms1: 4<<5 | 8, Reg: true, Backoff: 3, SysIdentCode: 0x1234, BroadcastParms2: 0x001234}
	got, ok := b.AsMassReg()
	want := pdu.MassRegParms{AnnouncementType: enums.AnnouncementMassReg, RegWindow: 4, AlohaMask: 8, Reg: true, Backoff: 3, SysIdentCode: 0x1234, Address: 0x001234}
	if !ok || got != want {
		t.Errorf("AsMassReg = %+v, %v, want %+v", got, ok, want)
	}
	checkBroadcastEncode(t, &b, must(got.Encode()))
}

func TestBroadcastPDU_AsGenSite(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementGenSiteParams, Backoff: 4, SysIdentCode: 0x0101, BroadcastParms2: 1<<22 | 1<<7}
	got, ok := b.AsGenSite()
	want := pdu.GenSiteParms{AnnouncementType: enums.AnnouncementGenSiteParams, Backoff: 4, SysIdentCode: 0x0101, Hibernating: true, RegWithoutTG: true}
	if !ok || got != want {
		t.Errorf("AsGenSite = %+v, %v, want %+v", got, ok, want)
	}
	checkBroadcastEncode(t, &b, must(got.Encode()))
	if _, ok := b.AsMassReg(); ok {
		t.Error("AsMassReg: ok for Gen_Site_Params")
	}
}

func checkBroadcastEncode(t *testing.T, b *pdu.BroadcastPDU, got [8]byte) {
	t.Helper()
	if want := must(b.Encode()); got != want {
		t.Errorf("parameters encode as %x, C_BCAST as %x", got, want)
	}
}

func FuzzBroadcastParms(f *testing.F) {
	broadcastSeeds(func(typ uint8, p1 uint16, reg bool, backoff uint8, sys uint16, p2 uint32) {
		f.Add(typ, p1, reg, backoff, sys, p2)
	})
	f.Fuzz(fuzzBroadcastParms)
}

func broadcastSeeds(add func(typ uint8, p1 uint16, reg bool, backoff uint8, sys uint16, p2 uint32)) {
	add(uint8(enums.AnnouncementLocalTime), 21<<9|10<<5|5<<1|1, true, 3, 0x4015, 11<<19|49<<13)
	add(uint8(enums.AnnouncementGenSiteParams), 0, false, 1, 1, 1<<22)
	add(uint8(enums.AnnouncementCallTimer), 0x3FFF, true, 15, 0xFFFF, 0xFFFFFF)
	add(uint8(enums.AnnouncementVoteNow), 0x2603, true, 6, 0x9827, 0xC0_01F9)
	add(uint8(enums.AnnouncementMassReg), 1<<14, false, 16, 0, 1<<24)
}

func fuzzBroadcastParms(t *testing.T, typ uint8, p1 uint16, reg bool, backoff uint8, sys uint16, p2 uint32) {
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementType(typ), BroadcastParms1: p1, Reg: reg, Backoff: backoff, SysIdentCode: sys, BroadcastParms2: p2}
	want, err := b.Encode()
	views := 0
	check := func(ok bool, enc func() ([8]byte, error)) {
		if !ok {
			return
		}
		views++
		if got, gerr := enc(); err != nil || gerr != nil || got != want {
			t.Fatalf("%+v: view encodes as %x (%v), want %x (%v)", b, got, gerr, want, err)
		}
	}
	v1, ok1 := b.AsCallTimer()
	check(ok1, v1.Encode)
	v2, ok2 := b.AsVoteNow()
	check(ok2, v2.Encode)
	v3, ok3 := b.AsLocalTime()
	check(ok3, v3.Encode)
	v4, ok4 := b.AsMassReg()
	check(ok4, v4.Encode)
	v5, ok5 := b.AsGenSite()
	check(ok5, v5.Encode)
	known := typ >= 1 && typ <= 7 && typ != 5 && typ != 6
	if views > 1 || err == nil && known != (views == 1) {
		t.Fatalf("%+v: %d views", b, views)
	}
}
