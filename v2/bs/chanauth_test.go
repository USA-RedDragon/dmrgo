package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type driver struct {
	t   *testing.T
	r   *bs.Repeater
	now time.Duration
	out []bs.Output
	all []bs.Output
}

func newDriver(t *testing.T, cfg *bs.Config) *driver {
	t.Helper()
	return &driver{t: t, r: newRepeater(t, cfg), out: make([]bs.Output, 0, bs.MaxOutputs)}
}

func (d *driver) until(end time.Duration) []bs.Output {
	d.t.Helper()
	start := len(d.all)
	for {
		at, ok := d.r.Deadline()
		if !ok || at > end {
			break
		}
		d.now = max(d.now, at)
		d.out = d.r.Tick(d.now, d.out[:0])
		d.all = append(d.all, d.out...)
	}
	d.now = end
	return d.all[start:]
}

func (d *driver) in(at time.Duration, s enums.Slot, from bs.Source, raw layer2.RawBurst) []bs.Output {
	d.t.Helper()
	pre := d.until(at)
	var err error
	d.out, err = d.r.Inbound(at, &raw, layer2.Hint{Slot: s}, from, d.out[:0])
	if err != nil {
		d.t.Fatal(err)
	}
	d.all = append(d.all, d.out...)
	return append(pre, d.out...)
}

func (d *driver) feed(in []input) {
	for _, i := range in {
		d.in(i.at, i.slot, i.from, i.raw)
	}
}

func header(t testing.TB, cc enums.ColorCode, target, source uint32) layer2.RawBurst {
	t.Helper()
	return call(t, cc, target, source, 0, enums.Slot1, bs.Source{}, 0)[0].raw
}

