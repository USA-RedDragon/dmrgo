package voice_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func rcall() voice.Call {
	lc, _ := pdu.NewVoiceLCBytes(true, 91, 3191868, layer3.ServiceOptions{})
	return voice.Call{Group: true, Target: 91, Source: 3191868, LC: lc, Origin: voice.OriginHeader}
}

func vburst(t *testing.T, pos int) layer2.RawBurst {
	b := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	return b[1+pos]
}

func TestRegression_StopDuringTimeoutPadding(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Timeout: 8 * slot, Queue: 3 * 20})
	_ = tx.Start(0, voice.Call{Target: 2, Source: 1}, nil)
	for range 3 * 18 {
		_ = tx.Frame(vocoder.AMBESilence())
	}
	buf := make([]voice.Output, 0, 2)
	stopped := false
	for i := 0; i < 100; i++ {
		at, ok := tx.Deadline()
		if !ok {
			break
		}
		buf = tx.Tick(at, buf[:0])
		if !stopped && tx.State() == voice.TransmitterEnding && tx.Frame(vocoder.AMBESilence()) != nil {
			tx.Stop()
			stopped = true
		}
	}
	if !stopped {
		t.Fatal("no stop")
	}
	if tx.State() != voice.TransmitterIdle {
		_, ok := tx.Deadline()
		t.Errorf("after Stop in padding: state %v, deadline ok %v; Start: %v", tx.State(), ok, tx.Start(0, voice.Call{Target: 2, Source: 1}, nil))
	}
}

func TestRegression_ResetKeepsReason(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	c := rcall()
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallEnd, Call: c, Reason: voice.EndLost})
	tx.Reset()
	_ = tx.Start(0, voice.Call{Target: 2, Source: 1}, nil)
	tx.Stop()
	out := drain(t, tx, 20)
	if r := out[len(out)-1].Reason; r != voice.EndTerminator {
		t.Errorf("source call after Reset ends %v", r)
	}
}

func TestRegression_RelayPILate(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	c := rcall()
	o := layer2.VoiceTransmitterOptions{LC: groupLC(t, 91, 3191868), ColorCode: 1, PIHeader: &pdu.PIHeader{AlgorithmID: 1}}
	vt, err := layer2.NewVoiceTransmitter(&o)
	if err != nil {
		t.Fatal(err)
	}
	pi := vt.Headers()[1]
	sent := make([]voice.Output, 0, 64)
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	sent = append(sent, tx.Tick(0, nil)...)
	sent = append(sent, tx.Tick(slot, nil)...)
	_ = tx.Relay(2*slot, &voice.Event{Kind: voice.EventPIHeader, Call: c, Burst: pi})
	sent = append(sent, tx.Tick(2*slot, nil)...)
	sent = append(sent, tx.Tick(3*slot, nil)...)
	n := 0
	for _, s := range sent {
		var b layer2.Burst
		_ = b.DecodeFromBytes(s.Burst)
		if b.Kind == layer2.KindData && b.SlotType.DataType == enums.DataTypePIHeader {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d PI Headers sent, want 1", n)
	}
}

func TestRegression_NextCallGap(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	c := rcall()
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	tx.Tick(0, nil)
	_ = tx.Relay(slot, &voice.Event{Kind: voice.EventCallEnd, Call: c, Reason: voice.EndSuperseded})
	c2 := c
	c2.Source = 1234
	c2.LC, _ = pdu.NewVoiceLCBytes(true, 91, 1234, layer3.ServiceOptions{})
	if err := tx.Relay(slot, &voice.Event{Kind: voice.EventCallStart, Call: c2}); err != nil {
		t.Fatal(err)
	}
	var all []voice.Output
	for i := 1; i < 6; i++ {
		all = append(all, tx.Tick(time.Duration(i)*slot, nil)...)
	}
	var dues []time.Duration
	for _, o := range all {
		if o.Kind == voice.OutputBurst {
			dues = append(dues, o.Due)
		}
	}
	for i := 1; i < len(dues); i++ {
		if dues[i]-dues[i-1] != slot {
			t.Errorf("burst %d due %v after %v", i, dues[i], dues[i-1])
		}
	}
}

func TestRegression_NextCallVoiceDropped(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Terminators: 3, Queue: 3 * 12})
	c := rcall()
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	tx.Tick(0, nil)
	_ = tx.Relay(slot, &voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, 0), Position: enums.VoiceBurstA})
	tx.Tick(slot, nil)
	_ = tx.Relay(2*slot, &voice.Event{Kind: voice.EventCallEnd, Call: c, Reason: voice.EndSuperseded})
	c2 := c
	c2.Source = 1234
	c2.LC, _ = pdu.NewVoiceLCBytes(true, 91, 1234, layer3.ServiceOptions{})
	_ = tx.Relay(2*slot, &voice.Event{Kind: voice.EventCallStart, Call: c2})
	for i := 2; i < 12; i++ {
		_ = tx.Relay(time.Duration(i)*slot, &voice.Event{Kind: voice.EventVoice, Call: c2, Burst: vburst(t, (i-2)%6), Position: enums.VoiceBurstA})
		tx.Tick(time.Duration(i)*slot, nil)
	}
	if tx.Dropped() != 0 {
		t.Errorf("dropped %d voice bursts of the next call, whose LC is known", tx.Dropped())
	}
}

