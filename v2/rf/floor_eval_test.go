package rf

import (
	"math"
	"math/bits"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const floorMargin = 2

type floorEval struct {
	f          *floorSignal
	g          *genieTrellis
	g1         *genieTrellis
	g2         *genieTrellis
	dec        *softDecoder
	tail       bool
	slots      []rftx.BSSlot
	truth      []benchSlot
	app        []float32
	llr        [softBurstBits]int16
	free       [softBurstBits]bool
	ch         []floorChange
	sc         map[string]*benchScore
	conv       map[string][]float64
	r1pos      [192]int16
	elc        map[int]*[4][softBurstBits]int16
	elcI       map[int]*[4]float64
	elcAt      map[int]*[4]int
	only       map[string]bool
	gridIters  int
	gridWeight float32
	aware      bool
	aw         *awareState
	as         *ambeSearch
	ts         *trellisSearch
}

func newFloorEval(d int) *floorEval {
	dec := newSoftDecoder()
	dec.OSDOrder, dec.OSDWindow = 3, 24
	g := newGenieTrellis(d, 2*slotSymbols)
	e := &floorEval{g: g, g1: g, dec: dec, app: make([]float32, 4*2*slotSymbols), tail: true}
	for i := range 96 {
		e.r1pos[i] = softInfoPos[i]
		e.r1pos[96+i] = softInfoPos[100+i]
	}
	return e
}

func floorPayloads() []string {
	return slices.DeleteFunc(benchPayloads(), func(k string) bool { return k == "ambe_u" })
}

func (e *floorEval) want(k string) bool { return e.only == nil || e.only[k] }

func (e *floorEval) add(k string, ok bool) {
	s := e.sc[k]
	if s == nil {
		s = &benchScore{}
		e.sc[k] = s
	}
	s.Truth++
	if ok {
		s.Exact++
	}
}

func (e *floorEval) addInfo(k string, u float64) {
	e.conv[k] = append(e.conv[k], u)
}

func (e *floorEval) span(i int) (n0, n1 int, mask func(int) uint8) {
	d := e.g.d
	lo, hi := -1, -1
	for b, fr := range e.free {
		if fr {
			if lo < 0 {
				lo = b / 2
			}
			hi = b / 2
		}
	}
	base := i*slotSymbols + cachSymbols
	n0 = base + lo - d - floorMargin
	n1 = base + hi + d + 1 + floorMargin
	sym := e.f.sym
	mask = func(n int) uint8 {
		t := digitOf(sym[n])
		k := n - base
		if k < 0 || k >= burstSymbols {
			return 1 << t
		}
		f0, f1 := e.free[2*k], e.free[2*k+1]
		if !f0 && !f1 {
			return 1 << t
		}
		t0, t1 := dibitOfDigit(t)
		var m uint8
		for v := range 4 {
			b0, b1 := dibitOfDigit(v)
			if (f0 || b0 == t0) && (f1 || b1 == t1) {
				m |= 1 << v
			}
		}
		return m
	}
	return n0, n1, mask
}

func (e *floorEval) detect(i int, maxlog bool) float64 {
	d := e.g.d
	n0, n1, mask := e.span(i)
	e.g.run(e.f, n0, n1, mask, maxlog, e.tail, nil, e.app)
	e.llr = [softBurstBits]int16{}
	base := i*slotSymbols + cachSymbols
	for k := range burstSymbols {
		if !e.free[2*k] && !e.free[2*k+1] {
			continue
		}
		j := base + k - n0 - d
		a := e.app[4*j : 4*j+4]
		var l0, l1 int16
		if maxlog {
			l0, l1 = dibitLLR(a, 1)
		} else {
			l0 = satLLR(floorLogAdd(a[2], a[3]) - floorLogAdd(a[0], a[1]))
			l1 = satLLR(floorLogAdd(a[1], a[2]) - floorLogAdd(a[0], a[3]))
		}
		if e.free[2*k] {
			e.llr[2*k] = l0
		}
		if e.free[2*k+1] {
			e.llr[2*k+1] = l1
		}
	}
	if maxlog {
		return 0
	}
	var la float64
	for n := n0 + d; n < n1+d; n++ {
		la += math.Log(float64(bits.OnesCount8(mask(n))))
	}
	sps := samplesPerSy
	num := e.f.scale * real(e.f.zp[e.f.center(n1)-sps/2]-e.f.zp[e.f.center(n0)-sps/2])
	return num - e.g.logZ + la
}

func (e *floorEval) setFree(pos ...[]int16) {
	e.free = [softBurstBits]bool{}
	for _, ps := range pos {
		for _, p := range ps {
			e.free[p] = true
		}
	}
}

func (e *floorEval) beats(i int, c *layer2.RawBurst) bool {
	e.ch = e.f.burstChanges(e.ch[:0], i, c)
	return len(e.ch) > 0 && e.f.delta(e.ch, e.tail) > 0
}

func smallCodeword(c *softSmallCode, info uint32) uint32 {
	cw := c.base
	for i := range c.k {
		if info>>i&1 != 0 {
			cw ^= c.rows[i]
		}
	}
	return cw
}

func (e *floorEval) smallBeats(i int, id softSmallID, fixed, want uint32) bool {
	c := &softSmallCodes[id]
	tb := e.truth[i].burst
	for info := range uint32(1) << c.k {
		if info&fixed != want {
			continue
		}
		cand := tb
		softPutSmall(&cand, id, smallCodeword(c, info))
		if e.beats(i, &cand) {
			return true
		}
	}
	return false
}

func (e *floorEval) stream() {
	if e.aware {
		e.awareStream()
		return
	}
	e.sc = map[string]*benchScore{}
	e.conv = map[string][]float64{}
	e.elc = map[int]*[4][softBurstBits]int16{}
	e.elcI = map[int]*[4]float64{}
	e.elcAt = map[int]*[4]int{}
	cc := enums.ColorCode(0)
	for _, ts := range e.truth {
		if ts.kind == "data" {
			cc = ts.burst.SlotType().ColorCode
			break
		}
	}
	for i, ts := range e.truth {
		switch ts.kind {
		case "voice":
			e.voice(i, cc)
		case "data":
			e.data(i, cc)
		}
	}
}

func otherSync(p enums.SyncPattern) enums.SyncPattern {
	voice, data, _ := layer2.SyncSourceBS.SyncPatterns()
	if p == voice {
		return data
	}
	return voice
}

func (e *floorEval) syncOK(i int, p enums.SyncPattern) bool {
	cand := e.truth[i].burst
	_ = cand.SetSync(otherSync(p))
	return !e.beats(i, &cand)
}

const (
	syncFirstBit = 108
	syncBits     = 48
)

func (e *floorEval) sync(i int, p enums.SyncPattern) {
	e.add("sync", e.syncOK(i, p))
	e.setFree(seqInts(syncFirstBit, syncBits))
	e.addInfo("sync", e.detect(i, false)-math.Ln2)
}

func (e *floorEval) voice(i int, cc enums.ColorCode) {
	ts := e.truth[i]
	voice, _, _ := layer2.SyncSourceBS.SyncPatterns()
	for f := range layer2.VoiceSlotsPerBurst {
		if e.want("ambe") || e.want("ambe_c0c1") {
			e.ambe(i, f)
		}
	}
	if ts.pos == enums.VoiceBurstA {
		if e.want("sync") {
			e.sync(i, voice)
		}
		return
	}
	if e.want("emb") {
		e.add("emb", !e.smallBeats(i, softEMBCode, 0x78, uint32(cc&0x0f)<<3))
		e.setFree(softSmallCodes[softEMBCode].pos[:softSmallCodes[softEMBCode].n])
		e.addInfo("emb", e.detect(i, false)-3*math.Ln2)
	}
	if ts.pos >= enums.VoiceBurstB && ts.pos <= enums.VoiceBurstE && e.want("elc") {
		e.elcFragment(i, ts)
	}
}

func (e *floorEval) ambe(i, f int) {
	sl := softAMBESlot(f)
	tb := e.truth[i].burst
	e.setFree(sl.id0.bits(), sl.id1.bits(), sl.c2[:])
	e.addInfo("ambe", e.detect(i, false)-49*math.Ln2)
	okFrame := true
	e.setFree(sl.c2[:])
	e.detect(i, true)
	cand := tb
	for _, p := range sl.c2 {
		softPut(&cand, p, e.llr[p] < 0)
	}
	if e.beats(i, &cand) {
		okFrame = false
	}
	e.setFree(sl.id0.bits(), sl.id1.bits())
	e.addInfo("ambe_c0c1", e.detect(i, false)-24*math.Ln2)
	if e.want("ambe_c0c1") {
		okC0C1 := !e.c0c1Search(i, f)
		e.add("ambe", okFrame && okC0C1)
		e.add("ambe_c0c1", okC0C1)
		return
	}
	okC0C1 := true
	s := &e.dec.small
	sl.id0.gather(e.llr[:], &s.l)
	softTables(s.tab, s.l[:sl.c0.n])
	s.top.reset(softListMax)
	sl.id0.search(s.tab, 0, 0, 0, &s.top)
	top0 := s.top
	sl.id1.gather(e.llr[:], &s.l)
	softTables(s.tab, s.l[:sl.c1.n])
	for c := range top0.n {
		s.top1.reset(4)
		sl.id1.search(s.tab, softAMBEPN(top0.info[c]), 0, 0, &s.top1)
		for c1 := range s.top1.n {
			cand := tb
			softPutSmall(&cand, sl.id0, top0.cw[c])
			softPutSmall(&cand, sl.id1, s.top1.cw[c1])
			if e.beats(i, &cand) {
				okC0C1, okFrame = false, false
			}
		}
	}
	e.add("ambe", okFrame)
	e.add("ambe_c0c1", okC0C1)
}

func (e *floorEval) elcFragment(i int, ts benchSlot) {
	e.setFree(seqInts(embFirstBit, embBits))
	u := e.detect(i, false)
	b := int(ts.pos - enums.VoiceBurstB)
	l := e.elc[ts.sf]
	if l == nil {
		l = &[4][softBurstBits]int16{}
		e.elc[ts.sf] = l
		e.elcI[ts.sf] = &[4]float64{}
		e.elcAt[ts.sf] = &[4]int{-1, -1, -1, -1}
	}
	l[b] = e.llr
	e.elcI[ts.sf][b] = u
	e.elcAt[ts.sf][b] = i
	if ts.pos != enums.VoiceBurstE {
		return
	}
	var sum [4][softBurstBits]int16
	var sfs []int
	info := -softEmbLCBits * math.Ln2
	for sf := 0; sf <= ts.sf; sf++ {
		at := e.elcAt[sf]
		if at == nil || slices.Contains(at[:], -1) {
			continue
		}
		sfs = append(sfs, sf)
		for k := range sum {
			softCombine(sum[k][:], e.elc[sf][k][:])
			info += e.elcI[sf][k]
		}
	}
	e.addInfo("elc", info)
	at := e.elcAt[ts.sf]
	var tb [4]layer2.RawBurst
	for k := range tb {
		tb[k] = e.truth[at[k]].burst
	}
	d, s := e.dec, &e.dec.osd
	c := softEmbLCCode.code()
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(sum[p/softBurstBits][p%softBurstBits])
	}
	s.accept = softEmbLCAccept
	softEmbLCCode.osd(s, d.OSDOrder, d.OSDWindow, softListMax)
	ok := true
	for x := range s.list.n {
		cw := &s.list.cw[x]
		if !s.accept(cw) {
			continue
		}
		cand := tb
		for j := range c.n {
			p := c.pos[j]
			softPut(&cand[p/softBurstBits], p%softBurstBits, softBit(cw, j))
		}
		if cand == tb {
			continue
		}
		e.ch = e.ch[:0]
		for _, sf := range sfs {
			for k, slot := range e.elcAt[sf] {
				e.ch = e.f.burstChanges(e.ch, slot, &cand[k])
			}
		}
		slices.SortFunc(e.ch, func(a, b floorChange) int { return a.n - b.n })
		if e.f.delta(e.ch, e.tail) > 0 {
			ok = false
			break
		}
	}
	e.add("elc", ok)
}

