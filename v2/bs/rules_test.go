package bs_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

var rf = bs.Source{RF: true}

func TestWakeupActivation(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, Activation: bs.ActivationWakeup})
	in := call(t, 1, 9, 3191868, 1, enums.Slot1, rf, 0)[:3]
	in = append(in, input{at: 200 * time.Millisecond, slot: enums.Slot1, from: rf, raw: wakeup(t, 2)})
	got := run(t, r, in, 300*time.Millisecond)
	if len(got) != 0 || r.State() != bs.StateHibernating {
		t.Fatalf("before the wakeup: %d outputs, state %v", len(got), r.State())
	}
	in = []input{{at: 400 * time.Millisecond, slot: enums.Slot1, from: rf, raw: wakeup(t, 1)}}
	got = run(t, r, in, 6*time.Second)
	if len(filter(got, bs.OutputKey)) != 1 || len(filter(got, bs.OutputDekey)) != 1 {
		t.Fatalf("Key %d, Dekey %d, want 1 and 1", len(filter(got, bs.OutputKey)), len(filter(got, bs.OutputDekey)))
	}
	bursts := filter(got, bs.OutputBurst)
	for _, o := range bursts {
		if dt := o.Burst.SlotType().DataType; dt != enums.DataTypeIdle {
			t.Fatalf("outbound %v, want only Idle bursts", dt)
		}
	}
	dekey := filter(got, bs.OutputDekey)[0]
	if want := int((5 * time.Second) / bs.SlotTime); len(bursts) < want-1 || len(bursts) > want+1 {
		t.Errorf("%d Idle bursts before hibernation, want about %d", len(bursts), want)
	}
	_ = dekey
	if r.State() != bs.StateHibernating {
		t.Errorf("state %v, want Hibernating", r.State())
	}
}

func TestColorCode(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
	got := run(t, r, call(t, 2, 9, 3191868, 1, enums.Slot1, rf, 0), time.Second)
	if len(filter(got, bs.OutputCallStart)) != 0 {
		t.Fatal("RF call with CC 2 started")
	}
	got = run(t, r, call(t, 2, 9, 3191868, 1, enums.Slot1, bs.Source{Peer: 7}, 2*time.Second), 3*time.Second)
	if len(filter(got, bs.OutputCallStart)) != 1 {
		t.Fatal("network call with CC 2 did not start")
	}
	for _, o := range filter(got, bs.OutputBurst) {
		if o.To&bs.ToNetwork != 0 {
			t.Errorf("network call sent back to the network")
		}
		if p, _ := o.Burst.Sync(); p == enums.SyncBSSourcedData && o.Burst.SlotType().ColorCode != 1 {
			t.Errorf("outbound %v with CC %d", o.Burst.SlotType().DataType, o.Burst.SlotType().ColorCode)
		}
	}
}

func TestRFToNetwork(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
	got := run(t, r, call(t, 1, 9, 3191868, 2, enums.Slot1, rf, 0), 2*time.Second)
	var net, hang int
	for _, o := range filter(got, bs.OutputBurst) {
		dt := o.Burst.SlotType().DataType
		p, _ := o.Burst.Sync()
		data := p == enums.SyncBSSourcedData
		switch {
		case o.To&bs.ToNetwork != 0:
			net++
			if data && dt == enums.DataTypeIdle {
				t.Error("Idle burst sent to the network")
			}
		case o.Slot == enums.Slot1 && data && dt == enums.DataTypeTerminatorWithLC:
			hang++
		}
	}
	if net != 1+12+1 || hang == 0 {
		t.Errorf("%d bursts to the network, want 14; %d hang time terminators", net, hang)
	}
}