func TestRegression_RelayLostDoubleFill(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	c := rcall()
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	var out []voice.Output
	out = append(out, tx.Tick(0, nil)...)
	for i := 1; i <= 12; i++ {
		now := time.Duration(i) * slot
		if i != 3 {
			e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, (i-1)%6), Position: enums.VoiceBurstA}
			if i == 4 {
				e.Lost = 1
			}
			_ = tx.Relay(now, &e)
		}
		out = append(out, tx.Tick(now, nil)...)
	}
	fills := 0
	for _, o := range out {
		if o.Fill {
			fills++
		}
	}
	if fills != 1 || tx.Room() != voice.DefaultQueue {
		t.Errorf("%d Fill outputs for 1 lost burst, %d slots still queued after the last input (extra latency)", fills, voice.DefaultQueue-tx.Room())
	}
}

func TestRegression_StopIgnoresTimeout(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Timeout: 8 * slot, Queue: voice.MaxQueue})
	_ = tx.Start(0, voice.Call{Target: 2, Source: 1}, nil)
	for range voice.MaxQueue {
		_ = tx.Frame(vocoder.AMBESilence())
	}
	tx.Stop()
	out := drain(t, tx, 200)
	last := out[len(out)-1]
	if last.Due > 8*slot+7*slot {
		t.Errorf("ended at %v (%v) after %d outputs, T_TO %v", last.Due, last.Reason, len(out), 8*slot)
	}
}

func TestRegression_PIHeaderSeqLost(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 2, func(o *layer2.VoiceTransmitterOptions) {
		o.PIHeader = &pdu.PIHeader{AlgorithmID: 1}
	})
	in := make([]input, 0, len(bursts))
	for i, b := range bursts {
		in = append(in, input{at: time.Duration(i) * slot, raw: b, hint: layer2.Hint{Seq: uint16(100 + i), HasSeq: true}})
	}
	got := feed(t, newReceiver(t, nil), in, -1)
	for _, e := range find(got, voice.EventVoice) {
		if e.Lost != 0 {
			t.Errorf("voice at %v: Lost %d", e.Position, e.Lost)
		}
	}
	for _, e := range find(got, voice.EventCallEnd) {
		if e.Stats.Lost != 0 {
			t.Errorf("call end: Stats.Lost %d", e.Stats.Lost)
		}
	}
}

func TestRegression_RewriteEmbedded(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 4, nil)
	rx := newReceiver(t, nil)
	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot()})
	ev := make([]voice.Event, 0, voice.MaxEvents)
	sent := make([]voice.Output, 0, 64)
	for i := range bursts {
		now := time.Duration(i) * slot
		ev = rx.Burst(now, &bursts[i], layer2.Hint{}, ev[:0])
		for j := range ev {
			if ev[j].Kind == voice.EventCallStart {
				ev[j].Call.Target = 3100
			}
			_ = tx.Relay(now, &ev[j])
		}
		sent = append(sent, tx.Tick(now, nil)...)
	}
	sent = append(sent, drain(t, tx, 40)...)
	var vt layer2.VoiceReceiver
	var lc pdu.FullLinkControl
	var lcp pdu.FullLinkControlPayloads
	for _, o := range sent {
		if o.Kind != voice.OutputBurst || o.Hint.VoiceBurst == enums.VoiceBurstUnknown {
			continue
		}
		var b layer2.Burst
		_ = b.DecodeFromBytes(o.Burst)
		ok, _ := vt.ReceiveInto(&lc, &lcp, &b)
		if ok && lc.GroupVoice != nil && lc.GroupVoice.GroupAddress != 3100 {
			t.Errorf("embedded LC at %v: target %d, want 3100", o.Due, lc.GroupVoice.GroupAddress)
		}
	}
}

func TestRegression_PendingPI(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	c := rcall()
	_ = tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c})
	tx.Tick(0, nil)
	_ = tx.Relay(slot, &voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, 0), Position: enums.VoiceBurstA})
	tx.Tick(slot, nil)
	_ = tx.Relay(2*slot, &voice.Event{Kind: voice.EventCallEnd, Call: c, Reason: voice.EndSuperseded})
	c2 := rcall()
	c2.Source = 1234
	_ = tx.Relay(2*slot, &voice.Event{Kind: voice.EventCallStart, Call: c2})
	vt, _ := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(t, 91, 1234), ColorCode: 1, PIHeader: &pdu.PIHeader{AlgorithmID: 1}})
	_ = tx.Relay(3*slot, &voice.Event{Kind: voice.EventPIHeader, Call: c2, Burst: vt.Headers()[1]})
	sent := make([]voice.Output, 0, 64)
	for i := 2; i < 20; i++ {
		sent = append(sent, tx.Tick(time.Duration(i)*slot, nil)...)
	}
	n := 0
	for _, o := range sent {
		var b layer2.Burst
		_ = b.DecodeFromBytes(o.Burst)
		if o.Kind == voice.OutputBurst && b.Kind == layer2.KindData && b.SlotType.DataType == enums.DataTypePIHeader {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d PI Headers for the pending encrypted call", n)
	}
}

func TestRegression_BadGPS(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	gps := pdu.FLCGPSInfo{Latitude: 500, Longitude: 900}
	err := tx.Start(0, voice.Call{Target: 2, Source: 1}, &voice.CallOptions{GPS: &gps})
	var ce *dmrgo.ConfigError
	if !errors.As(err, &ce) || ce.Field != "GPS" {
		t.Errorf("Start with a GPS LC that does not encode: %v", err)
	}
}
