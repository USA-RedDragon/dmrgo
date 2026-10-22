package voice_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func noiseBurst(r *rand.Rand) layer2.RawBurst {
	var raw layer2.RawBurst
	for i := range raw {
		raw[i] = byte(r.Uint32())
	}
	return raw
}

func TestRegression_NoiseIsNotVoice(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(1)
	for _, cfg := range []*voice.ReceiverConfig{nil, {ColorCode: &cc}} {
		r := rand.New(rand.NewPCG(1, 2))
		rx := newReceiver(t, cfg)
		in := make([]input, 0, 2000)
		for i := range 2000 {
			in = append(in, input{at: time.Duration(i) * slot, raw: noiseBurst(r)})
		}
		if n := len(find(feed(t, rx, in, -1), voice.EventVoice)); n != 0 {
			t.Errorf("color code %v: %d EventVoice from %d random bursts", cfg != nil, n, len(in))
		}
	}
}

func TestRegression_NoiseAfterTerminator(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(1)
	for _, cfg := range []*voice.ReceiverConfig{nil, {ColorCode: &cc}} {
		r := rand.New(rand.NewPCG(7, 8))
		rx := newReceiver(t, cfg)
		events := make([]voice.Event, 0, 64*50)
		at := time.Duration(0)
		for range 50 {
			call := transmission(t, groupLC(t, 91, 3191868), 1, nil)
			for _, b := range call {
				events = append(events, feed(t, rx, []input{{at: at, raw: b}}, -1)...)
				at += slot
			}
			for range 12 {
				events = append(events, feed(t, rx, []input{{at: at, raw: noiseBurst(r)}}, -1)...)
				at += slot
			}
			at += 5 * time.Second
		}
		if n := len(find(events, voice.EventVoice)); n != 50*layer2.VoiceBurstsPerSuperframe {
			t.Errorf("color code %v: %d EventVoice for %d voice bursts and noise after each Terminator", cfg != nil, n, 50*layer2.VoiceBurstsPerSuperframe)
		}
	}
}

func TestRegression_OtherCodecMissDoesNotCascade(t *testing.T) {
	t.Parallel()

	lc := groupLC(t, 91, 3191868)
	lc.FID = 0xC2
	vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(9, 10))
	call := vt.AppendHeaders(nil)
	for range 2 * layer2.VoiceBurstsPerSuperframe * layer2.VoiceSlotsPerBurst {
		var s [9]byte
		for i := range s {
			s[i] = byte(r.Uint32())
		}
		if call, err = vt.AppendSlots(call, s); err != nil {
			t.Fatal(err)
		}
	}
	in := timed(call, 0, layer2.Hint{})
	for _, i := range []int{2, 9} {
		for _, bit := range []int{108, 109, 110, 148} {
			in[i].raw[bit/8] ^= 0x80 >> (bit % 8)
		}
	}
	got := find(feed(t, newReceiver(t, nil), in, -1), voice.EventVoice)
	if len(got) != 2*layer2.VoiceBurstsPerSuperframe {
		t.Errorf("%d EventVoice for %d voice bursts with 2 bad EMBs", len(got), 2*layer2.VoiceBurstsPerSuperframe)
	}
}

func TestRegression_NoiseBurstInCallIsCounted(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(5, 6))
	call := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	in := timed(call, 0, layer2.Hint{})
	in[4].raw = noiseBurst(r)
	events := feed(t, newReceiver(t, nil), in, -1)
	v := find(events, voice.EventVoice)
	end := find(events, voice.EventCallEnd)
	if len(end) != 1 || int(end[0].Stats.Bursts) != len(v) || end[0].Stats.Bursts+end[0].Stats.Lost != 12 || len(v) == 11 && v[3].Lost != 1 {
		t.Errorf("noise burst in a call: %s, end %+v", kinds(events), end)
	}
}