func TestSlotOwner(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	a := call(t, 1, 9, 3191868, 2, enums.Slot1, bs.Source{Peer: 1}, 0)
	b := call(t, 1, 91, 3120000, 12, enums.Slot1, bs.Source{Peer: 2}, 10*time.Millisecond)
	got := run(t, r, merge(a, b), 6*time.Second)
	starts := filter(got, bs.OutputCallStart)
	if len(starts) != 2 || starts[0].From.Peer != 1 || starts[1].From.Peer != 2 {
		t.Fatalf("call starts %v", starts)
	}
	if starts[1].Call.Origin != voice.OriginEmbedded {
		t.Errorf("second call origin %v, want late entry", starts[1].Call.Origin)
	}
	rej := filter(got, bs.OutputRejected)
	if len(rej) != 1 || rej[0].From.Peer != 2 || rej[0].Reason != bs.ReasonSlotBusy {
		t.Fatalf("rejections %v", rej)
	}
	end := filter(got, bs.OutputCallEnd)[0]
	for _, o := range filter(got, bs.OutputBurst) {
		if o.From.Peer == 2 && o.Due < end.Due+3*time.Second {
			t.Fatalf("peer 2 burst forwarded in the hang time of peer 1")
		}
		if o.To != bs.ToNetwork || o.HasCACH {
			t.Fatalf("network burst To %v HasCACH %v", o.To, o.HasCACH)
		}
	}
}

func TestHangTimeSameTarget(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	a := call(t, 1, 9, 3191868, 1, enums.Slot2, bs.Source{Peer: 1}, 0)
	b := call(t, 1, 9, 3120000, 1, enums.Slot2, bs.Source{Peer: 2}, time.Second)
	c := call(t, 1, 92, 3120001, 1, enums.Slot2, bs.Source{Peer: 3}, 3*time.Second)
	got := run(t, r, merge(merge(a, b), c), 5*time.Second)
	starts := filter(got, bs.OutputCallStart)
	if len(starts) != 2 || starts[1].From.Peer != 2 {
		t.Fatalf("call starts %v", starts)
	}
	rej := filter(got, bs.OutputRejected)
	if len(rej) != 1 || rej[0].From.Peer != 3 || rej[0].Reason != bs.ReasonHangTime {
		t.Fatalf("rejections %v", rej)
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, Timeout: time.Second})
	got := run(t, r, call(t, 1, 9, 3191868, 6, enums.Slot1, rf, 0), 3*time.Second)
	rej := filter(got, bs.OutputRejected)
	if len(rej) != 1 || rej[0].Reason != bs.ReasonTimeout {
		t.Fatalf("rejections %v", rej)
	}
	var voiceAfter, terms int
	for _, o := range filter(got, bs.OutputBurst) {
		if o.Slot != enums.Slot1 {
			continue
		}
		p, _ := o.Burst.Sync()
		switch {
		case p == enums.SyncBSSourcedData && o.Burst.SlotType().DataType == enums.DataTypeTerminatorWithLC:
			terms++
		case p != enums.SyncBSSourcedData && terms > 0:
			voiceAfter++
		}
	}
	if terms != 1 || voiceAfter != 0 {
		t.Errorf("%d terminators, %d voice bursts after the timeout", terms, voiceAfter)
	}
	if len(filter(got, bs.OutputCallEnd)) != 1 || r.SlotState(enums.Slot1) != bs.SlotIdle {
		t.Errorf("call end %d, slot state %v", len(filter(got, bs.OutputCallEnd)), r.SlotState(enums.Slot1))
	}
}

func TestReject(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	in := call(t, 1, 9, 3191868, 1, enums.Slot1, bs.Source{Peer: 4}, 0)
	out := r.Inbound(0, &in[0].raw, layer2.Hint{Slot: enums.Slot1}, in[0].from, nil)
	if len(filter(out, bs.OutputCallStart)) != 1 {
		t.Fatalf("%v", out)
	}
	if err := r.Reject(enums.Slot1); err != nil {
		t.Fatal(err)
	}
	got := run(t, r, in[1:], 2*time.Second)
	if n := len(filter(got, bs.OutputBurst)); n != 0 {
		t.Errorf("%d bursts of a rejected call", n)
	}
	if len(filter(got, bs.OutputCallEnd)) != 1 {
		t.Error("no call end")
	}
	if !errors.Is(r.Reject(enums.SlotUnknown), bs.ErrSlot) {
		t.Error("Reject of SlotUnknown")
	}
	var zero bs.Repeater
	if !errors.Is(zero.Reject(enums.Slot1), dmrgo.ErrZeroValue) {
		t.Error("Reject on a zero Repeater")
	}
}

