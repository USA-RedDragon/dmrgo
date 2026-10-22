package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	preTarget = 9
	preSource = 3120000
)

func dataPreambles(t testing.TB, n int) []layer2.RawBurst {
	t.Helper()
	out := make([]layer2.RawBurst, 0, n)
	for i := range n {
		c := pdu.CSBK{LastBlock: true}
		p := pdu.PreamblePDU{Data: true, Group: true, CSBKBlocksToFollow: byte(n - i - 1 + 3), TargetAddress: preTarget, SourceAddress: preSource}
		if err := c.SetPreamble(p); err != nil {
			t.Fatal(err)
		}
		out = append(out, must(layer2.BuildBurst(&c, 1, layer2.SyncSourceMS)))
	}
	return out
}

func unconfirmedData(t testing.TB) []layer2.RawBurst {
	t.Helper()
	var h pdu.DataHeader
	if err := h.SetUnconfirmed(pdu.UnconfirmedDataHeader{Group: true, SAP: 4, LLIDDestination: preTarget, LLIDSource: preSource, FullMessageFlag: true}); err != nil {
		t.Fatal(err)
	}
	p, err := packet.NewBuilder(16).Segment(h, make([]byte, 20), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]layer2.RawBurst, 0, 1+len(p.Blocks))
	out = append(out, must(layer2.BuildBurst(&p.Header, 1, layer2.SyncSourceMS)))
	for _, blk := range p.Blocks {
		out = append(out, must(layer2.BuildBurst(blk, 1, layer2.SyncSourceMS)))
	}
	return out
}

func checkPreambleCACH(t *testing.T, got []bs.Output, from, to time.Duration, busy bool) {
	t.Helper()
	var asm layer2.ShortLCAssembler
	hash, _ := pdu.AddressHash(preTarget)
	updates := 0
	for _, o := range filter(got, bs.OutputBurst) {
		if !o.HasCACH {
			t.Fatalf("burst at %v without CACH", o.Due)
		}
		cach := layer2.DecodeCACHField(o.CACH)
		if o.Due < from || o.Due >= to {
			asm.Reset()
			continue
		}
		if o.Slot == enums.Slot2 && cach.TACT.AccessType != busy {
			t.Errorf("AT %v before the TS2 burst at %v, want %v", cach.TACT.AccessType, o.Due, busy)
		}
		slc, done, err := asm.Add(cach)
		if err != nil || !done {
			continue
		}
		au, ok := slc.AsActivityUpdate()
		switch {
		case !busy && ok:
			t.Errorf("Activity Update %v at %v, want Null", au, o.Due)
		case busy && (!ok || au.TS1ActivityID != enums.ActivityGroupData || au.HashTS1 != hash || au.TS2ActivityID != enums.ActivityNoActivity):
			t.Errorf("Short LC %v at %v, want the TS1 group data Activity Update", slc, o.Due)
		}
		updates++
	}
	if updates == 0 {
		t.Errorf("no Short LC in [%v, %v)", from, to)
	}
}

// ETSI TS 102 361-1 clause 6.3, MMDVMHost DMRSlot.cpp:582
func TestPreambleBusy(t *testing.T) {
	t.Parallel()
	pre := dataPreambles(t, 6)
	in := timed(append(pre, unconfirmedData(t)...), enums.Slot1, rf, 0)
	header := time.Duration(len(pre)) * 60 * time.Millisecond
	net := bs.Source{Peer: 7}
	in = merge(in, call(t, 1, 5, 3191868, 0, enums.Slot1, net, 100*time.Millisecond)[:1])
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
	var mid bs.SlotState
	end := header + time.Second
	got := run(t, r, in[:3], 130*time.Millisecond)
	mid = r.SlotState(enums.Slot1)
	got = append(got, run(t, r, in[3:], end)...)
	if mid != bs.SlotPreamble {
		t.Errorf("slot state %v during the preambles, want %v", mid, bs.SlotPreamble)
	}
	checkPreambleCACH(t, got, 0, header, true)
	rej := filter(got, bs.OutputRejected)
	if len(rej) != 1 || rej[0].From != net || rej[0].Reason != bs.ReasonSlotBusy {
		t.Errorf("rejected %v, want the network call refused as SlotBusy", rej)
	}
	starts, ends := filter(got, bs.OutputCallStart), filter(got, bs.OutputCallEnd)
	if len(starts) != 1 || starts[0].Service != bs.ServiceData || starts[0].From != rf || len(ends) != 1 || ends[0].End != 0 {
		t.Errorf("starts %v ends %v, want one data call", starts, ends)
	}
}

func TestPreambleTimeout(t *testing.T) {
	t.Parallel()
	pre := dataPreambles(t, 2)
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, Lost: 300 * time.Millisecond})
	got := run(t, r, timed(pre, enums.Slot1, rf, 0), 900*time.Millisecond)
	if s := r.SlotState(enums.Slot1); s != bs.SlotIdle {
		t.Errorf("slot state %v after the timeout, want Idle", s)
	}
	checkPreambleCACH(t, got, 0, 120*time.Millisecond, true)
	checkPreambleCACH(t, got, 480*time.Millisecond, 900*time.Millisecond, false)
	if n := len(filter(got, bs.OutputCallStart)) + len(filter(got, bs.OutputCallEnd)); n != 0 {
		t.Errorf("%d call outputs for preambles alone", n)
	}
}

func TestPreambleCSBK(t *testing.T) {
	t.Parallel()
	d := newDriver(t, &bs.Config{ColorCode: 1, CACH: true})
	d.in(0, enums.Slot1, rf, preCSBK(t, preSource))
	d.in(60*time.Millisecond, enums.Slot1, rf, preCSBK(t, preSource))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotPreamble {
		t.Fatalf("slot state %v during the preambles, want %v", st, bs.SlotPreamble)
	}
	if st := d.r.State(); st != bs.StateRepeatingSlot1 {
		t.Errorf("state %v during the preambles, want %v", st, bs.StateRepeatingSlot1)
	}
	d.until(400 * time.Millisecond)
	var asm layer2.ShortLCAssembler
	found := false
	for _, o := range filter(d.all, bs.OutputBurst) {
		if slc, done, err := asm.Add(layer2.DecodeCACHField(o.CACH)); err == nil && done {
			au, ok := slc.AsActivityUpdate()
			found = found || ok && au.TS1ActivityID == enums.ActivityGroupCSBK
		}
	}
	if !found {
		t.Error("no group CSBK Activity Update during the CSBK preambles")
	}
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKUnitToUnitVoiceServiceRequest, RawPayload: [8]byte{0, 0, 0, 0, preTarget, 0x2F, 0x9B, 0x80}}
	d.in(420*time.Millisecond, enums.Slot1, rf, must(layer2.BuildBurst(&c, 1, layer2.SyncSourceMS)))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotIdle {
		t.Errorf("slot state %v after the CSBK, want Idle", st)
	}
}
