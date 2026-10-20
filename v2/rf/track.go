package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	ibits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

const (
	damagedSyncDistance = 8
	voiceSyncDistance   = 16
	pendingSlots        = 14
	noSlot              = math.MinInt64
)

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

type llr8 [layer2.BurstBits]int8

func (l *llr8) set(v *[layer2.BurstBits]int16) {
	for i, x := range v {
		h := (int32(x) + 1) >> 1
		if x < 0 {
			h = -((1 - int32(x)) >> 1)
		}
		l[i] = int8(max(min(h, math.MaxInt8), -math.MaxInt8))
	}
}

func (l *llr8) get(v *[layer2.BurstBits]int16) {
	for i, q := range l {
		v[i] = int16(q) * 2
	}
}

type trackEntry struct {
	rec      slotRec
	out      received
	resolved bool
	keep     bool
	hold     bool
	elcOK    bool
}

type chanTrack struct {
	sf   int64
	hdr  int64
	acc  [4][embBits]int16
	accN int
	raw  bool
}

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

func newTracker() *tracker {
	t := &tracker{q: make([]trackEntry, pendingSlots+2)}
	t.reset(false)
	return t
}

func (t *tracker) reset(bs bool) {
	t.head, t.n = 0, 0
	t.bs = bs
	for i := range t.ch {
		t.ch[i].sf, t.ch[i].hdr, t.ch[i].accN, t.ch[i].raw = noSlot, noSlot, 0, false
	}
	t.votes = [16]int{}
}

func (t *tracker) at(i int) *trackEntry { return &t.q[(t.head+i)%len(t.q)] }

func (t *tracker) find(index int64) *trackEntry {
	for i := range t.n {
		if e := t.at(i); e.rec.index == index {
			return e
		}
	}
	return nil
}

func (t *tracker) colorCode() (enums.ColorCode, bool) {
	best := 0
	for i, v := range &t.votes {
		if v > t.votes[best] {
			best = i
		}
	}
	return enums.ColorCode(best), t.votes[best] > 0
}

func (t *tracker) add(rec *slotRec, emit func(*received)) {
	if t.n == len(t.q) {
		t.pop(emit)
	}
	e := t.at(t.n)
	t.n++
	*e = trackEntry{rec: *rec}
	e.out = received{data: rec.data, seconds: rec.seconds, distance: -1, bs: rec.bs, cach: rec.cach, hasCACH: rec.bs,
		stream: rec.stream, slot: rec.index}
	t.classify(e)
	for t.n > 0 && (t.at(0).resolved && !t.at(0).hold || rec.index-t.at(0).rec.index > pendingSlots) {
		t.pop(emit)
	}
}

func (t *tracker) pop(emit func(*received)) {
	e := t.at(0)
	if !e.resolved {
		t.resolveLeftover(e)
	}
	if e.keep {
		cc, ok := t.colorCode()
		if t.repair != nil {
			t.repair(e, cc, ok)
		}
		emit(&e.out)
	}
	t.head = (t.head + 1) % len(t.q)
	t.n--
}

func (t *tracker) flush(emit func(*received)) {
	for t.n > 0 {
		t.pop(emit)
	}
}

func (t *tracker) resolveLeftover(e *trackEntry) {
	_, data, _ := e.rec.src.SyncPatterns()
	if d := syncDist(e.rec.data, data); d <= damagedSyncDistance {
		e.out.distance, e.out.sync, e.keep = d, data, true
	}
	e.resolved = true
}

func syncDist(b layer2.RawBurst, p enums.SyncPattern) int {
	return bits.OnesCount64(ibits.Field(b[:], framer.SyncOffset, framer.SyncBits) ^ uint64(p))
}

func (t *tracker) setA(e *trackEntry, c *chanTrack, d int, timed bool) {
	voice, _, _ := e.rec.src.SyncPatterns()
	e.out.distance, e.out.sync = d, voice
	if timed {
		e.out.voice = enums.VoiceBurstA
	}
	e.resolved, e.keep = true, true
	c.sf = e.rec.index
}

func (t *tracker) classify(e *trackEntry) {
	idx := e.rec.index
	c := &t.ch[((idx%2)+2)%2]
	voice, data, _ := e.rec.src.SyncPatterns()
	dv, dd := syncDist(e.rec.data, voice), syncDist(e.rec.data, data)
	switch {
	case e.rec.anchor == data || e.rec.anchor == 0 && dd <= damagedSyncDistance && dd < dv:
		e.out.distance, e.out.sync = dd, data
		e.resolved, e.keep = true, true
		c.sf, c.accN = noSlot, 0
		t.noteData(e, c)
		return
	case e.rec.anchor == enums.SyncMSSourcedRC:
		e.out.distance, e.out.sync = syncDist(e.rec.data, enums.SyncMSSourcedRC), enums.SyncMSSourcedRC
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

func (t *tracker) noteData(e *trackEntry, c *chanTrack) {
	st := e.out.data.SlotType()
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

func (t *tracker) embFits(e *trackEntry, pos enums.VoiceBurstType) bool {
	cc, ok := t.colorCode()
	emb := e.rec.data.EMB()
	want, _ := pos.LCSS()
	return ok && !emb.FEC.Uncorrectable && emb.FEC.ErrorsCorrected <= 1 && emb.ColorCode == cc && emb.LCSS == want
}

func (t *tracker) place(e *trackEntry, c *chanTrack, pos enums.VoiceBurstType) {
	e.out.voice = pos
	e.resolved, e.keep = true, true
	e.hold = pos >= enums.VoiceBurstB && pos < enums.VoiceBurstE
	if pos != enums.VoiceBurstE {
		return
	}
	es := &t.es
	for i := range es {
		es[i] = t.find(e.rec.index - int64(2*(3-i)))
		if es[i] != nil && es[i].out.voice != enums.VoiceBurstB+enums.VoiceBurstType(i) {
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

func (t *tracker) lateEntry(e *trackEntry, c *chanTrack) {
	cc, ok := t.colorCode()
	if !ok {
		return
	}
	emb := e.out.data.EMB()
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
