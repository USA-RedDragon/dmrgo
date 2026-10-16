package main

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

// Limits of the superframe tracker.
const (
	// damagedSyncDistance is the largest SYNC distance of a burst A or a
	// data burst that only the slot timing finds.
	damagedSyncDistance = 8
	// voiceSyncDistance is the largest SYNC distance of a burst A at a slot
	// where the timing of the call expects one.
	voiceSyncDistance = 16
	// pendingSlots is how many slots a burst without a SYNC waits for a
	// later burst to place it in a superframe.
	pendingSlots = 14
	// noSlot marks an unset slot number.
	noSlot = math.MinInt64
)

// slotRec is a detected slot that the tracker places.
type slotRec struct {
	index   int64
	seconds float64
	bs      bool
	src     layer2.SyncSource
	data    layer2.RawBurst
	llr     llr8
	cachLLR [layer2.CACHBits]int16
	cach    [layer2.CACHBits]unpacked.Bit
	anchor  enums.SyncPattern
	stream  int
}

// llr8 holds the log-likelihood ratios of a burst as int8 multiples of 1/4,
// saturated at about 32: the soft decoders take ratios in units of 1/8, and
// at 1/4 with that limit the sweep of every tier stays the same, at half
// the bytes.
type llr8 [layer2.BurstBits]int8

// set stores the ratios of v.
func (l *llr8) set(v *[layer2.BurstBits]int16) {
	for i, x := range v {
		l[i] = int8(max(min((int32(x)+1)>>1, math.MaxInt8), math.MinInt8))
	}
}

// get writes the stored ratios into v.
func (l *llr8) get(v *[layer2.BurstBits]int16) {
	for i, q := range l {
		v[i] = int16(q) * 2
	}
}

// trackEntry is a slot waiting in the tracker's queue.
type trackEntry struct {
	rec      slotRec
	out      Received
	resolved bool
	keep     bool
	// hold keeps a burst B to D of a superframe until its burst E, so that
	// the embedded LC decode can correct the fragments of all four, and
	// elcOK marks the fragment of a decoded embedded LC.
	hold  bool
	elcOK bool
}

// chanTrack is the superframe state of one TDMA channel.
type chanTrack struct {
	sf  int64
	hdr int64
	// acc sums the log-likelihood ratios of the embedded LC fragments of
	// the superframes of the current call, which all carry the same LC.
	acc  [4][embBits]int16
	accN int
}

// tracker places the bursts of a stream: data bursts and bursts A by their
// SYNC, bursts B to F by the slot timing after a burst A, and bursts A
// whose SYNC is damaged where the timing of the call expects one (after a
// voice LC or PI header, every 360 ms in a call, or before a burst B or E
// whose EMB decodes clean). It emits them in time order.
type tracker struct {
	q      []trackEntry
	head   int
	n      int
	ch     [2]chanTrack
	bs     bool
	votes  [16]int
	repair func(*trackEntry, enums.ColorCode, bool)
	elc    func(*chanTrack, *[4]*trackEntry)
	es     [4]*trackEntry
}

// newTracker returns a tracker with an empty queue. The slots of a stream
// arrive in order, and add pops every entry more than pendingSlots slots
// older than the newest, so the queue holds at most pendingSlots+2.
func newTracker() *tracker {
	t := &tracker{q: make([]trackEntry, pendingSlots+2)}
	t.reset(false)
	return t
}

// reset empties the queue and the superframe state for a new stream, BS
// sourced when bs.
func (t *tracker) reset(bs bool) {
	t.head, t.n = 0, 0
	t.bs = bs
	for i := range t.ch {
		t.ch[i].sf, t.ch[i].hdr, t.ch[i].accN = noSlot, noSlot, 0
	}
	t.votes = [16]int{}
}

// at returns queued entry i, the oldest first.
func (t *tracker) at(i int) *trackEntry { return &t.q[(t.head+i)%len(t.q)] }

// find returns the queued entry of slot index, or nil.
func (t *tracker) find(index int64) *trackEntry {
	for i := range t.n {
		if e := t.at(i); e.rec.index == index {
			return e
		}
	}
	return nil
}

// colorCode returns the color code most clean data bursts carried.
func (t *tracker) colorCode() (enums.ColorCode, bool) {
	best := 0
	for i, v := range t.votes {
		if v > t.votes[best] {
			best = i
		}
	}
	return enums.ColorCode(best), t.votes[best] > 0
}