func seqInts(a, n int) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(a + i)
	}
	return out
}

func (e *floorEval) data(i int, cc enums.ColorCode) {
	ts := e.truth[i]
	_, dsync, _ := layer2.SyncSourceBS.SyncPatterns()
	k := benchDataKey(ts.dt)
	okST := !e.smallBeats(i, softSlotTypeCode, 0xf0, uint32(cc&0x0f)<<4)
	if e.want("slottype") {
		e.add("slottype", okST)
		e.setFree(softSmallCodes[softSlotTypeCode].pos[:softSmallCodes[softSlotTypeCode].n])
		e.addInfo("slottype", e.detect(i, false)-4*math.Ln2)
	}
	if e.want("sync") {
		e.sync(i, dsync)
	}
	if k == "" || !e.want(k) && (k != "rate1" || !e.want("rate1_bit")) {
		return
	}
	var ok bool
	switch ts.dt {
	case enums.DataTypeRate34:
		ok = e.rate34(i)
	case enums.DataTypeRate1:
		ok = e.rate1(i)
	default:
		ok = e.bptc(i, ts.dt)
	}
	e.add(k, ok)
	e.add(k+"/st", okST)
}

func bptcMessageBits(dt enums.DataType) int {
	if dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypeTerminatorWithLC {
		return 72
	}
	if dt == enums.DataTypeCSBK || dt == enums.DataTypeDataHeader {
		return 80
	}
	return softBPTCData
}

