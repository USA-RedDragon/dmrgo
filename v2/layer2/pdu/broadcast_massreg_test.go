package pdu_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBroadcastPDU_AsMassReg_Table7_76(t *testing.T) {
	t.Parallel()
	b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementMassReg, BroadcastParms1: 0b00000_1010_11000, Reg: true, Backoff: 3, SysIdentCode: 0x4015, BroadcastParms2: 3120001}
	got, ok := b.AsMassReg()
	want := pdu.MassRegParms{AnnouncementType: enums.AnnouncementMassReg, RegWindow: 10, AlohaMask: 24, Reg: true, Backoff: 3, SysIdentCode: 0x4015, Address: 3120001}
	if !ok || got != want {
		t.Errorf("AsMassReg = %+v, %v, want %+v", got, ok, want)
	}
	enc, err := pdu.EncodePayload(&got)
	if err != nil {
		t.Fatal(err)
	}
	cs := pdu.CSBK{LastBlock: true}
	if err := cs.SetBroadcast(b); err != nil {
		t.Fatal(err)
	}
	c, err := cs.Encode()
	if err != nil || [8]byte(c[2:10]) != enc {
		t.Errorf("MassRegParms encodes as %x, the C_BCAST as %x %v", enc, c[2:10], err)
	}
	b.AnnouncementType = enums.AnnouncementChanFreq
	if _, ok := b.AsMassReg(); ok {
		t.Error("AsMassReg: ok for Chan_Freq")
	}
}

func TestRegWindowTime_Table6_12(t *testing.T) {
	t.Parallel()
	want := []time.Duration{0, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second,
		100 * time.Second, 300 * time.Second, 1000 * time.Second, 3000 * time.Second, 10000 * time.Second, 30000 * time.Second, 100000 * time.Second, 200000 * time.Second}
	for w := range uint8(17) {
		d, ok := pdu.RegWindowTime(w)
		switch {
		case w == 0 || w > 15:
			if ok {
				t.Errorf("Reg_Window %d: %v, ok", w, d)
			}
		case !ok || d != want[w]:
			t.Errorf("Reg_Window %d: %v %v, want %v", w, d, ok, want[w])
		}
	}
}

func FuzzBroadcastPDU_AsMassReg(f *testing.F) {
	f.Add(uint16(0b00000_1010_11000), uint32(3120001), true)
	f.Fuzz(func(t *testing.T, p1 uint16, p2 uint32, reg bool) {
		b := pdu.BroadcastPDU{AnnouncementType: enums.AnnouncementMassReg, BroadcastParms1: p1, Reg: reg, Backoff: 3, SysIdentCode: 0x4015, BroadcastParms2: p2}
		v, ok := b.AsMassReg()
		if ok != (p1 < 1<<14 && p2 < 1<<24) {
			t.Fatalf("parms %x %x: ok %v", p1, p2, ok)
		}
		if !ok {
			return
		}
		_ = v.String()
		if v.RegWindow != uint8(p1>>5&0xF) || v.AlohaMask != uint8(p1&0x1F) || v.Address != p2 || v.Reserved != uint8(p1>>9) {
			t.Fatalf("parms %x %x: %+v", p1, p2, v)
		}
	})
}