// add places one slot and emits the bursts that are resolved.
func (t *tracker) add(rec *slotRec, emit func(Received)) {
	if t.n == len(t.q) {
		t.pop(emit)
	}
	e := t.at(t.n)
	t.n++
	*e = trackEntry{rec: *rec}
	e.out = Received{Data: rec.data, Seconds: rec.seconds, Distance: -1, BS: rec.bs, CACH: rec.cach, HasCACH: rec.bs,
		Stream: rec.stream, Slot: rec.index}
	t.classify(e)
	for t.n > 0 && (t.at(0).resolved && !t.at(0).hold || rec.index-t.at(0).rec.index > pendingSlots) {
		t.pop(emit)
	}
}

// pop removes the oldest entry and emits its burst when it is kept,
// repaired first.
func (t *tracker) pop(emit func(Received)) {
	e := t.at(0)
	if !e.resolved {
		t.resolveLeftover(e)
	}
	if e.keep {
		cc, ok := t.colorCode()
		if t.repair != nil {
			t.repair(e, cc, ok)
		}
		emit(e.out)
	}
	t.head = (t.head + 1) % len(t.q)
	t.n--
}

// flush emits every queued burst.
func (t *tracker) flush(emit func(Received)) {
	for t.n > 0 {
		t.pop(emit)
	}
}

// resolveLeftover keeps a burst that nothing placed only as a data burst
// within damagedSyncDistance of the data SYNC.
func (t *tracker) resolveLeftover(e *trackEntry) {
	_, data, _ := e.rec.src.SyncPatterns()
	if d := syncDist(e.rec.data, data); d <= damagedSyncDistance {
		e.out.Distance, e.out.Sync, e.keep = d, data, true
	}
	e.resolved = true
}

// syncDist returns the Hamming distance of the SYNC field of b from p.
func syncDist(b layer2.RawBurst, p enums.SyncPattern) int {
	var v uint64
	for i := framer.SyncOffset; i < framer.SyncOffset+framer.SyncBits; i++ {
		v = v<<1 | uint64(b[i/8]>>(7-i%8)&1)
	}
	return bits.OnesCount64(v ^ uint64(p))
}

// setA makes entry e a burst A at SYNC distance d and starts a superframe.
func (t *tracker) setA(e *trackEntry, c *chanTrack, d int, timed bool) {
	voice, _, _ := e.rec.src.SyncPatterns()
	e.out.Distance, e.out.Sync = d, voice
	if timed {
		e.out.Voice = enums.VoiceBurstA
	}
	e.resolved, e.keep = true, true
	c.sf = e.rec.index
}

// classify places entry e, the newest slot.
func (t *tracker) classify(e *trackEntry) {
	idx := e.rec.index
	c := &t.ch[((idx%2)+2)%2]
	voice, data, _ := e.rec.src.SyncPatterns()
	dv, dd := syncDist(e.rec.data, voice), syncDist(e.rec.data, data)
	switch {
	case e.rec.anchor == data || e.rec.anchor == 0 && dd <= damagedSyncDistance && dd < dv:
		e.out.Distance, e.out.Sync = dd, data
		e.resolved, e.keep = true, true
		c.sf, c.accN = noSlot, 0
		t.noteData(e, c)
		return
	case e.rec.anchor == enums.SyncMSSourcedRC:
		e.out.Distance, e.out.Sync = syncDist(e.rec.data, enums.SyncMSSourcedRC), enums.SyncMSSourcedRC
		e.resolved, e.keep = true, true
		return
	case e.rec.anchor == voice || dv <= layer2.DefaultSyncThreshold:
		t.setA(e, c, dv, false)
		t.backfill(e, c)
		return
	}
	if c.sf != noSlot {
		switch n := (idx - c.sf) / 2; {
		case n >= 1 && n <= 5 && (idx-c.sf)%2 == 0:
			t.place(e, c, enums.VoiceBurstA+enums.VoiceBurstType(n))
			return
		case n == 6 && dv <= voiceSyncDistance:
			t.setA(e, c, dv, true)
			return
		}
		c.sf = noSlot
	}
	if c.hdr == idx-2 && dv <= voiceSyncDistance || dv <= damagedSyncDistance {
		t.setA(e, c, dv, true)
		return
	}
	t.lateEntry(e, c)
}

