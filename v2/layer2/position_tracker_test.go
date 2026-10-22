package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func superframe(t *testing.T) [layer2.VoiceBurstsPerSuperframe]layer2.RawBurst {
	t.Helper()
	var out [layer2.VoiceBurstsPerSuperframe]layer2.RawBurst
	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	for i := range out {
		pos, _ := enums.VoiceBurstAt(i)
		r, err := layer2.BuildVoiceBurst(frames, pos, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{}, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = r
	}
	return out
}

func voiceHint(seq uint16) layer2.Hint {
	return layer2.Hint{Kind: layer2.KindVoice, Seq: seq, HasSeq: true, Stream: 7, HasStream: true}
}

func TestPositionTrackerBySequence(t *testing.T) {
	t.Parallel()

	sf := superframe(t)
	var tr layer2.PositionTracker
	seq := uint16(65530)
	for i := range 12 {
		if i == 8 {
			seq++
			continue
		}
		h := voiceHint(seq)
		raw := sf[i%6]
		lost, p := tr.Place(&raw, &h)
		want, _ := enums.VoiceBurstAt(i % 6)
		wantLost := 0
		if i == 9 {
			wantLost = 1
		}
		if h.VoiceBurst != want || lost != wantLost || p != layer2.PlacementInOrder {
			t.Fatalf("burst %d: %v lost %d %v, want %v", i, h.VoiceBurst, lost, p, want)
		}
		seq++
	}
	h := voiceHint(seq - 1)
	if _, p := tr.Place(&sf[5], &h); p != layer2.PlacementDuplicate || h.VoiceBurst != enums.VoiceBurstF {
		t.Fatalf("duplicate: %v %v", p, h.VoiceBurst)
	}
	h = voiceHint(seq - 4)
	if _, p := tr.Place(&sf[2], &h); p != layer2.PlacementLate || h.VoiceBurst != enums.VoiceBurstC {
		t.Fatalf("late: %v %v", p, h.VoiceBurst)
	}
	h = voiceHint(seq)
	if _, p := tr.Place(&sf[0], &h); p != layer2.PlacementInOrder || h.VoiceBurst != enums.VoiceBurstA {
		t.Fatalf("after late: %v %v", p, h.VoiceBurst)
	}
}

func TestPositionTrackerLateEntry(t *testing.T) {
	t.Parallel()

	sf := superframe(t)
	var tr layer2.PositionTracker
	want := []enums.VoiceBurstType{enums.VoiceBurstUnknown, enums.VoiceBurstUnknown, enums.VoiceBurstE, enums.VoiceBurstF, enums.VoiceBurstA, enums.VoiceBurstB, enums.VoiceBurstC}
	for i, w := range want {
		h := voiceHint(uint16(i))
		h.HasSeq = false
		raw := sf[(i+2)%6]
		tr.Place(&raw, &h)
		if h.VoiceBurst != w {
			t.Fatalf("burst %d: %v, want %v", i, h.VoiceBurst, w)
		}
	}
}

func TestPositionTrackerHeaderAndStream(t *testing.T) {
	t.Parallel()

	sf := superframe(t)
	var tr layer2.PositionTracker
	h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeVoiceLCHeader, HasDataType: true, Seq: 1, HasSeq: true, Stream: 1, HasStream: true}
	var hdr layer2.RawBurst
	tr.Place(&hdr, &h)
	b := sf[1]
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 4, HasSeq: true, Stream: 1, HasStream: true}
	if lost, _ := tr.Place(&sf[2], &h); h.VoiceBurst != enums.VoiceBurstC || lost != 2 {
		t.Fatalf("after header with A and B lost: %v lost %d", h.VoiceBurst, lost)
	}
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 4, HasSeq: true, Stream: 2, HasStream: true}
	if tr.Place(&sf[3], &h); h.VoiceBurst != enums.VoiceBurstUnknown {
		t.Fatalf("new stream: %v", h.VoiceBurst)
	}
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 5, HasSeq: true, Stream: 2, HasStream: true}
	if tr.Place(&b, &h); h.VoiceBurst != enums.VoiceBurstB {
		t.Fatalf("B by LCSS: %v", h.VoiceBurst)
	}
	h = layer2.Hint{Seq: 6, HasSeq: true, Stream: 2, HasStream: true, SyncLost: true}
	tr.Place(nil, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 7, HasSeq: true, Stream: 2, HasStream: true}
	if tr.Place(&sf[3], &h); h.VoiceBurst != enums.VoiceBurstUnknown {
		t.Fatalf("after sync lost: %v", h.VoiceBurst)
	}
}