func TestRegression_NoiseAfterCarrierDrop(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(1)
	for _, cfg := range []*voice.ReceiverConfig{nil, {ColorCode: &cc}} {
		r := rand.New(rand.NewPCG(3, 4))
		call := transmission(t, groupLC(t, 91, 3191868), 3, nil)
		call = call[:len(call)-1]
		in := timed(call, 0, layer2.Hint{})
		start := len(in)
		for i := range 40 {
			in = append(in, input{at: time.Duration(start+i) * slot, raw: noiseBurst(r)})
		}
		rx := newReceiver(t, cfg)
		events := feed(t, rx, in, -1)
		voiceN, ended := 0, false
		for _, e := range events {
			switch e.Kind {
			case voice.EventVoice:
				voiceN++
			case voice.EventCallEnd:
				ended = true
				if e.Reason != voice.EndSyncLost || e.Stats.Duration > time.Duration(start+20)*slot {
					t.Errorf("color code %v: call ended %v after %v", cfg != nil, e.Reason, e.Stats.Duration)
				}
			case voice.EventCallStart, voice.EventEmbedded, voice.EventTalkerAlias, voice.EventPIHeader, voice.EventSessionStart, voice.EventSessionEnd:
			}
		}
		if want := 3 * layer2.VoiceBurstsPerSuperframe; voiceN != want || !ended {
			t.Errorf("color code %v: %d EventVoice for %d voice bursts and noise, ended %v: %s", cfg != nil, voiceN, want, ended, kinds(events))
		}
	}
}

func TestRegression_WeakCallKeepsVoice(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(1)
	for _, cfg := range []*voice.ReceiverConfig{nil, {ColorCode: &cc}} {
		r := rand.New(rand.NewPCG(11, 12))
		have, got := 0, 0
		for range 40 {
			vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(t, 91, 3191868), ColorCode: 1})
			if err != nil {
				t.Fatal(err)
			}
			call := vt.AppendHeaders(nil)
			for range 10 * layer2.VoiceBurstsPerSuperframe * layer2.VoiceSlotsPerBurst {
				var f vocoder.AMBEFrame
				for i := range 6 {
					f[i] = byte(r.Uint32())
				}
				f[6] = byte(r.Uint32()) & 0x80
				s, _ := vocoder.EncodeAMBE72(f)
				if call, err = vt.AppendSlots(call, s); err != nil {
					t.Fatal(err)
				}
			}
			if call, err = vt.AppendTerminate(call); err != nil {
				t.Fatal(err)
			}
			rx := newReceiver(t, cfg)
			recent := make([]taken, 0, len(call))
			for i, sent := range call {
				raw := sent
				for k := range 264 {
					if r.Float64() < 0.08 {
						raw[k/8] ^= 0x80 >> (k % 8)
					}
				}
				good := 0
				if i > 0 {
					for j, s := range raw.VoiceSlots() {
						f, res := vocoder.DecodeAMBE72(s)
						w, _ := vocoder.DecodeAMBE72(sent.VoiceSlots()[j])
						if !res.Uncorrectable && [3]byte(f[:3]) == [3]byte(w[:3]) {
							good++
						}
					}
				}
				have += good
				recent = append(recent, taken{raw: raw, good: good})
				for _, e := range find(feed(t, rx, []input{{at: time.Duration(i) * slot, raw: raw}}, -1), voice.EventVoice) {
					for k := range recent {
						if recent[k].raw == e.Burst {
							got += recent[k].good
							recent[k].good = 0
						}
					}
				}
			}
		}
		if floor := 999 - 49*boolInt(cfg != nil); got*1000 < have*floor {
			t.Errorf("color code %v: %d of %d correct frames at 8%% BER", cfg != nil, got, have)
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestRegression_SyncCheckRejectsStructuredNoise(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(1)
	ours := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	ours = ours[:len(ours)-1]
	other := transmission(t, groupLC(t, 92, 3191869), 3, func(o *layer2.VoiceTransmitterOptions) { o.ColorCode = 2 })
	var zero, ones layer2.RawBurst
	for i := range ones {
		ones[i] = 0xFF
	}
	for name, tail := range map[string][]layer2.RawBurst{
		"other color code": other[1 : len(other)-1],
		"zeros":            {zero, zero, zero, zero, zero, zero, zero, zero},
		"ones":             {ones, ones, ones, ones, ones, ones, ones, ones},
	} {
		for _, cfg := range []*voice.ReceiverConfig{nil, {ColorCode: &cc}} {
			in := timed(append(append([]layer2.RawBurst(nil), ours...), tail...), 0, layer2.Hint{})
			events := feed(t, newReceiver(t, cfg), in, -1)
			extra := len(find(events, voice.EventVoice)) - 2*layer2.VoiceBurstsPerSuperframe
			switch {
			case name == "other color code" && cfg == nil:
			case name == "other color code" && extra > 3:
				t.Errorf("%s: %d bursts taken after our call, more than its 3 bursts A", name, extra)
			case name != "other color code" && extra != 0:
				t.Errorf("%s, color code %v: %d bursts taken after our call", name, cfg != nil, extra)
			}
		}
	}
}

func TestRegression_NoiseAfterLostCall(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(13, 14))
	call := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	call = call[:len(call)-1]
	n := 0
	for range 50 {
		rx := newReceiver(t, nil)
		in := timed(call, 0, layer2.Hint{})
		start := time.Duration(len(call))*slot + 2*time.Second
		for i := range 40 {
			in = append(in, input{at: start + time.Duration(i)*slot, raw: noiseBurst(r)})
		}
		n += len(find(feed(t, rx, in, -1), voice.EventVoice)) - 2*layer2.VoiceBurstsPerSuperframe
	}
	if n > 1 {
		t.Errorf("%d noise bursts taken after 50 lost calls", n)
	}
}