// noteData records the color code of a clean data burst and the slot of a
// voice LC or PI header.
func (t *tracker) noteData(e *trackEntry, c *chanTrack) {
	st := e.out.Data.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	if st.FEC.ErrorsCorrected == 0 {
		t.votes[st.ColorCode&0xF]++
	}
	if dt := st.DataType; dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader {
		c.hdr = e.rec.index
	}
}

// backfill places the queued slots of the superframe before the burst A in
// e: the slot 360 ms before e becomes a burst A when it can be one, and the
// queued slots between them become bursts B to F. When that burst A was not
// received, a slot becomes a burst B to F only when its EMB fits.
func (t *tracker) backfill(e *trackEntry, c *chanTrack) {
	a := e.rec.index - 2*layer2.VoiceBurstsPerSuperframe
	saved := c.sf
	check := true
	if prev := t.find(a); prev != nil && !prev.resolved {
		voice, _, _ := prev.rec.src.SyncPatterns()
		if d := syncDist(prev.rec.data, voice); d <= voiceSyncDistance {
			t.setA(prev, c, d, true)
			check = false
		}
	}
	t.fillFrom(a, c, check)
	c.sf = saved
}

// fillFrom places the queued slots B to F of the superframe whose burst A
// is at slot a, each only when its EMB fits its position if check.
func (t *tracker) fillFrom(a int64, c *chanTrack, check bool) {
	for n := 1; n < layer2.VoiceBurstsPerSuperframe; n++ {
		x := t.find(a + int64(2*n))
		if x == nil || x.resolved {
			continue
		}
		_, data, _ := x.rec.src.SyncPatterns()
		if syncDist(x.rec.data, data) <= damagedSyncDistance {
			return
		}
		pos := enums.VoiceBurstA + enums.VoiceBurstType(n)
		if check && !t.embFits(x, pos) {
			continue
		}
		t.place(x, c, pos)
	}
}

// embFits reports whether the EMB of entry e decodes with at most one
// correction, the color code of the stream and the LCSS of position pos.
func (t *tracker) embFits(e *trackEntry, pos enums.VoiceBurstType) bool {
	cc, ok := t.colorCode()
	emb := e.rec.data.EMB()
	want, _ := pos.LCSS()
	return ok && !emb.FEC.Uncorrectable && emb.FEC.ErrorsCorrected <= 1 && emb.ColorCode == cc && emb.LCSS == want
}

// place makes entry e the voice burst at pos. Bursts B to D wait for the
// burst E of their superframe, which decodes the embedded LC of all four.
func (t *tracker) place(e *trackEntry, c *chanTrack, pos enums.VoiceBurstType) {
	e.out.Voice = pos
	e.resolved, e.keep = true, true
	e.hold = pos >= enums.VoiceBurstB && pos < enums.VoiceBurstE
	if pos != enums.VoiceBurstE {
		return
	}
	es := &t.es
	for i := range es {
		es[i] = t.find(e.rec.index - int64(2*(3-i)))
		if es[i] != nil && es[i].out.Voice != enums.VoiceBurstB+enums.VoiceBurstType(i) {
			es[i] = nil
		}
	}
	if t.elc != nil && es[0] != nil && es[1] != nil && es[2] != nil {
		t.elc(c, es)
	}
	for _, x := range es {
		if x != nil {
			x.hold = false
		}
	}
}

// lateEntry places a slot by its EMB: a burst B or E whose EMB decodes with
// at most one correction and the color code of the stream starts the
// superframe one or four frames before it, when the slot there can be a
// burst A. Otherwise the slot waits.
func (t *tracker) lateEntry(e *trackEntry, c *chanTrack) {
	cc, ok := t.colorCode()
	if !ok {
		return
	}
	emb := e.out.Data.EMB()
	if emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > 1 || emb.ColorCode != cc {
		return
	}
	back := 0
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		back = 1
	case enums.LCSSLastFragment:
		back = 4
	case enums.LCSSSingleFragment, enums.LCSSContinuationFragment:
		return
	}
	a := t.find(e.rec.index - int64(2*back))
	if a == nil || a.resolved {
		return
	}
	voice, _, _ := a.rec.src.SyncPatterns()
	d := syncDist(a.rec.data, voice)
	if d > voiceSyncDistance {
		return
	}
	t.setA(a, c, d, true)
	t.fillFrom(a.rec.index, c, false)
	c.sf = a.rec.index
}
