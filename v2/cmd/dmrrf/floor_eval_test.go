package main

import (
	"math"
	"math/bits"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// floorMargin is the number of known symbols that a genie trellis span
// adds on each side of the free symbols, past its window.
const floorMargin = 2

// floorEval computes the floor of one stream. For each scored field a genie
// gives the receiver every transmitted bit outside the field, the timing,
// the carrier and the noise level. It bounds the field's error in two
// ways, both valid for any receiver that knows less: a field is in error
// when some candidate value has a higher likelihood than the transmitted
// one, which maximum likelihood would then pick, and the meta-converse
// bound of the information density of the field (see convBound).
type floorEval struct {
	f     *floorSignal
	g     *genieTrellis
	g1    *genieTrellis
	g2    *genieTrellis
	dec   *softDecoder
	tail  bool
	slots []BSSlot
	truth []benchSlot
	app   []float32
	llr   [softBurstBits]int16
	free  [softBurstBits]bool
	ch    []floorChange
	sc    map[string]*benchScore
	conv  map[string][]float64
	r1pos [192]int16
	elc   map[int]*[4][softBurstBits]int16
	elcI  map[int]*[4]float64
	elcAt map[int]*[4]int
	only  map[string]bool
}

// newFloorEval returns an evaluator with a genie trellis of window d.
func newFloorEval(d int) *floorEval {
	dec, err := newSoftDecoder()
	if err != nil {
		panic(err)
	}
	dec.OSDOrder, dec.OSDWindow = 3, 24
	g := newGenieTrellis(d, 2*slotSymbols)
	e := &floorEval{g: g, g1: g, dec: dec, app: make([]float32, 4*2*slotSymbols), tail: true}
	for i := range 96 {
		e.r1pos[i] = dec.info[i]
		e.r1pos[96+i] = dec.info[100+i]
	}
	return e
}

// floorPayloads lists the payload keys of a floor.
func floorPayloads() []string {
	return benchPayloads()
}

// want reports whether the evaluation scores payload k.
func (e *floorEval) want(k string) bool { return e.only == nil || e.only[k] }

// add counts one truth item of payload k, recovered when ok.
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

// addInfo records the information density u, minus the log of the number
// of field values, of one truth item of payload k.
func (e *floorEval) addInfo(k string, u float64) {
	e.conv[k] = append(e.conv[k], u)
}

// span returns the genie trellis span of the free bits of slot i: symbol
// intervals n0 to n1-1, and the allowed digits of each symbol.
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

// detect runs the genie trellis over the free bits of slot i, every other
// bit known, and sets e.llr of the free bits: max-log when maxlog, exact
// bitwise otherwise. It returns the information density of the free bits
// in nats when not maxlog: the exact log-likelihood of the transmitted
// bits minus the log of the likelihood averaged over independent uniform
// free bits, as the trellis models it.
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
			l0, l1 = dibitLLR(a)
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

// setFree marks the burst bits at pos as the free bits.
func (e *floorEval) setFree(pos ...[]int16) {
	e.free = [softBurstBits]bool{}
	for _, ps := range pos {
		for _, p := range ps {
			e.free[p] = true
		}
	}
}

// beats reports whether candidate burst c of slot i is more likely than
// the transmitted burst.
func (e *floorEval) beats(i int, c *layer2.RawBurst) bool {
	e.ch = e.f.burstChanges(e.ch[:0], i, c)
	return len(e.ch) > 0 && e.f.delta(e.ch, e.tail) > 0
}

// smallCodeword returns the codeword of info word info of c.
func smallCodeword(c *softSmallCode, info uint32) uint32 {
	cw := c.base
	for i := range c.k {
		if info>>i&1 != 0 {
			cw ^= c.rows[i]
		}
	}
	return cw
}

// smallBeats reports whether a codeword of c with info bits under fixed
// equal to want, other than the transmitted one, makes burst i more likely.
func (e *floorEval) smallBeats(i int, c *softSmallCode, fixed, want uint32) bool {
	tb := e.truth[i].burst
	for info := range uint32(1) << c.k {
		if info&fixed != want {
			continue
		}
		cand := tb
		softPutSmall(&cand, c, smallCodeword(c, info))
		if e.beats(i, &cand) {
			return true
		}
	}
	return false
}

// stream scores every field of the stream in f.
func (e *floorEval) stream() {
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

// otherSync returns the SYNC of the other burst kind of the BS source.
func otherSync(p enums.SyncPattern) enums.SyncPattern {
	voice, data, _ := layer2.SyncSourceBS.SyncPatterns()
	if p == voice {
		return data
	}
	return voice
}

// syncOK reports whether the transmitted SYNC of slot i beats the SYNC of
// the other burst kind.
func (e *floorEval) syncOK(i int, p enums.SyncPattern) bool {
	cand := e.truth[i].burst
	_ = cand.SetSync(otherSync(p))
	return !e.beats(i, &cand)
}

// voice scores the fields of voice burst i.
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
			e.add("sync", e.syncOK(i, voice))
		}
		return
	}
	if e.want("emb") {
		e.add("emb", !e.smallBeats(i, &e.dec.emb, 0x78, uint32(cc&0x0f)<<3))
	}
	if ts.pos >= enums.VoiceBurstB && ts.pos <= enums.VoiceBurstE && e.want("elc") {
		e.elcFragment(i, ts)
	}
}