func TestPositionTrackerLCSSAfterLostHeader(t *testing.T) {
	t.Parallel()

	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	b, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, pdu.EmbeddedSignalling{ColorCode: 1, LCSS: enums.LCSSFirstFragment}, [4]byte{}, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	sf := superframe(t)
	var tr layer2.PositionTracker
	h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeVoiceLCHeader, HasDataType: true, Seq: 0, HasSeq: true}
	var hdr layer2.RawBurst
	tr.Place(&hdr, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 3, HasSeq: true}
	if lost, _ := tr.Place(&b, &h); h.VoiceBurst != enums.VoiceBurstB || lost != 1 {
		t.Fatalf("B after a lost PI Header and a lost A: %v lost %d", h.VoiceBurst, lost)
	}
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 4, HasSeq: true}
	if lost, _ := tr.Place(&sf[2], &h); h.VoiceBurst != enums.VoiceBurstC || lost != 0 {
		t.Fatalf("C after B: %v lost %d", h.VoiceBurst, lost)
	}
}

func TestPositionTrackerLCSSAfterGuessFromHeader(t *testing.T) {
	t.Parallel()

	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	build := func(pos enums.VoiceBurstType, lcss enums.LCSS) layer2.RawBurst {
		r, err := layer2.BuildVoiceBurst(frames, pos, pdu.EmbeddedSignalling{ColorCode: 1, LCSS: lcss}, [4]byte{}, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	c, d, e := build(enums.VoiceBurstC, enums.LCSSContinuationFragment), build(enums.VoiceBurstD, enums.LCSSContinuationFragment), build(enums.VoiceBurstE, enums.LCSSLastFragment)
	var tr layer2.PositionTracker
	h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeVoiceLCHeader, HasDataType: true, Seq: 0, HasSeq: true}
	var hdr layer2.RawBurst
	tr.Place(&hdr, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 4, HasSeq: true}
	tr.Place(&c, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 5, HasSeq: true}
	tr.Place(&d, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 6, HasSeq: true}
	if lost, _ := tr.Place(&e, &h); h.VoiceBurst != enums.VoiceBurstE || lost != 0 {
		t.Fatalf("E after a lost PI Header, A and B: %v lost %d", h.VoiceBurst, lost)
	}
}

func TestPositionTrackerSequenceLostAfterGuess(t *testing.T) {
	t.Parallel()

	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	build := func(pos enums.VoiceBurstType, lcss enums.LCSS) layer2.RawBurst {
		r, err := layer2.BuildVoiceBurst(frames, pos, pdu.EmbeddedSignalling{ColorCode: 1, LCSS: lcss}, [4]byte{}, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	c, b := build(enums.VoiceBurstC, enums.LCSSContinuationFragment), build(enums.VoiceBurstB, enums.LCSSFirstFragment)
	var tr layer2.PositionTracker
	h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeVoiceLCHeader, HasDataType: true, Seq: 0, HasSeq: true}
	var hdr layer2.RawBurst
	tr.Place(&hdr, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 4, HasSeq: true}
	tr.Place(&c, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, Seq: 7, HasSeq: true}
	if lost, _ := tr.Place(&b, &h); h.VoiceBurst != enums.VoiceBurstB || lost != 2 {
		t.Fatalf("B three sequence numbers after a guessed burst: %v lost %d", h.VoiceBurst, lost)
	}
}

func TestPositionTrackerMisuse(t *testing.T) {
	t.Parallel()

	sf := superframe(t)
	var nilTracker *layer2.PositionTracker
	h := voiceHint(1)
	nilTracker.Place(&sf[0], &h)
	nilTracker.PlaceDecoded(layer2.NewBurst(), &h, 1)
	nilTracker.Header(&h)
	nilTracker.Forget()
	nilTracker.Reset()
	var tr layer2.PositionTracker
	tr.Place(&sf[0], nil)
	tr.Place(nil, nil)
	tr.PlaceDecoded(nil, &h, 1)
	tr.PlaceDecoded(layer2.NewBurst(), nil, 1)
	tr.Header(nil)
	h = layer2.Hint{Kind: layer2.Kind(9), VoiceBurst: 200, SeqBits: 40, HasSeq: true}
	tr.Place(&sf[0], &h)
	h = layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: 200}
	if tr.PlaceDecoded(layer2.NewBurst(), &h, -5); h.VoiceBurst != enums.VoiceBurstUnknown {
		t.Fatalf("zero burst: %v", h.VoiceBurst)
	}
	for _, c := range []struct {
		p    layer2.Placement
		want string
	}{{layer2.PlacementInOrder, "InOrder"}, {layer2.PlacementDuplicate, "Duplicate"}, {layer2.PlacementLate, "Late"}, {9, "Placement(9)"}} {
		if got := c.p.String(); got != c.want {
			t.Errorf("%d: %q, want %q", c.p, got, c.want)
		}
	}
}
