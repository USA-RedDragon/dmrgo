package layer2_test

import (
	"testing"
	"time"

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

type lostStep struct {
	pos   int
	seq   uint16
	fixed bool
	at    time.Duration
	lost  int
}

const (
	stepHeader   = -1
	stepSyncLost = -2
	stepForget   = -3
	noCheck      = -1
	ms           = time.Millisecond
)

func voiceRun(n int, seq uint16, at, jitter time.Duration) []lostStep {
	out := make([]lostStep, n)
	for i := range out {
		j := jitter
		if i%2 == 1 {
			j = -jitter
		}
		out[i] = lostStep{pos: i % 6, seq: seq + uint16(i), fixed: true, lost: 0}
		if at != 0 {
			out[i].at = at + time.Duration(i)*60*ms + j
		}
	}
	return out
}

func runLostSteps(t *testing.T, name string, steps []lostStep) {
	t.Helper()
	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	lcss := [layer2.VoiceBurstsPerSuperframe]enums.LCSS{0, enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment, enums.LCSSSingleFragment}
	var sf [layer2.VoiceBurstsPerSuperframe]layer2.RawBurst
	for i := range sf {
		pos, _ := enums.VoiceBurstAt(i)
		r, err := layer2.BuildVoiceBurst(frames, pos, pdu.EmbeddedSignalling{ColorCode: 1, LCSS: lcss[i]}, [4]byte{}, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		sf[i] = r
	}
	var tr layer2.PositionTracker
	for k, s := range steps {
		switch s.pos {
		case stepHeader:
			h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeVoiceLCHeader, HasDataType: true, Seq: s.seq, HasSeq: true, At: s.at}
			var hdr layer2.RawBurst
			tr.Place(&hdr, &h)
			continue
		case stepSyncLost:
			h := layer2.Hint{Seq: s.seq, HasSeq: true, At: s.at, SyncLost: true}
			tr.Place(nil, &h)
			continue
		case stepForget:
			tr.Forget()
			continue
		}
		h := layer2.Hint{Kind: layer2.KindVoice, Seq: s.seq, HasSeq: true, At: s.at}
		if s.fixed {
			h.VoiceBurst, _ = enums.VoiceBurstAt(s.pos)
		}
		raw := sf[s.pos]
		lost, _ := tr.Place(&raw, &h)
		if i, ok := h.VoiceBurst.Index(); !ok || i != s.pos {
			t.Fatalf("%s: step %d at %v, want %d", name, k, h.VoiceBurst, s.pos)
		}
		if s.lost != noCheck && lost != s.lost {
			t.Errorf("%s: step %d lost %d, want %d", name, k, lost, s.lost)
		}
	}
}

func TestPositionTrackerLostBySequence(t *testing.T) {
	t.Parallel()

	hdr := lostStep{pos: stepHeader}
	for _, c := range []struct {
		name  string
		steps []lostStep
	}{
		{"six lost, skipped A, B by its EMB", []lostStep{hdr, {1, 7, false, 0, 7}}},
		{"skipped A, B by its EMB", []lostStep{hdr, {1, 1, false, 0, 1}}},
		{"late entry header, E given", []lostStep{hdr, {4, 1, true, 0, 0}, {5, 2, true, 0, 0}}},
		{"late entry header, E lost, F given", []lostStep{hdr, {5, 2, true, 0, 1}}},
		{"A lost, B skipped, C given", []lostStep{hdr, {2, 2, true, 0, 2}}},
		{"two lost headers, A given", []lostStep{hdr, {0, 3, true, 0, 0}}},
		{"skipped C and D, E given", []lostStep{{0, 10, true, 0, 0}, {1, 11, false, 0, 0}, {4, 12, true, 0, 2}}},
		{"skipped A, B by its EMB, in the call", append(voiceRun(6, 10, 0, 0), lostStep{1, 16, false, 0, 1})},
		{"one counter for both slots", []lostStep{{0, 0, true, 0, 0}, {1, 2, true, 0, 0}, {2, 4, true, 0, 0}, {3, 6, true, 0, 0}, {4, 8, true, 0, 0}, {5, 10, true, 0, 0}, {0, 12, true, 0, 0}}},
		{"sync lost without times", append(voiceRun(7, 10, 0, 0), lostStep{pos: stepSyncLost, seq: 17}, lostStep{0, 18, true, 0, 1})},
	} {
		runLostSteps(t, c.name, c.steps)
	}
}

func stallDrop() []lostStep {
	at := 1000 * ms
	out := voiceRun(7, 10, at, 0)
	for i := range 10 {
		out = append(out, lostStep{(7 + i) % 6, uint16(17 + i), true, at + 2360*ms + time.Duration(i)*ms, 0})
	}
	return append(out, lostStep{40 % 6, 50, true, at + 2400*ms, 23})
}

func TestPositionTrackerLostByTime(t *testing.T) {
	t.Parallel()

	at := 1000 * ms
	call := func(jitter time.Duration) []lostStep { return voiceRun(7, 10, at, jitter) }
	syncLoss := func(jitter time.Duration, want int) []lostStep {
		return append(call(0), lostStep{pos: stepSyncLost, seq: 17, at: at + 420*ms},
			lostStep{0, 18, true, at + 360*ms + 12*60*ms + jitter, want}, lostStep{1, 19, true, at + 360*ms + 13*60*ms + jitter, 0})
	}
	shared := func(jitter time.Duration) []lostStep {
		out := voiceRun(12, 0, at, jitter)
		for i := range out {
			out[i].seq = uint16(2 * i)
		}
		return out
	}
	stall := func(stall time.Duration, lostOne bool) []lostStep {
		out := call(0)
		next := lostStep{1, 17, true, at + 420*ms + stall, 0}
		if lostOne {
			next = lostStep{2, 18, true, at + 480*ms + stall, 1}
		}
		out = append(out, next)
		for i := 1; i < 4; i++ {
			out = append(out, lostStep{(next.pos + i) % 6, next.seq + uint16(i), true, next.at + time.Duration(i)*ms, 0})
		}
		return out
	}
	for _, c := range []struct {
		name  string
		steps []lostStep
	}{
		{"call, 30 ms jitter", call(30 * ms)},
		{"call, 74 ms jitter", call(74 * ms)},
		{"one counter for both slots, 30 ms jitter", shared(30 * ms)},
		{"one counter for both slots, 74 ms jitter", shared(74 * ms)},
		{"sync lost, then A", syncLoss(0, 11)},
		{"sync lost, then A 150 ms early", syncLoss(-150*ms, 11)},
		{"sync lost, then A 209 ms late", syncLoss(209*ms, 11)},
		{"sync lost, then A 151 ms early, past the tolerance", syncLoss(-151*ms, 5)},
		{"sync lost, then A 210 ms late, past the tolerance", syncLoss(210*ms, 17)},
		{"stall of 200 ms, then bunched", stall(200*ms, false)},
		{"stall of 500 ms, then bunched", stall(500*ms, false)},
		{"stall of 500 ms with a burst lost, then bunched", stall(500*ms, true)},
		{"late entry header with E at once", []lostStep{{stepHeader, 0, false, at, 0}, {4, 1, true, at, 0}, {5, 2, true, at + 60*ms, 0}}},
		{"skipped A, B one period after the header", []lostStep{{stepHeader, 0, false, at, 0}, {1, 1, false, at + 60*ms, 1}}},
		{"A lost, 30 ms jitter", []lostStep{{stepHeader, 0, false, at, 0}, {1, 2, true, at + 120*ms - 30*ms, 1}, {2, 3, true, at + 180*ms + 30*ms, 0}}},
		{"position forgotten, one lost, 20 ms jitter", append(append(call(0)[:6], lostStep{0, 16, true, at + 360*ms + 20*ms, 0}), lostStep{pos: stepForget}, lostStep{2, 18, true, at + 480*ms - 20*ms, 1})},
		{"burst before six lost came 300 ms late", append(voiceRun(6, 10, at, 0), lostStep{0, 16, true, at + 360*ms + 300*ms, 0}, lostStep{1, 23, true, at + 13*60*ms, 6})},
		{"bunch of seven after 900 ms", append(call(0), lostStep{1, 17, true, at + 1260*ms, 0}, lostStep{3, 25, true, at + 1262*ms, 7})},
		{"two second stall, bunched, then 23 lost", stallDrop()},
		{"second sync loss before A", append(call(0), lostStep{pos: stepSyncLost, seq: 17, at: at + 420*ms},
			lostStep{2, 18, true, at + 480*ms, 1}, lostStep{pos: stepSyncLost, seq: 19, at: at + 540*ms},
			lostStep{0, 20, true, at + 360*ms + 12*60*ms, 9})},
	} {
		runLostSteps(t, c.name, c.steps)
	}
}

func TestPositionTrackerTimeWithoutSequence(t *testing.T) {
	t.Parallel()

	sf := superframe(t)
	jitter := func(i int) time.Duration {
		d := time.Second + time.Duration(i)*60*ms + 16*ms
		if i%2 == 1 {
			d -= 32 * ms
		}
		return d
	}
	stall := func(i int) time.Duration {
		if i < 2 {
			return time.Second + time.Duration(i)*60*ms
		}
		return time.Second + 620*ms + time.Duration(i)*ms
	}
	for _, c := range []struct {
		name string
		at   func(int) time.Duration
	}{
		{"16 ms jitter", jitter},
		{"500 ms stall, then bunched", stall},
	} {
		var tr layer2.PositionTracker
		for i := range 12 {
			h := layer2.Hint{Kind: layer2.KindVoice, At: c.at(i)}
			if i%6 == 0 || i%6 == 4 {
				h.VoiceBurst, _ = enums.VoiceBurstAt(i % 6)
			}
			raw := sf[i%6]
			lost, _ := tr.Place(&raw, &h)
			if j, ok := h.VoiceBurst.Index(); !ok || j != i%6 || lost != 0 {
				t.Errorf("%s: burst %d at %v lost %d", c.name, i, h.VoiceBurst, lost)
			}
		}
	}
}