// ambe scores frame f of voice burst i: the whole frame, and C0 and C1
// with the genie also giving C2 and C3.
func (e *floorEval) ambe(i, f int) {
	sl := &e.dec.ambe.slot[f]
	tb := e.truth[i].burst
	e.setFree(sl.c0.pos[:sl.c0.n], sl.c1.pos[:sl.c1.n], sl.c2[:])
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
	e.setFree(sl.c0.pos[:sl.c0.n], sl.c1.pos[:sl.c1.n])
	e.addInfo("ambe_c0c1", e.detect(i, false)-24*math.Ln2)
	okC0C1 := true
	s := &e.dec.small
	sl.c0.gather(e.llr[:], &s.l)
	softTables(s.tab, s.l[:sl.c0.n])
	s.top.reset(softListMax)
	sl.c0.search(s.tab, 0, 0, 0, &s.top)
	top0 := s.top
	sl.c1.gather(e.llr[:], &s.l)
	softTables(s.tab, s.l[:sl.c1.n])
	for c := range top0.n {
		s.top1.reset(4)
		sl.c1.search(s.tab, e.dec.ambe.pn(top0.info[c]), 0, 0, &s.top1)
		for c1 := range s.top1.n {
			cand := tb
			softPutSmall(&cand, &sl.c0, top0.cw[c])
			softPutSmall(&cand, &sl.c1, s.top1.cw[c1])
			if e.beats(i, &cand) {
				okC0C1, okFrame = false, false
			}
		}
	}
	e.add("ambe", okFrame)
	e.add("ambe_c0c1", okC0C1)
}

// elcFragment stores the genie log-likelihood ratios and information
// density of the embedded LC fragment of voice burst i and, at burst E,
// scores the embedded LC of the superframe with the fragments of every
// superframe of the call so far, as the receiver combines them.
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
	c := d.embLC.code
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(sum[p/softBurstBits][p%softBurstBits])
	}
	s.accept = d.acceptEmbLC
	c.osd(s, d.OSDOrder, d.OSDWindow, softListMax)
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

// seqInts returns n consecutive integers from a.
func seqInts(a, n int) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(a + i)
	}
	return out
}