func preCSBK(t testing.TB, cc enums.ColorCode, target, source uint32) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 3, TargetAddress: target, SourceAddress: source}}
	b, err := layer2.BuildBurst(&c, cc, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func slotBursts(t *testing.T, out []bs.Output, s enums.Slot) (dts []enums.DataType, sources []uint32) {
	t.Helper()
	var b layer2.Burst
	for _, o := range out {
		if o.Kind != bs.OutputBurst || o.Slot != s || o.To&bs.ToRF == 0 {
			continue
		}
		if err := b.DecodeFromBytes(o.Burst); err != nil {
			t.Fatal(err)
		}
		dt := b.SlotType.DataType
		if b.Kind == layer2.KindVoice {
			dt = enums.DataTypeReserved
		}
		dts = append(dts, dt)
		var src uint32
		switch d := b.Data.(type) {
		case *pdu.FullLinkControl:
			_, src, _ = d.Addresses()
		case *pdu.CSBK:
			_, src, _ = d.Addresses()
		}
		sources = append(sources, src)
	}
	return dts, sources
}

func keyed(t *testing.T, cfg bs.Config) *driver {
	t.Helper()
	cfg.ColorCode, cfg.CACH = 1, true
	d := newDriver(t, &cfg)
	d.feed(call(t, 1, 9, 3191868, 1, enums.Slot1, bs.Source{RF: true}, 0))
	return d
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1
func TestChanAuthSingleSiteVoice(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
	d.until(time.Second)
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotIdle {
		t.Fatalf("before: %v", st)
	}
	d.in(time.Second, enums.Slot1, rf, header(t, 1, 9, 3120005))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
		t.Fatalf("after the ChanAuth PDU: %v", st)
	}
	d.in(1060*time.Millisecond, enums.Slot1, rf, header(t, 1, 9, 3120007))
	out := d.until(1300 * time.Millisecond)
	dts, src := slotBursts(t, out, enums.Slot1)
	if len(dts) < 4 {
		t.Fatalf("%d bursts", len(dts))
	}
	for i := range dts {
		if dts[i] != enums.DataTypeVoiceLCHeader || src[i] != 3120005 {
			t.Fatalf("burst %d: %v from %d", i, dts[i], src[i])
		}
	}
	start := 1300 * time.Millisecond
	for _, o := range filter(append(out, d.all...), bs.OutputCallStart) {
		if o.Call.Source != 3191868 {
			t.Fatalf("call start before the authorized call: %+v", o.Call)
		}
	}
	mark := len(d.all)
	d.feed(call(t, 1, 9, 3120005, 1, enums.Slot1, rf, start))
	d.until(start + 2*time.Second)
	starts := filter(d.all[mark:], bs.OutputCallStart)
	if len(starts) != 1 || starts[0].Call.Source != 3120005 {
		t.Fatalf("starts %+v", starts)
	}
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotIdle {
		t.Fatalf("after the call: %v", st)
	}
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1
func TestChanAuthResponseTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hang    time.Duration
		burst   func(*testing.T) layer2.RawBurst
		rsp     time.Duration
		cfgRsp  time.Duration
		prev    bs.SlotState
		hangDT  enums.DataType
		noState bool
	}{
		{name: "voice in channel hang", hang: -1, burst: func(t *testing.T) layer2.RawBurst { return header(t, 1, 9, 3120005) }, rsp: 720 * time.Millisecond, prev: bs.SlotIdle, hangDT: enums.DataTypeIdle},
		{name: "voice in call hang", burst: func(t *testing.T) layer2.RawBurst { return header(t, 1, 9, 3120005) }, rsp: 720 * time.Millisecond, prev: bs.SlotCallHang, hangDT: enums.DataTypeTerminatorWithLC},
		{name: "Pre_CSBK in channel hang", hang: -1, burst: func(t *testing.T) layer2.RawBurst { return preCSBK(t, 1, 9, 3120005) }, rsp: 900 * time.Millisecond, prev: bs.SlotIdle, hangDT: enums.DataTypeIdle},
		{name: "configured response", hang: -1, cfgRsp: 300 * time.Millisecond, burst: func(t *testing.T) layer2.RawBurst { return preCSBK(t, 1, 9, 3120005) }, rsp: 300 * time.Millisecond, prev: bs.SlotIdle, hangDT: enums.DataTypeIdle},
		{name: "Pre_CSBK in call hang", burst: func(t *testing.T) layer2.RawBurst { return preCSBK(t, 1, 9, 3120005) }, noState: true, prev: bs.SlotCallHang},
		{name: "voice to another target in call hang", burst: func(t *testing.T) layer2.RawBurst { return header(t, 1, 92, 3120005) }, noState: true, prev: bs.SlotCallHang},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: tc.hang, ChanAuthResponse: tc.cfgRsp, Inactivity: -1})
			at := 1500 * time.Millisecond
			d.until(at)
			d.in(at, enums.Slot1, bs.Source{RF: true}, tc.burst(t))
			if tc.noState {
				if st := d.r.SlotState(enums.Slot1); st == bs.SlotChanAuth || st == bs.SlotChanAuthSelect {
					t.Fatalf("state %v", st)
				}
				return
			}
			d.until(at + tc.rsp - time.Millisecond)
			if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
				t.Fatalf("before T_BS_ChanAuthRsp: %v", st)
			}
			d.until(at + tc.rsp)
			if st := d.r.SlotState(enums.Slot1); st != tc.prev {
				t.Fatalf("after T_BS_ChanAuthRsp: %v", st)
			}
			out := d.until(at + tc.rsp + 300*time.Millisecond)
			dts, _ := slotBursts(t, out, enums.Slot1)
			for _, dt := range dts {
				if dt != tc.hangDT {
					t.Fatalf("hang bursts %v", dts)
				}
			}
		})
	}
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.2
func TestChanAuthMultiSite(t *testing.T) {
	t.Parallel()

	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthMultiSite, CallHang: -1})
	at := 1500 * time.Millisecond
	d.until(at)
	out := d.in(at, enums.Slot1, bs.Source{RF: true}, header(t, 1, 9, 3120005))
	fwd := 0
	for _, o := range filter(out, bs.OutputBurst) {
		if o.To == bs.ToNetwork && o.Slot == enums.Slot1 {
			fwd++
		}
	}
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuthSelect || fwd != 1 {
		t.Fatalf("state %v, %d forwarded", st, fwd)
	}
	d.in(at+100*time.Millisecond, enums.Slot1, bs.Source{Peer: 7}, header(t, 1, 9, 3120001))
	d.in(at+150*time.Millisecond, enums.Slot1, bs.Source{Peer: 8}, header(t, 1, 9, 3120003))
	out = d.until(at + 239*time.Millisecond)
	if dts, _ := slotBursts(t, out, enums.Slot1); len(dts) == 0 || dts[0] != enums.DataTypeIdle {
		t.Fatalf("select bursts %v", dts)
	}
	d.until(at + 240*time.Millisecond)
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
		t.Fatalf("after T_BS_ChanAuthSel: %v", st)
	}
	out = d.until(at + 600*time.Millisecond)
	dts, src := slotBursts(t, out, enums.Slot1)
	if len(dts) == 0 || dts[0] != enums.DataTypeVoiceLCHeader || src[0] != 3120001 {
		t.Fatalf("selected %v %v", dts, src)
	}
	mark := len(d.all)
	d.feed(call(t, 1, 9, 3120001, 1, enums.Slot1, bs.Source{Peer: 7}, at+600*time.Millisecond))
	d.until(at + 2*time.Second)
	starts := filter(d.all[mark:], bs.OutputCallStart)
	if len(starts) != 1 || starts[0].Call.Source != 3120001 || starts[0].From.Peer != 7 {
		t.Fatalf("starts %+v", starts)
	}
}

