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

func newTransmitter(t *testing.T, cfg *voice.TransmitterConfig) *voice.Transmitter {
	t.Helper()
	tx, err := voice.NewTransmitter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func drain(t *testing.T, tx *voice.Transmitter, limit int) []voice.Output {
	t.Helper()
	var all []voice.Output
	buf := make([]voice.Output, 0, voice.MaxOutputs)
	for len(all) < limit {
		at, ok := tx.Deadline()
		if !ok {
			break
		}
		buf = tx.Tick(at, buf[:0])
		if len(buf) == 0 || len(buf) > voice.MaxOutputs {
			t.Fatalf("Tick at %v: %d outputs", at, len(buf))
		}
		for _, o := range buf {
			if n := len(all); o.Kind == voice.OutputBurst && n > 0 && all[n-1].Kind == voice.OutputBurst && o.Due != all[n-1].Due+slot {
				t.Fatalf("burst due %v after %v", o.Due, all[n-1].Due)
			}
		}
		all = append(all, buf...)
	}
	return all
}

func outBursts(out []voice.Output) []layer2.RawBurst {
	var b []layer2.RawBurst
	for _, o := range out {
		if o.Kind == voice.OutputBurst {
			b = append(b, o.Burst)
		}
	}
	return b
}

func framesOf(t *testing.T, bursts []layer2.RawBurst) []vocoder.AMBEFrame {
	t.Helper()
	var out []vocoder.AMBEFrame
	for _, raw := range bursts {
		var b layer2.Burst
		if err := b.DecodeFromBytes(raw); err != nil {
			t.Fatal(err)
		}
		if b.Kind != layer2.KindVoice {
			continue
		}
		f, res := b.AMBEFrames()
		if res.Uncorrectable {
			t.Fatal("uncorrectable frame")
		}
		out = append(out, f[:]...)
	}
	return out
}

func TestTransmitter_Golden(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"voice", "voice-late", "pi"} {
		golden := loadGolden(t, name)[1:]
		var b layer2.Burst
		if err := b.DecodeFromBytes(golden[len(golden)-1]); err != nil {
			t.Fatal(err)
		}
		lc, ok := b.Data.(*pdu.FullLinkControl)
		if !ok {
			t.Fatalf("%s: last burst is %T", name, b.Data)
		}
		octets, err := lc.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		c := voice.Call{Group: true, LC: octets}
		lcb := pdu.LCBytes(octets)
		c.Target, c.Source, _ = lcb.Addresses()
		c.Options, _ = lcb.ServiceOptions()
		var opts *voice.CallOptions
		if name == "pi" {
			var pb layer2.Burst
			if err := pb.DecodeFromBytes(golden[1]); err != nil {
				t.Fatal(err)
			}
			pi, ok := pb.Data.(*pdu.PIHeader)
			if !ok {
				t.Fatalf("pi burst is %T", pb.Data)
			}
			opts = &voice.CallOptions{PIHeader: pi}
		}
		tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 7, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot(), Queue: 3 * 12})
		if err := tx.Start(0, c, opts); err != nil {
			t.Fatal(err)
		}
		if err := tx.Frames(framesOf(t, golden)...); err != nil {
			t.Fatal(err)
		}
		tx.Stop()
		got := outBursts(drain(t, tx, 100))
		if name == "voice-late" {
			got = got[1:]
		}
		if len(got) != len(golden) {
			t.Fatalf("%s: %d bursts, want %d", name, len(got), len(golden))
		}
		for i := range got {
			if got[i] != golden[i] {
				t.Errorf("%s burst %d differs", name, i)
			}
		}
	}
}