// data scores the fields of data burst i.
func (e *floorEval) data(i int, cc enums.ColorCode) {
	ts := e.truth[i]
	_, dsync, _ := layer2.SyncSourceBS.SyncPatterns()
	k := benchDataKey(ts.dt)
	okST := !e.smallBeats(i, &e.dec.slotType, 0xf0, uint32(cc&0x0f)<<4)
	if e.want("slottype") {
		e.add("slottype", okST)
	}
	if e.want("sync") {
		e.add("sync", e.syncOK(i, dsync))
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

// bptcMessageBits returns the number of free message bits of a BPTC
// payload of data type dt: 96 less its check bits.
func bptcMessageBits(dt enums.DataType) int {
	if dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypeTerminatorWithLC {
		return 72
	}
	if dt == enums.DataTypeCSBK || dt == enums.DataTypeDataHeader {
		return 80
	}
	return softBPTCData
}

// bptc scores the BPTC(196,96) payload of data burst i of data type dt,
// whose value has to pass the check of dt when it has one. The genie gives
// the reserved bits.
func (e *floorEval) bptc(i int, dt enums.DataType) bool {
	check := softBPTCCheck(dt)
	d, s := e.dec, &e.dec.osd
	tb := e.truth[i].burst
	want, _ := tb.BPTCPayload()
	var free, rpos []int16
	for j := range d.bptc.n {
		r := false
		for x := softBPTCData; x < softBPTCInfo; x++ {
			r = r || int(d.bptc.sys[x]) == j
		}
		if r {
			rpos = append(rpos, d.bptc.pos[j])
		} else {
			free = append(free, d.bptc.pos[j])
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
	d.bptc.osd(s, d.OSDOrder, d.OSDWindow, softListMax)
	for x := range s.list.n {
		cw := &s.list.cw[x]
		if check != nil && !check(d.bptc.bptcResult(cw).Payload) {
			continue
		}
		cand := tb
		for j := range d.bptc.n {
			softPut(&cand, d.bptc.pos[j], softBit(cw, j))
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

// sameBits reports whether bursts a and b agree at the burst bits pos.
func sameBits(a, b *layer2.RawBurst, pos []int16) bool {
	for _, p := range pos {
		m := byte(0x80) >> (p % 8)
		if a[p/8]&m != b[p/8]&m {
			return false
		}
	}
	return true
}

// rate34 scores the rate 3/4 payload of data burst i.
func (e *floorEval) rate34(i int) bool {
	tb := e.truth[i].burst
	want, _ := tb.Rate34Payload()
	e.setFree(e.dec.trellis.pos[:])
	e.addInfo("rate34", e.detect(i, false)-144*math.Ln2)
	cand := tb
	e.dec.decodeRate34(&e.llr, &cand)
	if got, _ := cand.Rate34Payload(); got == want {
		return true
	}
	return !e.beats(i, &cand)
}

// rate1 scores the rate 1 payload of data burst i, and its bits with the
// exact bitwise MAP detection, with a genie trellis of window 2: the long
// runs of free symbols of a rate 1 block need the wider window.
func (e *floorEval) rate1(i int) bool {
	if e.g2 == nil {
		e.g2 = newGenieTrellis(2, 2*slotSymbols)
	}
	e.g = e.g2
	defer func() { e.g = e.g1 }()
	tb := e.truth[i].burst
	e.setFree(e.r1pos[:])
	e.addInfo("rate1", e.detect(i, false)-192*math.Ln2)
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

// convBound returns the converse lower bound on the error rate of any
// decoder of a field whose information densities, less the log of the
// number M of field values, are u: the largest P[u <= t] - e^t over t.
// For any decoder, any output density q and any g > 0, P(correct) is at
// most P[i > log g] + g/M, where i = log p(y|x)/q(y): on the outputs where
// i <= log g, p(y|x) <= g q(y), and the M decoding sets have a total q
// measure of at most 1 (Verdu and Han, IEEE Trans. Inf. Theory 40(4),
// 1994, with any q as in Polyanskiy, Poor and Verdu, IEEE Trans. Inf.
// Theory 56(5), 2010). q here is the output density of independent
// uniform free symbols that the genie trellis computes.
func convBound(u []float64) float64 {
	v := slices.Clone(u)
	slices.Sort(v)
	var best float64
	for k, t := range v {
		best = max(best, float64(k+1)/float64(len(v))-math.Exp(t))
	}
	return best
}