func TestChanAuthNotInHangTime(t *testing.T) {
	t.Parallel()

	d := newDriver(t, &bs.Config{ColorCode: 1, CACH: true, ChannelAuth: bs.ChannelAuthSingleSite})
	d.feed(call(t, 1, 9, 3120005, 1, enums.Slot1, bs.Source{RF: true}, 0))
	d.until(2 * time.Second)
	if starts := filter(d.all, bs.OutputCallStart); len(starts) != 1 {
		t.Fatalf("wakeup call: %+v", starts)
	}
	n := newDriver(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1, ChannelAuth: bs.ChannelAuthMultiSite})
	n.in(0, enums.Slot1, bs.Source{Peer: 1}, header(t, 1, 9, 3120005))
	if st := n.r.SlotState(enums.Slot1); st != bs.SlotVoice {
		t.Fatalf("network mode: %v", st)
	}
	s := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
	s.until(time.Second)
	s.in(time.Second, enums.Slot1, bs.Source{Peer: 1}, header(t, 1, 9, 3120005))
	if st := s.r.SlotState(enums.Slot1); st != bs.SlotVoice {
		t.Fatalf("network burst on a single site: %v", st)
	}
	if _, err := bs.New(&bs.Config{ChannelAuth: bs.ChannelAuthMultiSite + 1}); err == nil {
		t.Fatal("New took an unknown ChannelAuth")
	}
	names := map[bs.ChannelAuth]string{bs.ChannelAuthOff: "Off", bs.ChannelAuthSingleSite: "SingleSite", bs.ChannelAuthMultiSite: "MultiSite", 9: "ChannelAuth(9)"}
	for c, want := range names {
		if c.String() != want {
			t.Errorf("%d: %q", c, c.String())
		}
	}
	if bs.SlotChanAuth.String() != "ChanAuth" || bs.SlotChanAuthSelect.String() != "ChanAuthSelect" {
		t.Error("slot state names")
	}
}

// ETSI TS 102 361-2 clauses 5.1.1.6.1.1 and 5.1.1.6.2
func TestChanAuthWithTransmitter(t *testing.T) {
	t.Parallel()

	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
	start := 1500 * time.Millisecond
	d.until(start)
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{ColorCode: 1, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot(), ChanAuth: constants.TMSChanAuth, Queue: 3 * 12})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(start, voice.Call{Group: true, Target: 9, Source: 3120005}, &voice.CallOptions{HangTime: true}); err != nil {
		t.Fatal(err)
	}
	for range 3 * 12 {
		if err := tx.Frame(vocoder.AMBESilence()); err != nil {
			t.Fatal(err)
		}
	}
	mark := len(d.all)
	txOut := make([]voice.Output, 0, voice.MaxOutputs)
	states := map[voice.TransmitterState]bool{}
	for now := start; now < start+3*time.Second; now += 10 * time.Millisecond {
		states[tx.State()] = true
		for _, o := range tx.Tick(now, txOut[:0]) {
			if o.Kind == voice.OutputBurst {
				d.in(o.Due, enums.Slot1, bs.Source{RF: true}, o.Burst)
			}
		}
		mark2 := len(d.all)
		d.until(now)
		for _, o := range d.all[mark2:] {
			if o.Kind == bs.OutputBurst && o.Slot == enums.Slot1 && o.To&bs.ToRF != 0 {
				for _, e := range tx.Burst(now, &o.Burst, layer2.Hint{}, txOut[:0]) {
					if e.Kind == voice.OutputBurst {
						d.in(e.Due, enums.Slot1, bs.Source{RF: true}, e.Burst)
					}
				}
			}
		}
		if now == start+time.Second {
			tx.Stop()
		}
	}
	starts := filter(d.all[mark:], bs.OutputCallStart)
	if len(starts) != 1 || starts[0].Call.Source != 3120005 || !states[voice.TransmitterChanAuth] || tx.State() != voice.TransmitterIdle {
		t.Fatalf("starts %+v, states %v", starts, states)
	}
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1
func TestChanAuthMatch(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
	d.until(time.Second)
	d.in(time.Second, enums.Slot1, rf, preCSBK(t, 1, 9, 3120005))
	d.in(1060*time.Millisecond, enums.Slot1, rf, preCSBK(t, 1, 9, 3120006))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
		t.Fatalf("other source: %v", st)
	}
	d.in(1300*time.Millisecond, enums.Slot1, rf, preCSBK(t, 1, 9, 3120005))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotIdle {
		t.Fatalf("matching Pre_CSBK: %v", st)
	}

	v := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite})
	in := call(t, 1, 9, 3120005, 1, enums.Slot1, rf, 1500*time.Millisecond)
	v.until(1500 * time.Millisecond)
	mark := len(v.all)
	v.feed(in[:1])
	if st := v.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
		t.Fatalf("header in call hang: %v", st)
	}
	v.feed(in[1:])
	v.until(4 * time.Second)
	if starts := filter(v.all[mark:], bs.OutputCallStart); len(starts) != 1 || starts[0].Call.Source != 3120005 {
		t.Fatalf("voice after the ChanAuth PDU: %+v", starts)
	}

	h := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: 1400 * time.Millisecond})
	h.until(1300 * time.Millisecond)
	h.in(1300*time.Millisecond, enums.Slot1, rf, header(t, 1, 9, 3120005))
	if st := h.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
		t.Fatalf("header in call hang: %v", st)
	}
	h.until(1300*time.Millisecond + 720*time.Millisecond)
	if st := h.r.SlotState(enums.Slot1); st != bs.SlotIdle {
		t.Fatalf("call hang ended during ChanAuth: %v", st)
	}
}
