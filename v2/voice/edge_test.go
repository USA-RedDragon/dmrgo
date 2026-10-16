package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// TestReceiver_ShortSequence runs a call of 300 voice bursts whose 8-bit
// DMRD sequence number wraps, then a sequence that jumps back by more than
// a few bursts, as after a transport restart.
func TestReceiver_ShortSequence(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 50, nil)
	in := timed(bursts, 0, layer2.Hint{})
	for i := range in {
		seq := uint16(i)
		if i > 200 {
			seq -= 100
		}
		in[i].hint = layer2.Hint{Seq: seq & 0xFF, HasSeq: true, SeqBits: 8}
	}
	got := feed(t, newReceiver(t, nil), in, -1)
	end := got[len(got)-1]
	if end.Kind != voice.EventCallEnd || end.Stats.Bursts != 300 || end.Stats.Late != 0 || end.Stats.Lost != 0 {
		t.Fatalf("end %s %+v", end.Kind, end.Stats)
	}
	if p := positions(got); p != repeat("ABCDEF", 50) {
		t.Errorf("positions %s", p)
	}
}

// TestReceiver_BunchedArrival gets the voice bursts of a call in pairs, as
// from a serial port read without sequence numbers. Only a burst with the
// same bits as the one before it is a duplicate.
func TestReceiver_BunchedArrival(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 4, nil)
	in := timed(bursts, 0, layer2.Hint{})
	for i := range in {
		in[i].at = time.Duration(i/2) * 2 * slot
	}
	in = append(in[:5:5], append([]input{{at: in[4].at, raw: in[4].raw}}, in[5:]...)...)
	got := feed(t, newReceiver(t, nil), in, -1)
	end := got[len(got)-1]
	if end.Stats.Bursts != 24 || end.Stats.Duplicates != 1 || end.Stats.Lost != 0 {
		t.Fatalf("stats %+v", end.Stats)
	}
	if p := positions(got); p != repeat("ABCDEF", 4) {
		t.Errorf("positions %s", p)
	}
}

// TestReceiver_HeaderAfterLateEntry gets voice of one call without its LC,
// then the header of another call. The stats of the new call start at its
// header.
func TestReceiver_HeaderAfterLateEntry(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 1111), 1, nil)
	b := transmission(t, groupLC(t, 92, 2222), 2, nil)
	in := append(timed(a[2:5], 0, layer2.Hint{}), timed(b, 3*slot, layer2.Hint{})...)
	got := feed(t, newReceiver(t, nil), in, -1)
	end := got[len(got)-1]
	if k := kinds(got); k != "VVVS"+repeat("V", 12)+"E" {
		t.Fatalf("events %s", k)
	}
	if end.Stats.Bursts != 12 || end.Stats.Duration != 13*slot || end.Call.Source != 2222 {
		t.Errorf("end %+v %+v", end.Call, end.Stats)
	}
}

// TestReceiver_UndecodableHint gives a burst that does not decode, with a
// Hint that ends the call.
func TestReceiver_UndecodableHint(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	other, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 2, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	cc := enums.ColorCode(1)
	for _, tt := range []struct {
		hint layer2.Hint
		end  voice.EndReason
	}{
		{layer2.Hint{SyncLost: true}, voice.EndSyncLost},
		{layer2.Hint{End: layer2.EndStrong}, voice.EndTransport},
		{layer2.Hint{HasStream: true, Stream: 2}, voice.EndSuperseded},
	} {
		rx := newReceiver(t, &voice.ReceiverConfig{ColorCode: &cc})
		in := timed(bursts[:4], 0, layer2.Hint{HasStream: true, Stream: 1})
		in = append(in, input{at: 4 * slot, raw: other, hint: tt.hint})
		got := feed(t, rx, in, -1)
		if k := kinds(got); k != "SVVVE" || got[4].Reason != tt.end {
			t.Errorf("%+v: events %s, end %v", tt.hint, k, got[len(got)-1].Reason)
		}
	}
}

// TestReceiver_DecodedBurstA checks that ReceiverConfig.ColorCode does not
// drop a burst A given to Decoded.
func TestReceiver_DecodedBurstA(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	var b layer2.Burst
	if err := b.DecodeFromBytes(bursts[1]); err != nil || b.VoiceBurst != enums.VoiceBurstA {
		t.Fatalf("burst A: %v", err)
	}
	cc := enums.ColorCode(1)
	got := feed(t, newReceiver(t, &voice.ReceiverConfig{ColorCode: &cc}), []input{{decoded: &b}}, -1)
	if k := kinds(got); k != "V" {
		t.Errorf("events %s", k)
	}
}

// TestReceiver_LateEntryHangEnds has Lost off, and gets one voice burst
// without an LC in the hang time. The hang time still ends.
func TestReceiver_LateEntryHangEnds(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	rx := newReceiver(t, &voice.ReceiverConfig{Lost: -1})
	end := time.Duration(len(a)-1) * slot
	got := feed(t, rx, append(timed(a, 0, layer2.Hint{}), input{at: end + time.Second, raw: a[3]}), -1)
	if k := kinds(got); k != "S"+repeat("V", 6)+"EV" || rx.State() != voice.StateLateEntry {
		t.Fatalf("events %s, state %v", k, rx.State())
	}
	at, ok := rx.Deadline()
	if !ok || at != end+3*time.Second {
		t.Fatalf("deadline %v %v", at, ok)
	}
	got = rx.Tick(at, nil)
	if k := kinds(got); k != "e" || rx.State() != voice.StateLateEntry {
		t.Fatalf("tick: %s, state %v", k, rx.State())
	}
	if _, ok := rx.Deadline(); ok {
		t.Error("deadline after the hang time, with Lost off")
	}
}