func (e *floorEval) bptc(i int, dt enums.DataType) bool {
	check := softBPTCCheck(dt)
	d, s := e.dec, &e.dec.osd
	tb := e.truth[i].burst
	want, _ := tb.BPTCPayload()
	var free, rpos []int16
	for j := range softBPTCCode.code().n {
		r := false
		for x := softBPTCData; x < softBPTCInfo; x++ {
			r = r || int(softBPTCCode.code().sys[x]) == j
		}
		if r {
			rpos = append(rpos, softBPTCCode.code().pos[j])
		} else {
			free = append(free, softBPTCCode.code().pos[j])
		}
	}
	e.setFree(free)
	e.addInfo(benchDataKey(dt), e.detect(i, false)-float64(bptcMessageBits(dt))*math.Ln2)
	for _, p := range rpos {
		if tb[p/8]&(0x80>>(p%8)) != 0 {
			e.llr[p] = -32000
		} else {
			e.llr[p] = 32000
		}
	}
	d.bptcLLRs(&e.llr)
	d.check = check
	s.accept = nil
	if check != nil {
		s.accept = d.acceptBPTC
	}
	softBPTCCode.osd(s, d.OSDOrder, d.OSDWindow, softListMax)
	for x := range s.list.n {
		cw := &s.list.cw[x]
		if check != nil && !check(softBPTCOf(cw).Payload) {
			continue
		}
		cand := tb
		for j := range softBPTCCode.code().n {
			softPut(&cand, softBPTCCode.code().pos[j], softBit(cw, j))
		}
		if !sameBits(&cand, &tb, rpos) {
			continue
		}
		if got, _ := cand.BPTCPayload(); got == want {
			continue
		}
		if e.beats(i, &cand) {
			return false
		}
	}
	return true
}