func TestDataSMS(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		bursts := loadBin(t, name)
		r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
		got := run(t, r, timed(bursts, enums.Slot2, rf, 0), 2*time.Second)
		var rep []layer2.RawBurst
		for _, o := range filter(got, bs.OutputBurst) {
			if o.Slot == enums.Slot2 && o.Burst.SlotType().DataType != enums.DataTypeIdle {
				rep = append(rep, o.Burst)
				if o.To != bs.ToRF|bs.ToNetwork {
					t.Errorf("%s: To %v", name, o.To)
				}
			}
		}
		if len(rep) != len(bursts) {
			t.Fatalf("%s: %d bursts repeated, want %d", name, len(rep), len(bursts))
		}
		for i := range rep {
			if rep[i] != bursts[i] {
				t.Errorf("%s: burst %d differs", name, i)
			}
		}
		starts, ends := filter(got, bs.OutputCallStart), filter(got, bs.OutputCallEnd)
		if len(starts) != 1 || starts[0].Service != bs.ServiceData || len(ends) != 1 || ends[0].End != 0 {
			t.Errorf("%s: starts %v ends %v", name, starts, ends)
		}
	}
}

func TestConfirmedDataHang(t *testing.T) {
	t.Parallel()
	b := packet.NewBuilder(16)
	var h pdu.DataHeader
	if err := h.SetConfirmed(pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: 4, LLIDDestination: 3191868, LLIDSource: 3120000, FullMessageFlag: true, SendSequenceNumber: 5}); err != nil {
		t.Fatal(err)
	}
	p, err := b.Segment(h, make([]byte, 30), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	bursts := make([]layer2.RawBurst, 0, 1+len(p.Blocks))
	bursts = append(bursts, must(layer2.BuildBurst(&p.Header, 1, layer2.SyncSourceMS)))
	for _, blk := range p.Blocks {
		bursts = append(bursts, must(layer2.BuildBurst(blk, 1, layer2.SyncSourceMS)))
	}
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, DataHang: 300 * time.Millisecond})
	got := run(t, r, timed(bursts, enums.Slot1, rf, 0), time.Second)
	var tdlc int
	idleAfter := false
	for _, o := range filter(got, bs.OutputBurst) {
		if o.Slot != enums.Slot1 {
			continue
		}
		dt := o.Burst.SlotType().DataType
		if dt == enums.DataTypeIdle {
			idleAfter = idleAfter || tdlc > 0
		}
		if dt == enums.DataTypeTerminatorWithLC {
			cw, _ := o.Burst.BPTCPayload()
			lc, _, err := pdu.DecodeFullLCCodeword(cw, enums.DataTypeTerminatorWithLC)
			l := pdu.LCBytes(lc)
			if err != nil || l.FLCO() != enums.FLCOTerminatorDataLinkControl || lc[2] != 0x30 || lc[8]&0xC7 != 0x45 {
				t.Fatalf("TD_LC %X", lc)
			}
			tdlc++
		}
	}
	if tdlc < 4 || tdlc > 6 || !idleAfter {
		t.Errorf("%d TD_LC bursts, Idle after %v", tdlc, idleAfter)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestNew(t *testing.T) {
	t.Parallel()
	for _, cfg := range []bs.Config{{ColorCode: 16}, {Mode: 2}, {Activation: 2}, {Pending: bs.MaxPending + 1},
		{Headers: 9}, {Terminators: 65}, {DataQueue: bs.MaxDataQueue + 1}, {Lead: -1}} {
		var ce *dmrgo.ConfigError
		if _, err := bs.New(&cfg); !errors.As(err, &ce) {
			t.Errorf("New(%+v) = %v, want a *dmrgo.ConfigError", cfg, err)
		}
	}
	r := newRepeater(t, nil)
	var raw layer2.RawBurst
	if out := r.Inbound(0, &raw, layer2.Hint{}, rf, nil); len(out) != 0 {
		t.Errorf("Inbound without a slot: %+v", out)
	}
	if out := r.Inbound(0, nil, layer2.Hint{Slot: enums.Slot1}, rf, nil); len(out) != 0 {
		t.Errorf("Inbound of a nil burst: %+v", out)
	}
	if out := r.Lost(0, 3, rf, nil); len(out) != 0 {
		t.Errorf("Lost of slot 3: %+v", out)
	}
	for _, s := range []enums.Slot{enums.SlotUnknown, 3} {
		if err := bs.CheckSlot(s); !errors.Is(err, bs.ErrSlot) {
			t.Errorf("CheckSlot(%d) = %v", s, err)
		}
	}
	for _, s := range []enums.Slot{enums.Slot1, enums.Slot2} {
		if err := bs.CheckSlot(s); err != nil {
			t.Errorf("CheckSlot(%v) = %v", s, err)
		}
	}
}

func TestSuperseded(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	a := call(t, 1, 9, 3191868, 1, enums.Slot1, bs.Source{Peer: 1}, 0)
	a = a[:len(a)-1]
	b := call(t, 1, 91, 3191868, 1, enums.Slot1, bs.Source{Peer: 1}, a[len(a)-1].at+60*time.Millisecond)
	got := run(t, r, append(a, b...), 2*time.Second)
	starts, ends := filter(got, bs.OutputCallStart), filter(got, bs.OutputCallEnd)
	if len(starts) != 2 || starts[1].Call.Target != 91 || len(ends) != 2 || ends[0].End != voice.EndSuperseded {
		t.Fatalf("starts %v ends %v", starts, ends)
	}
	if len(filter(got, bs.OutputRejected)) != 0 {
		t.Error("owner refused")
	}
}

func TestDecodedMatchesInbound(t *testing.T) {
	t.Parallel()
	inputs := map[string][]input{
		"voice": call(t, 1, 9, 3191868, 2, enums.Slot1, rf, 0),
		"net":   call(t, 1, 9, 3191868, 2, enums.Slot2, bs.Source{Peer: 4}, 0),
	}
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		inputs[name] = timed(loadBin(t, name), enums.Slot2, rf, 0)
	}
	for name, in := range inputs {
		for _, mode := range []bs.Mode{bs.ModeRF, bs.ModeNetwork} {
			cfg := &bs.Config{Mode: mode, ColorCode: 1, CACH: true}
			want := run(t, newRepeater(t, cfg), in, 2*time.Second)
			got := runWith(t, newRepeater(t, cfg), in, 2*time.Second, layer2.NewBurst())
			if len(want) < 10 {
				t.Fatalf("%s mode %d: only %d outputs", name, mode, len(want))
			}
			if len(got) != len(want) {
				t.Fatalf("%s mode %d: %d outputs, want %d", name, mode, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("%s mode %d: output %d is %+v, want %+v", name, mode, i, got[i], want[i])
				}
			}
		}
	}
	for seed := uint64(1); seed <= 64; seed++ {
		for name, in := range inputs {
			flipped := flip(in, seed)
			for _, mode := range []bs.Mode{bs.ModeRF, bs.ModeNetwork} {
				cfg := &bs.Config{Mode: mode, ColorCode: 1, CACH: true}
				want := run(t, newRepeater(t, cfg), flipped, 2*time.Second)
				got := runWith(t, newRepeater(t, cfg), flipped, 2*time.Second, layer2.NewBurst())
				if len(got) != len(want) {
					t.Fatalf("%s seed %d mode %d: %d outputs, want %d", name, seed, mode, len(got), len(want))
				}
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("%s seed %d mode %d: output %d is %+v, want %+v", name, seed, mode, i, got[i], want[i])
					}
				}
			}
		}
	}
	var zero bs.Repeater
	if out := zero.Decoded(0, nil, layer2.Hint{Slot: enums.Slot1}, rf, nil); len(out) != 0 {
		t.Error("zero Repeater Decoded gave outputs")
	}
}

func flip(in []input, seed uint64) []input {
	out := make([]input, len(in))
	copy(out, in)
	x := seed*0x9E3779B97F4A7C15 | 1
	for i := range out {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		n := int(x % 24)
		for range n {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			bit := int(x % (8 * uint64(len(out[i].raw))))
			out[i].raw[bit/8] ^= 0x80 >> (bit % 8)
		}
	}
	return out
}

func TestLeadKey(t *testing.T) {
	t.Parallel()
	const lead = 20 * time.Millisecond
	r := newRepeater(t, &bs.Config{Mode: bs.ModeRF, ColorCode: 1, CACH: true, Lead: lead})
	in := call(t, 1, 9, 3191868, 1, enums.Slot1, rf, 0)
	out := r.Inbound(0, &in[0].raw, layer2.Hint{Slot: enums.Slot1}, rf, nil)
	out = r.Tick(0, out)
	for _, o := range out {
		if o.Kind == bs.OutputBurst {
			if o.Due != lead {
				t.Fatalf("first burst Due %v, want %v", o.Due, lead)
			}
			return
		}
	}
	t.Fatalf("no burst at Tick(0): %+v", out)
}