func TestTransmitter_Source(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Headers: 3, Terminators: 2, Queue: 3 * 12})
	call := voice.Call{Group: true, Target: 91, Source: 3191868}
	if err := tx.Start(time.Second, call, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(time.Second, call, nil); !errors.Is(err, voice.ErrBusy) {
		t.Fatalf("second Start: %v", err)
	}
	if at, ok := tx.Deadline(); !ok || at != time.Second {
		t.Fatalf("deadline %v %v", at, ok)
	}
	for i := range 30 {
		if err := tx.Frame(vocoder.AMBESilence()); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	got := drain(t, tx, 6)
	tx.Stop()
	got = append(got, drain(t, tx, 100)...)
	var pos []byte
	fills := 0
	for _, o := range got {
		switch {
		case o.Kind == voice.OutputEnded:
			pos = append(pos, '.')
		case o.Hint.DataType == enums.DataTypeVoiceLCHeader:
			pos = append(pos, 'h')
		case o.Hint.DataType == enums.DataTypeTerminatorWithLC:
			pos = append(pos, 't')
		default:
			pos = append(pos, o.Hint.VoiceBurst.String()[0])
		}
		if o.Fill {
			fills++
		}
	}
	if string(pos) != "hhhABCDEFABCDEFtt." || fills != 2 || got[0].Due != time.Second {
		t.Fatalf("outputs %s, %d fills", pos, fills)
	}
	if got[len(got)-1].Reason != voice.EndTerminator || tx.State() != voice.TransmitterIdle {
		t.Errorf("end %v, state %v", got[len(got)-1].Reason, tx.State())
	}
}

func TestTransmitter_RoundTrip(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 3, Silence: vocoder.AMBESilenceSlot(), Queue: 3 * 40})
	gps := pdu.FLCGPSInfo{Longitude: -87.5, Latitude: 41.75}
	opts := &voice.CallOptions{TalkerAlias: "KI5VMF JACOB", TalkerAliasFormat: layer3.TalkerAliasISOEightBit, GPS: &gps}
	if err := tx.Start(0, voice.Call{Group: true, Target: 91, Source: 3191868}, opts); err != nil {
		t.Fatal(err)
	}
	for i := range 3 * 6 * 6 {
		var s [9]byte
		s[i%72/8] = 0x80 >> (i % 8)
		if err := tx.VocoderSlot(s); err != nil {
			t.Fatal(err)
		}
	}
	tx.Stop()
	out := drain(t, tx, 200)
	var in []input
	for i, o := range out {
		if o.Kind == voice.OutputBurst && i != 14 {
			in = append(in, input{at: o.Due, raw: o.Burst})
		}
	}
	got := feed(t, newReceiver(t, nil), in, -1)
	starts, ends := find(got, voice.EventCallStart), find(got, voice.EventCallEnd)
	if len(starts) != 1 || len(ends) != 1 || ends[0].Reason != voice.EndTerminator || ends[0].Stats.Lost != 1 || ends[0].Stats.Bursts != 35 {
		t.Fatalf("events %s, end %+v", kinds(got), ends)
	}
	if ta := find(got, voice.EventTalkerAlias); len(ta) != 1 || string(ta[0].Text) != "KI5VMF JACOB" {
		t.Errorf("talker alias %v", kinds(got))
	}
	gpsSeen := false
	for _, e := range find(got, voice.EventEmbedded) {
		_, ok := e.LC.AsGPSInfo()
		gpsSeen = gpsSeen || ok
	}
	if !gpsSeen {
		t.Error("no GPS")
	}
}

// ETSI TS 102 361-2 clause 6.1
func TestTransmitter_Timeout(t *testing.T) {
	t.Parallel()

	for k := range 6 {
		timeout := time.Duration(8+k) * slot
		tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Timeout: timeout, Queue: 3 * 20})
		if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
			t.Fatal(err)
		}
		for range 3 * 18 {
			_ = tx.Frame(vocoder.AMBESilence())
		}
		out := drain(t, tx, 100)
		b := outBursts(out)
		if len(b) != 1+12+1 || tx.State() != voice.TransmitterTimedOut || len(out) != len(b) {
			t.Fatalf("expiry at %d: %d bursts, %d outputs, state %v", k, len(b), len(out), tx.State())
		}
		if err := tx.Frame(vocoder.AMBESilence()); !errors.Is(err, voice.ErrTimedOut) {
			t.Fatalf("frame after T_TO: %v", err)
		}
		tx.Stop()
		end := drain(t, tx, 10)
		if len(end) != 1 || end[0].Kind != voice.OutputEnded || end[0].Reason != voice.EndTimeout || tx.State() != voice.TransmitterIdle {
			t.Fatalf("after Stop: %+v", end)
		}
	}
}

func TestTransmitter_Relay(t *testing.T) {
	t.Parallel()

	in := loadWeak(t)
	rx := newReceiver(t, nil)
	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 7, Silence: vocoder.AMBESilenceSlot()})
	ev := make([]voice.Event, 0, voice.MaxEvents)
	out := make([]voice.Output, 0, voice.MaxOutputs)
	sent := make([]voice.Output, 0, 64)
	for i := range in {
		ev = rx.Burst(in[i].at, &in[i].raw, in[i].hint, ev[:0])
		for j := range ev {
			if err := tx.Relay(in[i].at, &ev[j]); err != nil {
				t.Fatal(err)
			}
		}
		out = tx.Tick(in[i].at, out[:0])
		sent = append(sent, out...)
	}
	sent = append(sent, drain(t, tx, 20)...)
	got := outBursts(sent)
	want := make([]layer2.RawBurst, 0, len(in))
	for _, x := range in {
		want = append(want, x.raw)
	}
	if len(got) != len(want) || sent[len(sent)-1].Kind != voice.OutputEnded || sent[len(sent)-1].Reason != voice.EndTerminator {
		t.Fatalf("%d bursts, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("burst %d differs", i)
		}
	}
	if tx.Dropped() != 0 {
		t.Errorf("dropped %d", tx.Dropped())
	}
}