func sameBits(a, b *layer2.RawBurst, pos []int16) bool {
	for _, p := range pos {
		m := byte(0x80) >> (p % 8)
		if a[p/8]&m != b[p/8]&m {
			return false
		}
	}
	return true
}

func (e *floorEval) rate34(i int) bool {
	tb := e.truth[i].burst
	want, _ := tb.Rate34Payload()
	e.setFree(softTrellisTables.pos[:])
	e.addInfo("rate34", e.detect(i, false)-144*math.Ln2)
	cand := tb
	e.dec.decodeRate34(&e.llr, &cand)
	if got, _ := cand.Rate34Payload(); got != want && e.beats(i, &cand) {
		return false
	}
	return !e.rate34Search(i)
}

func (e *floorEval) rate1(i int) bool {
	if e.g2 == nil {
		e.g2 = newGenieTrellis(2, 2*slotSymbols)
	}
	e.g = e.g2
	defer func() { e.g = e.g1 }()
	tb := e.truth[i].burst
	e.setFree(e.r1pos[:])
	u := e.detect(i, false) - 192*math.Ln2
	e.addInfo("rate1", u)
	e.addInfo("rate1_bit", u)
	for _, p := range e.r1pos {
		one := tb[p/8]&(0x80>>(p%8)) != 0
		e.add("rate1_bit", (e.llr[p] < 0) == one)
	}
	e.detect(i, true)
	cand := tb
	for _, p := range e.r1pos {
		softPut(&cand, p, e.llr[p] < 0)
	}
	return !e.beats(i, &cand)
}

func fanoBound(u []float64, k int) float64 {
	var mean float64
	for _, v := range u {
		mean += v
	}
	mean = mean/float64(len(u))/math.Ln2 + float64(k)
	h := 1 - mean/float64(k)
	if h <= 0 {
		return 0
	}
	lo, hi := 0.0, 0.5
	for range 60 {
		m := (lo + hi) / 2
		if -m*math.Log2(m)-(1-m)*math.Log2(1-m) < h {
			lo = m
		} else {
			hi = m
		}
	}
	return lo
}

func convBound(u []float64) float64 {
	v := slices.Clone(u)
	slices.Sort(v)
	var best float64
	for k, t := range v {
		best = max(best, float64(k+1)/float64(len(v))-math.Exp(t))
	}
	return best
}
