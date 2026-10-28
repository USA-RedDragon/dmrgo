package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRegression_RelayUnderrunRepeatsLastSlot(t *testing.T) {
	t.Parallel()

	sil := vocoder.AMBESilenceSlot()
	tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue})
	c := rcall()
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
		t.Fatal(err)
	}
	out := tx.Tick(0, nil)
	lost := map[int]bool{4: true, 7: true, 8: true}
	n := 0
	for i := 1; i <= 10; i++ {
		now := time.Duration(i) * slot
		if lost[i] {
			n++
		} else {
			e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, (i-1)%6), Position: enums.VoiceBurstA, Lost: uint8(n)}
			n = 0
			if err := tx.Relay(now, &e); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, tx.Tick(now, nil)...)
	}
	var v [][layer2.VoiceSlotsPerBurst][9]byte
	for _, o := range out {
		if o.Kind == voice.OutputBurst && o.Hint.Kind == layer2.KindVoice {
			v = append(v, o.Burst.VoiceSlots())
		}
	}
	if len(v) != 10 {
		t.Fatalf("%d voice bursts, want 10", len(v))
	}
	for _, i := range []int{4, 7} {
		last := v[i-2][2]
		if last == sil {
			t.Fatalf("burst %d: the test needs a slot other than silence", i-1)
		}
		for j, s := range v[i-1] {
			if s != last {
				t.Errorf("first lost burst %d slot %d = %x, want the last slot %x", i, j, s, last)
			}
		}
	}
	for j, s := range v[7] {
		if s != sil {
			t.Errorf("second lost burst 8 slot %d = %x, want silence", j, s)
		}
	}
}

func TestRegression_RelayUnderrunThenLostRepeatsOnce(t *testing.T) {
	t.Parallel()

	sil := vocoder.AMBESilenceSlot()
	tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue})
	c := rcall()
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
		t.Fatal(err)
	}
	out := tx.Tick(0, nil)
	relay := func(at time.Duration, pos int, lost uint8) {
		e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, pos), Position: enums.VoiceBurstA, Lost: lost}
		if err := tx.Relay(at, &e); err != nil {
			t.Fatal(err)
		}
	}
	relay(slot, 0, 0)
	out = append(out, tx.Tick(slot, nil)...)
	out = append(out, tx.Tick(2*slot, nil)...)
	relay(3*slot-slot/2, 3, 2)
	for i := 3; i <= 4; i++ {
		out = append(out, tx.Tick(time.Duration(i)*slot, nil)...)
	}
	var v [][layer2.VoiceSlotsPerBurst][9]byte
	for _, o := range out {
		if o.Kind == voice.OutputBurst && o.Hint.Kind == layer2.KindVoice {
			v = append(v, o.Burst.VoiceSlots())
		}
	}
	if len(v) != 4 {
		t.Fatalf("%d voice bursts, want 4", len(v))
	}
	for j := range v[1] {
		if v[1][j] != v[0][2] {
			t.Errorf("first lost burst slot %d = %x, want the last slot %x", j, v[1][j], v[0][2])
		}
		if v[2][j] != sil {
			t.Errorf("second lost burst slot %d = %x, want silence", j, v[2][j])
		}
	}
}

func TestRegression_RelayFillsLeaveRoomForVoice(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue})
	c := rcall()
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
		t.Fatal(err)
	}
	for _, lost := range []uint8{0, 255} {
		e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, 0), Position: enums.VoiceBurstA, Lost: lost}
		if err := tx.Relay(0, &e); err != nil {
			t.Fatal(err)
		}
	}
	if tx.Dropped() != 0 {
		t.Errorf("%d bursts dropped: the fills for Lost 255 left no room for the burst", tx.Dropped())
	}
}

func TestRegression_RelayPendingCallRepeatsLastSlot(t *testing.T) {
	t.Parallel()

	sil := vocoder.AMBESilenceSlot()
	for _, fid := range []enums.FID{enums.FIDStandardized, 0xC2} {
		tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue})
		a := rcall()
		lc, _ := pdu.NewVoiceLCBytes(true, 91, 3191869, layer3.ServiceOptions{})
		lc.SetFID(fid)
		b := voice.Call{Group: true, Target: 91, Source: 3191869, LC: lc, FID: fid, Origin: voice.OriginHeader}
		relay := func(e voice.Event) {
			if err := tx.Relay(slot, &e); err != nil {
				t.Fatal(err)
			}
		}
		relay(voice.Event{Kind: voice.EventCallStart, Call: a})
		out := tx.Tick(0, nil)
		relay(voice.Event{Kind: voice.EventVoice, Call: a, Burst: vburst(t, 0), Position: enums.VoiceBurstA})
		out = append(out, tx.Tick(slot, nil)...)
		relay(voice.Event{Kind: voice.EventCallEnd, Call: a})
		relay(voice.Event{Kind: voice.EventCallStart, Call: b})
		relay(voice.Event{Kind: voice.EventVoice, Call: b, Burst: vburst(t, 1), Position: enums.VoiceBurstA})
		relay(voice.Event{Kind: voice.EventVoice, Call: b, Burst: vburst(t, 3), Position: enums.VoiceBurstA, Lost: 1})
		for i := 2; i < 40; i++ {
			out = append(out, tx.Tick(time.Duration(i)*slot, nil)...)
		}
		var v [][layer2.VoiceSlotsPerBurst][9]byte
		ended := false
		for _, o := range out {
			switch {
			case o.Kind == voice.OutputEnded:
				ended = true
			case ended && o.Kind == voice.OutputBurst && o.Hint.Kind == layer2.KindVoice:
				v = append(v, o.Burst.VoiceSlots())
			}
		}
		if len(v) < 5 {
			t.Fatalf("FID %v: %d voice bursts of the pending call", fid, len(v))
		}
		for j := range v[1] {
			if v[1][j] != v[0][2] {
				t.Errorf("FID %v: lost burst slot %d = %x, want the last slot %x", fid, j, v[1][j], v[0][2])
			}
			if v[3][j] != v[2][2] {
				t.Errorf("FID %v: first underrun slot %d = %x, want the last slot %x", fid, j, v[3][j], v[2][2])
			}
			if want := map[bool][9]byte{true: sil, false: v[2][2]}[fid == enums.FIDStandardized]; v[4][j] != want {
				t.Errorf("FID %v: second underrun slot %d = %x, want %x", fid, j, v[4][j], want)
			}
		}
	}
}