func TestRegression_DecodedBadFECDoesNotPanic(t *testing.T) {
	t.Parallel()

	call := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	rx := newReceiver(t, nil)
	for i, raw := range call {
		b := layer2.NewBurst()
		_ = b.DecodeFromBytes(raw)
		b.EmbeddedSignalling.FEC.ErrorsCorrected = -1 - i
		b.SyncDistance = 1000 * i
		_ = rx.Decoded(time.Duration(i)*slot, b, layer2.Hint{}, nil)
	}
}

func TestRegression_RepeatedNoiseBurstEndsCall(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(15, 16))
	call := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	call = call[:len(call)-1]
	held := 0
	for range 1000 {
		noise := noiseBurst(r)
		in := timed(call, 0, layer2.Hint{})
		for i := range 60 {
			in = append(in, input{at: time.Duration(len(call)+i) * slot, raw: noise})
		}
		events := feed(t, newReceiver(t, nil), in, -1)
		if extra := len(find(events, voice.EventVoice)) - 2*layer2.VoiceBurstsPerSuperframe; extra > 1 || len(find(events, voice.EventCallEnd)) == 0 {
			held++
		}
	}
	if held != 0 {
		t.Errorf("%d of 1000 calls took or stayed open on 60 copies of one noise burst", held)
	}
}

func TestRegression_StrongBurstsNotDelayed(t *testing.T) {
	t.Parallel()

	call := transmission(t, groupLC(t, 91, 3191868), 3, nil)
	rx := newReceiver(t, nil)
	for i, raw := range call[1 : len(call)-1] {
		events := rx.Burst(time.Duration(i+1)*slot, &raw, layer2.Hint{}, nil)
		if v := find(events, voice.EventVoice); len(v) != 1 || v[0].Burst != raw {
			t.Fatalf("burst %d: %s", i, kinds(events))
		}
	}
}

type taken struct {
	raw  layer2.RawBurst
	good int
}

func TestRegression_HeldNoiseExpires(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(17, 18))
	call := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	term := call[len(call)-1]
	n := 0
	for range 500 {
		rx := newReceiver(t, nil)
		in := []input{{at: 0, raw: noiseBurst(r)}, {at: 10 * time.Second, raw: term}}
		n += len(find(feed(t, rx, in, -1), voice.EventVoice))
	}
	if n != 0 {
		t.Errorf("%d noise bursts went out with a Terminator 10 s later", n)
	}
}

func TestRegression_NoiseBeforeLateEntry(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(19, 20))
	call := transmission(t, groupLC(t, 91, 3191868), 4, nil)
	leaked := 0
	for range 200 {
		noise := make([]layer2.RawBurst, 0, 40)
		in := make([]input, 0, 40+len(call))
		for i := range 40 {
			noise = append(noise, noiseBurst(r))
			in = append(in, input{at: time.Duration(i) * slot, raw: noise[i]})
		}
		for i, b := range call[2+r.IntN(12):] {
			in = append(in, input{at: time.Duration(40+i) * slot, raw: b})
		}
		for _, e := range find(feed(t, newReceiver(t, nil), in, -1), voice.EventVoice) {
			for _, nb := range noise {
				if e.Burst == nb {
					leaked++
				}
			}
		}
	}
	if leaked > 10 {
		t.Errorf("%d noise bursts went out before 200 late entries", leaked)
	}
}