func TestTransmitter_RelayLateEntry(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 9, 3191868), 3, nil)
	rx := newReceiver(t, nil)
	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot()})
	ev := make([]voice.Event, 0, voice.MaxEvents)
	sent := make([]voice.Output, 0, 64)
	for i, raw := range bursts[4:] {
		now := time.Duration(i) * slot
		ev = rx.Burst(now, &raw, layer2.Hint{}, ev[:0])
		for j := range ev {
			_ = tx.Relay(now, &ev[j])
		}
		sent = append(sent, tx.Tick(now, nil)...)
	}
	sent = append(sent, drain(t, tx, 30)...)
	if sent[0].Hint.DataType != enums.DataTypeVoiceLCHeader || tx.Dropped() != 7 {
		t.Fatalf("first output %+v, dropped %d", sent[0], tx.Dropped())
	}
	got := feed(t, newReceiver(t, nil), timedOutputs(sent), -1)
	if k := kinds(got); k[0] != 'S' || k[len(k)-1] != 'E' || len(find(got, voice.EventVoice)) != 12 {
		t.Errorf("relayed call: %s", k)
	}
}

func timedOutputs(out []voice.Output) []input {
	var in []input
	for _, o := range out {
		if o.Kind == voice.OutputBurst {
			in = append(in, input{at: o.Due, raw: o.Burst})
		}
	}
	return in
}

func TestTransmitter_RelayLost(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 9, 3191868), 2, nil)
	rx := newReceiver(t, nil)
	tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Queue: 3 * 8})
	ev := make([]voice.Event, 0, voice.MaxEvents)
	sent := make([]voice.Output, 0, 64)
	relay := func(now time.Duration) {
		for j := range ev {
			if err := tx.Relay(now, &ev[j]); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, raw := range bursts[:10] {
		if i == 4 || i == 5 {
			continue
		}
		now := time.Duration(i) * slot
		ev = rx.Burst(now, &raw, layer2.Hint{}, ev[:0])
		relay(now)
	}
	ev = rx.Tick(10*slot+time.Second, ev[:0])
	relay(10*slot + time.Second)
	sent = append(sent, drain(t, tx, 40)...)
	end := sent[len(sent)-1]
	fills := 0
	for _, o := range sent {
		if o.Fill {
			fills++
		}
	}
	if end.Kind != voice.OutputEnded || end.Reason != voice.EndLost || len(outBursts(sent)) != 1+12+1 || fills != 4 {
		t.Fatalf("%d bursts, end %+v, %d fills", len(outBursts(sent)), end, fills)
	}
}

func TestTransmitter_Errors(t *testing.T) {
	t.Parallel()

	for _, cfg := range []voice.TransmitterConfig{
		{ColorCode: 16}, {Source: layer2.SyncSource(9)}, {Headers: 9}, {Terminators: 65}, {Headers: 3, Queue: 9}, {Queue: voice.MaxQueue + 1},
	} {
		var ce *dmrgo.ConfigError
		if _, err := voice.NewTransmitter(&cfg); !errors.As(err, &ce) {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
	tx := newTransmitter(t, nil)
	if err := tx.Frame(vocoder.AMBESilence()); !errors.Is(err, voice.ErrNotActive) {
		t.Errorf("frame before Start: %v", err)
	}
	if err := tx.Start(0, voice.Call{Target: 1 << 24}, nil); !errors.As(err, new(*dmrgo.FieldRangeError)) {
		t.Errorf("bad call: %v", err)
	}
	if err := tx.Start(0, voice.Call{Target: 1, Source: 2}, &voice.CallOptions{TalkerAlias: "€"}); !errors.As(err, new(*dmrgo.ConfigError)) {
		t.Errorf("bad alias: %v", err)
	}
	if err := tx.Start(0, voice.Call{Target: 1, Source: 2}, nil); err != nil {
		t.Fatal(err)
	}
	for range voice.DefaultQueue {
		if err := tx.Frame(vocoder.AMBESilence()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Frame(vocoder.AMBESilence()); !errors.Is(err, voice.ErrQueueFull) || tx.Room() != 0 {
		t.Errorf("full queue: %v", err)
	}
	tx.Reset()
	if tx.State() != voice.TransmitterIdle || tx.Room() != voice.DefaultQueue {
		t.Errorf("Reset: %v %d", tx.State(), tx.Room())
	}
	if voice.OutputEnded.String() != "Ended" || voice.TransmitterTimedOut.String() != "TimedOut" || voice.OutputKind(9).String() == "" ||
		voice.TransmitterState(9).String() == "" || voice.EndTimeout.String() != "Timeout" {
		t.Error("names")
	}
}

func TestTransmitter_Lead(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Lead: 20 * time.Millisecond})
	if err := tx.Start(time.Second, voice.Call{Target: 2, Source: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if out := tx.Tick(time.Second-21*time.Millisecond, nil); len(out) != 0 {
		t.Fatal("too early")
	}
	out := tx.Tick(time.Second-20*time.Millisecond, nil)
	if len(out) != 1 || out[0].Due != time.Second {
		t.Fatalf("%+v", out)
	}
}
