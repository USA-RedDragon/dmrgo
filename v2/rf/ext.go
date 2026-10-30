package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type leanExt interface {
	burst(c *leanCore, b *Burst)
	end(c *leanCore)
	kept() []int16
	locked(c *leanCore, d float32)
	firstSlot(c *leanCore)
	place(c *leanCore, b *Burst, j int32) bool
	unplaced(c *leanCore, b *Burst)
	verify(c *leanCore, p enums.SyncPattern) bool
	scan(c *leanCore) (src int, data bool, back int64, ok bool)
	lockShift(c *leanCore, p enums.SyncPattern, back int64) (float32, bool)
}

const extQueue = 8

const extLate = 5

type lateBurst struct {
	b    Burst
	ring [leanRing]int8
	base int64
}

type extChan struct {
	frag  [4][embBits]int16
	have  uint8
	b     int64
	acc   [4][embBits]int16
	accN  int
	late  [extLate]lateBurst
	lateN int
	raw   bool
}

type softExt struct {
	vHz             float64
	vOK             bool
	soft            *softDecoder
	checks          [16]func([12]byte) bool
	work, work0     int64
	passes          int
	llr             [layer2.BurstBits]int16
	cachLLR         [layer2.CACHBits]int16
	ch              [2]extChan
	q               [extQueue]Burst
	held            [extQueue]bool
	head            int
	n               int
	sum             [4][embBits]int16
	out             [4]layer2.RawBurst
	acq             []int16
	refill          func(*leanCore, int64)
	lookback        int
	ring            [leanRing]int8
	lockSym         int64
	lockAt          float32
	lockN           int64
	stages          uint8
	coh             *cohStage
	saver           cohSaver
	ph              *discPhase
	union           *discUnion
	ac              *acStage
	np              *npml
	qcoh            [extQueue]bool
	qcore           [extQueue]layer2.RawBurst
	cur, side       combSide
	qside           [extQueue]combSide
	errCore, errCoh float32
	known           [layer2.BurstBits]bool
	ext             [layer2.BurstBits]float32
	next            [layer2.BurstBits]int16
	pch             [layer2.BurstBits]int16
}

const (
	extAMBE = 1 << iota
	extOSD
	extELC
	extAll = extAMBE | extOSD | extELC
)

func newSoftExt(lookback int, sps float32) *softExt {
	return newSoftExtStages(extAll, lookback, sps)
}

func softExtOf(opt *options, sps float32) *softExt {
	var st uint8
	if opt.ExtAMBE {
		st |= extAMBE
	}
	if opt.ExtOSD {
		st |= extOSD
	}
	if opt.ExtELC {
		st |= extELC
	}
	x := newSoftExtStages(st, opt.ExtLookback, sps)
	x.soft.setBytes(opt)
	if opt.ExtAMBECandidates > 0 {
		x.soft.AMBECandidates = opt.ExtAMBECandidates
	}
	if opt.ExtOSDOrder > 0 {
		x.soft.OSDOrder = opt.ExtOSDOrder - 1
	}
	if opt.ExtGridIters > 0 {
		x.soft.GridIters = opt.ExtGridIters
	}
	if opt.ExtNPML {
		x.np = &npml{}
	}
	if opt.CohDataPasses != 0 {
		x.passes = max(opt.CohDataPasses, 0)
	}
	return x
}

func newSoftExtStages(st uint8, lookback int, sps float32) *softExt {
	x := &softExt{soft: newSoftDecoder(), lookback: lookback, stages: st, passes: cohDataPasses}
	if lookback > 0 {
		n := int(float32((lookback+1)*slotSymbols)*sps) + leanAcqSamples
		x.acq = make([]int16, (n+1)/2)
	}
	for dt := range x.checks {
		x.checks[dt] = softBPTCCheck(enums.DataType(dt))
	}
	return x
}

func (x *softExt) burst(c *leanCore, b *Burst) {
	if c.isReplayDry() {
		x.warm(c, b)
		return
	}
	if x.np != nil && c.anchor != 0 {
		x.np.fitSync(c, c.anchor, c.base)
	}
	cc, ccOK := c.colorCode()
	ch := &x.ch[b.Slot&1]
	x.placeLate(c, ch, b, cc, ccOK)
	c.repair(&c.fec, b, cc, ccOK)
	out := &b.Fixed
	if x.coh != nil {
		s := x.coh
		s.fitOnly = (s.budget > 0 || s.ex != nil) && x.checked(b) || s.voiceOnly && !isVoiceBurst(b)
		x.work0 = s.work + x.work
		done := x.cohBurst(c, b, ch, cc, ccOK)
		if s.budget > 0 && done {
			s.credit -= float64(s.work + x.work - x.work0)
		}
		if done {
			return
		}
	}
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	hold := false
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if x.stages&(extAMBE|extELC) != 0 {
			x.fill(c, &b.Data)
		}
		if x.stages&extAMBE != 0 && !ch.raw {
			x.soft.decodeAMBE(&x.llr, out)
		}
		if x.stages&extELC != 0 {
			hold = x.fragment(ch, b)
		}
	case known && kind == layer2.KindData:
		ch.have, ch.accN = 0, 0
		if x.stages&extOSD != 0 {
			x.data(c, b)
		}
		noteCallLC(&ch.raw, out, x.checks[enums.DataTypeVoiceLCHeader])
	default:
		ch.have = 0
	}
	x.push(c, b, hold)
	if b.Voice == enums.VoiceBurstE && ch.have == 0xf {
		x.embeddedLC(ch, b.Slot)
		ch.have = 0
	}
	x.drain(c, false, 0)
	if x.coh != nil && x.coh.budget > 0 {
		x.coh.credit -= float64(x.coh.work + x.work - x.work0)
	}
}

func (x *softExt) warm(c *leanCore, b *Burst) {
	s := x.coh
	if s == nil || !s.warm {
		return
	}
	fit := s.fitOnly
	s.fitOnly = true
	s.detect(c, b, &x.llr)
	s.fitOnly = fit
}

func (x *softExt) cohBurst(c *leanCore, b *Burst, ch *extChan, cc enums.ColorCode, ccOK bool) bool {
	if !x.coh.detect(c, b, &x.llr) {
		return false
	}
	fixed, data := b.Fixed, b.Data
	x.repairAll(c, b, cc, ccOK)
	coh := b.Data
	if x.voiceKnown(b) && x.coh.refine(&b.Fixed, &x.known, &x.llr) {
		x.repairAll(c, b, cc, ccOK)
	} else {
		x.dataPasses(c, b, cc, ccOK)
	}
	x.uncoded(b, &data, &coh, true)
	if x.bptcRetry(b) && x.affords(extCoreCredit) {
		x.coreData(c, b, fixed, data)
	}
	if x.coh.cachLLR(&x.cachLLR) {
		c.sink.noteCoh(b.Seconds, b.CACH, &x.cachLLR)
	}
	x.coh.trackBurst(&b.Fixed)
	hold := false
	if _, kind, known := layer2.SyncSourceOf(b.Sync); b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice {
		hold = x.fragment(ch, b)
	} else {
		ch.have, ch.accN = 0, 0
		noteCallLC(&ch.raw, &b.Fixed, x.checks[enums.DataTypeVoiceLCHeader])
	}
	i := x.push(c, b, hold)
	x.coh.qllr[i], x.qcoh[i], x.qcore[i] = x.llr, true, data
	if x.coh.nextOK {
		x.back(c, b.Slot-1)
	}
	if b.Voice == enums.VoiceBurstE && ch.have == 0xf {
		x.embeddedLC(ch, b.Slot)
		ch.have = 0
	}
	x.drain(c, false, 1)
	return true
}

func (x *softExt) data(c *leanCore, b *Burst) {
	out := &b.Fixed
	st := out.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	dt := st.DataType
	if dt == enums.DataTypeRate34 && x.np != nil {
		x.fill(c, &b.Data)
		x.soft.decodeRate34(&x.llr, out)
		return
	}
	check := x.checks[dt&0xF]
	if check == nil {
		return
	}
	if p, _ := out.BPTCPayload(); check(p) {
		return
	}
	x.fill(c, &b.Data)
	saved := *out
	if !x.decodeBPTC(dt, out) {
		*out = saved
	}
}

func (x *softExt) decodeBPTC(dt enums.DataType, out *layer2.RawBurst) bool {
	check := x.checks[dt&0xF]
	_, r := x.soft.decodeBPTCOSD(&x.llr, check, out)
	if r.Uncorrectable || r.ErrorsCorrected > 0 {
		x.work += extWorkOSD
	}
	if check == nil || !r.Uncorrectable {
		return true
	}
	saved := *out
	x.work += extWorkGrid + extWorkOSD
	if _, r = x.soft.decodeBPTCGrid(&x.llr, check, out); r.Uncorrectable {
		*out = saved
		return false
	}
	return true
}

const (
	extWorkAMBE = 70
	extWorkOSD  = 300
	extWorkGrid = 120
)

func (x *softExt) back(c *leanCore, j int64) {
	for k := range x.n {
		i := (x.head + k) % extQueue
		b := &x.q[i]
		if b.Slot != j || !x.qcoh[i] {
			continue
		}
		cur, lc, data := x.llr, b.Fixed, b.Data
		x.llr = x.coh.qllr[i]
		x.coh.nextInto(&x.llr)
		x.coh.qllr[i] = x.llr
		cc, ccOK := c.colorCode()
		ok := !x.bptcRetry(b)
		x.repairAll(c, b, cc, ccOK)
		coh := b.Data
		x.uncoded(b, &x.qcore[i], &coh, false)
		x.qside[i], x.cur = x.cur, combSide{}
		if ok && x.bptcRetry(b) {
			b.Fixed, b.Data = lc, data
		}
		if b.Voice >= enums.VoiceBurstB && b.Voice <= enums.VoiceBurstE {
			for q := embFirstBit; q < embFirstBit+embBits; q++ {
				m := byte(0x80) >> (q % 8)
				b.Fixed[q/8] = b.Fixed[q/8]&^m | lc[q/8]&m
			}
		}
		x.llr = cur
		return
	}
}

const (
	cohDataPasses = 2
	cohDataPrior  = 0.7
)

func (x *softExt) dataPasses(c *leanCore, b *Burst, cc enums.ColorCode, ccOK bool) {
	ch := &x.pch
	*ch = x.llr
	fixed, data := b.Fixed, b.Data
	var st [20]int16
	for i := range st {
		st[i] = ch[98+i+i/10*48]
	}
	for range x.passes {
		if !x.bptcRetry(b) || !x.affords(extPassCredit) {
			break
		}
		x.soft.bptcExtrinsic(ch, &x.ext)
		x.work += extWorkGrid
		if !x.coh.refineData(&x.ext, cohDataPrior, &x.next) {
			break
		}
		x.llr = x.next
		bp := &softOSDCodes[softBPTCCode]
		for _, i := range bp.pos[:bp.n] {
			v := float32(x.next[i])/8 - cohDataPrior*x.ext[i]
			ch[i] = satLLR(v)
			x.llr[i] = satLLR(v + x.ext[i])
		}
		x.repairAll(c, b, cc, ccOK)
	}
	if x.bptcRetry(b) {
		x.coh.mineOK = false
	}
	for i := range 20 {
		q := 98 + i + i/10*48
		m := byte(0x80) >> (q % 8)
		b.Fixed[q/8] = b.Fixed[q/8]&^m | fixed[q/8]&m
		b.Data[q/8] = b.Data[q/8]&^m | data[q/8]&m
		x.llr[q] = st[i]
	}
}

//go:noinline
func (x *softExt) coreData(c *leanCore, b *Burst, fixed, data layer2.RawBurst) {
	coh, cohFixed, cohData := x.llr, b.Fixed, b.Data
	b.Fixed, b.Data = fixed, data
	x.data(c, b)
	if x.bptcRetry(b) {
		b.Fixed, b.Data = cohFixed, cohData
	}
	x.llr = coh
}

func (x *softExt) affords(n float64) bool {
	s := x.coh
	return s.budget <= 0 || s.credit-float64(s.work+x.work-x.work0) >= n*s.budget
}

const (
	extPassCredit = 2
	extCoreCredit = 3
)

func (x *softExt) checked(b *Burst) bool {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	if !known || kind != layer2.KindData {
		return false
	}
	st := b.Fixed.SlotType()
	if st.FEC.Uncorrectable {
		return false
	}
	check := x.checks[st.DataType&0xF]
	if check == nil {
		return false
	}
	p, _ := b.Fixed.BPTCPayload()
	return check(p)
}

func (x *softExt) bptcRetry(b *Burst) bool {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	if !known || kind != layer2.KindData {
		return false
	}
	st := b.Fixed.SlotType()
	if st.FEC.Uncorrectable {
		return false
	}
	switch dt := st.DataType; dt {
	case enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		return false
	default:
		p, _ := b.Fixed.BPTCPayload()
		if check := x.checks[dt&0xF]; check != nil {
			return !check(p)
		}
		h, _ := b.Data.BPTCPayload()
		return h != p
	}
}

func isVoiceBurst(b *Burst) bool {
	_, kind, ok := layer2.SyncSourceOf(b.Sync)
	return b.Voice != enums.VoiceBurstUnknown || ok && kind == layer2.KindVoice
}

func (x *softExt) voiceKnown(b *Burst) bool {
	_, kind, ok := layer2.SyncSourceOf(b.Sync)
	if b.Voice == enums.VoiceBurstUnknown && (!ok || kind != layer2.KindVoice) || x.ch[b.Slot&1].raw {
		return false
	}
	clear(x.known[:])
	for id := softAMBECode; id < softSmallCount; id++ {
		c := smallCode(id)
		for _, q := range c.pos[:c.n] {
			x.known[q] = true
		}
	}
	if b.Voice != enums.VoiceBurstUnknown && b.Voice != enums.VoiceBurstA {
		c := &softSmallCodes[softEMBCode]
		for _, q := range c.pos[:c.n] {
			x.known[q] = true
		}
	}
	return true
}

func (x *softExt) repairAll(c *leanCore, b *Burst, cc enums.ColorCode, ccOK bool) {
	out := &b.Fixed
	*out = layer2.RawBurst{}
	for p, v := range &x.llr {
		if v < 0 {
			out[p/8] |= 0x80 >> (p % 8)
		}
	}
	b.Data = *out
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if !x.ch[b.Slot&1].raw {
			x.soft.decodeAMBE(&x.llr, out)
			x.work += extWorkAMBE
		}
		if b.Voice != enums.VoiceBurstUnknown && b.Voice != enums.VoiceBurstA {
			x.soft.decodeEMB(&x.llr, cc, ccOK, out)
		}
	case known && kind == layer2.KindData:
		st, res := x.soft.decodeSlotType(&x.llr, cc, ccOK, out)
		if res.Uncorrectable {
			break
		}
		switch dt := st.DataType(); dt {
		case enums.DataTypeRate34:
			x.soft.decodeRate34(&x.llr, out)
		case enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		default:
			_ = x.decodeBPTC(dt, out)
		}
	}
	if known && c.anchor == b.Sync {
		_ = out.SetSync(b.Sync)
	}
}

func (x *softExt) fill(c *leanCore, hard *layer2.RawBurst) {
	if x.np != nil {
		x.np.fill(c, &x.llr)
	} else {
		for p := range x.llr {
			x.llr[p] = c.llrAt(p)
		}
	}
	for p, v := range &x.llr {
		if v == 0 {
			x.llr[p] = 1 - 2*int16(hard[p/8]>>(7-p%8)&1)
		}
	}
}

func (x *softExt) fragment(ch *extChan, b *Burst) bool {
	if b.Voice < enums.VoiceBurstB || b.Voice > enums.VoiceBurstE {
		if b.Voice != enums.VoiceBurstF {
			ch.have = 0
		}
		return false
	}
	i := int(b.Voice - enums.VoiceBurstB)
	if i == 0 {
		ch.have, ch.b = 0, b.Slot
	}
	if b.Slot != ch.b+2*int64(i) || ch.have != 1<<i-1 {
		ch.have = 0
		x.release(ch)
		return false
	}
	copy(ch.frag[i][:], x.llr[embFirstBit:embFirstBit+embBits])
	ch.have |= 1 << i
	return i < 3
}

func (x *softExt) embeddedLC(ch *extChan, e int64) {
	var ps [4]*Burst
	for k := range x.n {
		b := &x.q[(x.head+k)%extQueue]
		if i := (b.Slot - ch.b) / 2; b.Slot&1 == e&1 && b.Slot >= ch.b && b.Slot <= e && (b.Slot-ch.b)%2 == 0 {
			ps[i] = b
		}
	}
	for i, b := range ps {
		if b == nil {
			x.release(ch)
			return
		}
		x.out[i] = b.Fixed
	}
	ok := false
	var lc [9]byte
	if ch.accN > 0 {
		for i := range x.sum {
			x.sum[i] = ch.acc[i]
			softCombine(x.sum[i][:], ch.frag[i][:])
		}
		if l, res := x.soft.decodeEmbeddedFrags(&x.sum, &x.out); !res.Uncorrectable {
			ok, lc = true, l
			ch.acc, ch.accN = x.sum, min(ch.accN+1, maxCombine)
		}
	}
	if !ok {
		l, res := x.soft.decodeEmbeddedFrags(&ch.frag, &x.out)
		ok, lc = !res.Uncorrectable, l
		ch.acc, ch.accN = ch.frag, 1
	}
	if ok {
		if r, voice := rawVoiceLC(lc[:]); voice {
			ch.raw = r
		}
	}
	for i, b := range ps {
		if ok {
			b.Fixed = x.out[i]
		}
		if ch.raw {
			keepVoiceSlots(&b.Fixed, &b.Data)
		}
	}
	x.release(ch)
}

func (x *softExt) release(ch *extChan) {
	j := int64(0)
	if ch == &x.ch[1] {
		j = 1
	}
	for k := range x.n {
		i := (x.head + k) % extQueue
		if x.q[i].Slot&1 == j {
			x.held[i] = false
		}
	}
}

func (x *softExt) push(c *leanCore, b *Burst, hold bool) int {
	if x.n == extQueue {
		x.held[x.head] = false
		x.drain(c, true, 0)
	}
	i := (x.head + x.n) % extQueue
	x.q[i], x.held[i], x.qcoh[i] = *b, hold, false
	x.qside[i], x.cur = x.cur, combSide{}
	x.n++
	return i
}

func (x *softExt) drain(c *leanCore, one bool, keep int) {
	for x.n > keep && !x.held[x.head] {
		x.side = x.qside[x.head]
		c.emit(&x.q[x.head])
		x.head = (x.head + 1) % extQueue
		x.n--
		if one {
			return
		}
	}
}

func (x *softExt) kept() []int16 { return x.acq }

func (x *softExt) unplaced(c *leanCore, b *Burst) {
	ch := &x.ch[b.Slot&1]
	if ch.lateN == extLate {
		copy(ch.late[:], ch.late[1:])
		ch.lateN--
	}
	l := &ch.late[ch.lateN]
	ch.lateN++
	l.b, l.ring, l.base = *b, c.ring, c.base
}

func (x *softExt) placeLate(c *leanCore, ch *extChan, b *Burst, cc enums.ColorCode, ccOK bool) {
	n := ch.lateN
	ch.lateN = 0
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	pos := b.Voice
	if pos == enums.VoiceBurstUnknown {
		if !known || kind != layer2.KindVoice {
			return
		}
		pos = enums.VoiceBurstA
	}
	if pos == enums.VoiceBurstA {
		pos += 6
	}
	voice, _, _ := sourceAt(int(c.src)).SyncPatterns()
	cur, ring, base := x.llr, c.ring, c.base
	pi := b.Data.EMB().PreemptionAndPowerControlIndicator
	for i := range n {
		l := &ch.late[i]
		v, ok := latePos(pos, b.Slot, l.b.Slot)
		if !ok {
			continue
		}
		lb := &l.b
		lb.Voice = v
		if v == enums.VoiceBurstA {
			lb.Voice, lb.Sync, lb.Distance = enums.VoiceBurstUnknown, voice, syncDist(&lb.Data, voice)
		}
		lb.HasCACH = false
		c.ring, c.base = l.ring, l.base
		c.repair(&c.fec, lb, cc, ccOK)
		if lb.Voice != enums.VoiceBurstUnknown && ccOK {
			_ = lb.Fixed.SetEMB(pdu.EmbeddedSignalling{ColorCode: cc, PreemptionAndPowerControlIndicator: pi, LCSS: lateLCSS(lb.Voice)})
		}
		data := lb.Data
		late := x.coh != nil && x.cohLate(c, lb)
		if !late {
			x.fill(c, &lb.Data)
		}
		c.ring, c.base = ring, base
		x.repairAll(c, lb, cc, ccOK)
		if late {
			coh := lb.Data
			x.uncoded(lb, &data, &coh, false)
		}
		hold := false
		if x.stages&extELC != 0 {
			hold = x.fragment(ch, lb)
		}
		x.push(c, lb, hold)
		if lb.Voice == enums.VoiceBurstE && ch.have == 0xf {
			x.embeddedLC(ch, lb.Slot)
			ch.have = 0
		}
	}
	x.llr = cur
}

func latePos(pos enums.VoiceBurstType, j, s int64) (enums.VoiceBurstType, bool) {
	for v, k := pos-1, j-2; v >= enums.VoiceBurstA && k >= s; v, k = v-1, k-2 {
		if k == s {
			return v, true
		}
	}
	return 0, false
}

func lateLCSS(v enums.VoiceBurstType) enums.LCSS {
	switch v {
	case enums.VoiceBurstB:
		return enums.LCSSFirstFragment
	case enums.VoiceBurstE:
		return enums.LCSSLastFragment
	case enums.VoiceBurstF:
		return enums.LCSSSingleFragment
	case enums.VoiceBurstUnknown, enums.VoiceBurstA, enums.VoiceBurstC, enums.VoiceBurstD:
	}
	return enums.LCSSContinuationFragment
}

func (x *softExt) setCoh(s *cohStage) {
	x.coh = s
	if cohSaveHeap {
		x.saver.save = new(cohStage)
	}
}

func (x *softExt) place(c *leanCore, b *Burst, j int32) bool {
	return x.coh != nil && x.coh.place(c, b, j)
}

func (x *softExt) scan(c *leanCore) (int, bool, int64, bool) {
	if x.coh == nil || x.ph != nil {
		return 0, false, 0, false
	}
	return x.coh.scan(c)
}

func (x *softExt) verify(c *leanCore, p enums.SyncPattern) bool {
	if x.coh == nil || x.coh.locked {
		return false
	}
	n := 0
	for k := c.ns - syncSymbols; k < c.ns; k++ {
		if q := c.ring[k%leanRing]; q == math.MaxInt8 || q == math.MinInt8 {
			n++
		}
	}
	return 4*n <= syncSymbols && x.coh.verify(c, p)
}

func (x *softExt) lockShift(c *leanCore, p enums.SyncPattern, back int64) (float32, bool) {
	if x.coh == nil {
		return 0, false
	}
	d, ok := x.coh.lockShift(c, p, back)
	if math.Abs(d) < cohWarmStepSym/2 {
		return 0, false
	}
	return float32(d), ok
}

func (x *softExt) locked(c *leanCore, d float32) {
	x.lockSym, x.lockAt, x.lockN = c.ns-1, c.lastAt+d, c.n
	x.vOK = false
	if x.coh != nil {
		x.vHz, x.vOK, x.coh.vOK = x.coh.vHz, x.coh.vOK, false
		x.lockHz(c)
		x.coh.freqOK = false
		x.coh.lock()
	}
	if x.ph != nil {
		x.ph.lock()
	}
	if x.np != nil {
		x.np.end()
	}
}

func (x *softExt) lockHz(c *leanCore) {
	if !x.vOK {
		return
	}
	hz, perHz := x.coh.src.cohLockHz()
	if perHz <= 0 || math.IsInf(perHz, 0) {
		return
	}
	d := float32((x.vHz - hz) * perHz)
	c.dcHz += d
	c.tm.dc = c.dcHz
	if a := c.audio; a != nil {
		c.fitHz += d / a.gain
	}
}

func (x *softExt) firstSlot(c *leanCore) {
	if x.lookback == 0 || c.forced && c.devScale <= leanMinScale {
		return
	}
	if x.refill != nil {
		end := x.lockN
		if c.hunt {
			end = c.n
		}
		x.refill(c, end)
	}
	sps := 2 * c.tm.step
	last := x.lockSym
	at := x.lockAt
	if c.hunt && c.acqFrom <= x.lockN {
		at -= float32(c.n - x.lockN)
	}
	room := float32(c.acqKept()-4) - at
	n := 0
	for n < x.lookback {
		oldest := c.first - int64(n+1)*slotSymbols - 1
		start := oldest + 1
		if !c.bs {
			start += cachSymbols
		}
		if start < 0 || float32(last-oldest)*sps > room {
			break
		}
		n++
	}
	if n == 0 {
		return
	}
	if x.coh != nil {
		x.coh.warmFreq(c, c.first, n)
	}
	stretch := x.replayStretch(c, n, last, at, sps)
	if stretch != 0 {
		c.tm.rate, c.tm.drift = stretch*sps, stretch*sps
	}
	x.ring = c.ring
	first, eAcc, eSyms, hold, slotEnd := c.first, c.eAcc, c.eSyms, c.tm.hold, c.slotEnd
	ms := c.ms
	levels := [3]float32{c.dcHz, c.devScale, c.discVar}
	clicks := c.clicks
	c.replaying = true
	for _, dry := range [2]bool{true, false} {
		votes := c.votes
		c.resetChans()
		c.votes = votes
		c.dcHz, c.devScale, c.discVar, c.clicks = levels[0], levels[1], levels[2], clicks
		c.next, c.lastAnch, c.lost = 0, leanNoSlot, 0
		c.ms.anch, c.ms.off = [2]int32{leanNoSlot, leanNoSlot}, [2]int16{}
		if !c.bs {
			c.lastAnch = int32(n)
			c.ms.anch[n&1] = int32(n)
		}
		c.first = first - int64(n)*slotSymbols
		c.slotEnd = c.first + slotSymbols
		c.replayDry = dry
		m := n
		if dry {
			m++
		}
		for j := range m {
			if j == n {
				c.ring = x.ring
			} else {
				base := c.first + int64(j)*slotSymbols + int64(c.ms.off[j&1])
				for k := base - 1; k <= base+slotSymbols; k++ {
					v := c.keptAt(at - float32(last-k)*sps*(1+stretch))
					c.ring[ringPos(k)] = int8(max(min(roundf(v), math.MaxInt8), math.MinInt8))
				}
			}
			c.eAcc, c.eSyms = 0, 0
			if j < n && replayFlat(c, c.first+int64(j)*slotSymbols) {
				noise := c.noise
				c.noise = max(noise, 1)
				c.slot()
				c.noise = noise
			} else {
				c.slot()
			}
		}
	}
	c.replaying, c.replayDry = false, false
	if c.lastAnch == int32(n) {
		c.lastAnch = leanNoSlot
	}
	c.clicks = clicks
	rs := &c.ms
	ms.anch[0], ms.anch[1] = leanNoSlot, leanNoSlot
	for i := range rs.anch {
		if a := rs.anch[i]; a != leanNoSlot && a != int32(n) {
			ms.anch[i], ms.lv[i] = a, rs.lv[i]
		}
	}
	c.ms = ms
	for i := range c.ms.anch {
		if c.ms.anch[i] != leanNoSlot {
			c.savePhase(int32(i))
		}
	}
	c.ring = x.ring
	c.first = first - int64(n)*slotSymbols
	c.slotEnd = slotEnd
	c.eAcc, c.eSyms, c.tm.hold = eAcc, eSyms, hold
	x.lockHz(c)
	x.vOK = false
}

func replayFlat(c *leanCore, base int64) bool {
	var sv, svv float32
	for k := base; k < base+slotSymbols; k++ {
		v := float32(c.ring[ringPos(k)])
		sv += v
		svv += v * v
	}
	const n = slotSymbols
	lvl := c.devScale * deviationHz / discStepHz * replayFlatLevel
	return svv/n-(sv/n)*(sv/n) < lvl*lvl
}

func (x *softExt) replayStretch(c *leanCore, n int, last int64, at, sps float32) float32 {
	voice, data, _ := sourceAt(int(c.src)).SyncPatterns()
	var syms [2][syncSymbols]int8
	syncSyms(voice, &syms[0])
	syncSyms(data, &syms[1])
	first := c.first - int64(n)*slotSymbols
	for j := range int64(n) {
		span := float32(last-first-j*slotSymbols-leanSyncEnd) * sps
		if span <= 0 || !c.bs && (int64(n)-j)&1 != 0 {
			continue
		}
		best, p := keptSyncPeak(c, &syms, at-span, sps)
		if best < 0 {
			continue
		}
		var cv [9]float32
		for i := range cv {
			cv[i] = keptSyncCorr(c, &syms[p], at-span+float32(i-4)*sps/8, sps)
		}
		off := float32(best-4) + edgeVertex9(&cv, best)
		if e := -off * sps / 8 / span; abs32(e) >= leanStretchMin && abs32(e) <= leanClockMax {
			return e
		}
		return 0
	}
	return 0
}

func keptSyncPeak(c *leanCore, syms *[2][syncSymbols]int8, end, sps float32) (best, pat int) {
	best = -1
	var top float32
	for p := range syms {
		for i := range 9 {
			t := end + float32(i-4)*sps/8
			var sv, svv, sxv, sp, spp float32
			for m, q := range &syms[p] {
				v := c.keptAt(t - float32(syncSymbols-1-m)*sps)
				sv += v
				svv += v * v
				sxv += float32(q) * v
				sp += float32(q)
				spp += float32(q) * float32(q)
			}
			const n = syncSymbols
			cov, vx, vp := sxv-sp*sv/n, svv-sv*sv/n, spp-sp*sp/n
			if cov <= 0 || vx <= 0 || cov*cov < replaySyncRho*vx*vp {
				continue
			}
			if r := cov * cov / (vx * vp); r > top {
				top, best, pat = r, i, p
			}
		}
	}
	return best, pat
}

func keptSyncCorr(c *leanCore, syms *[syncSymbols]int8, t, sps float32) float32 {
	var v float32
	for m, q := range syms {
		v += float32(q) * c.keptAt(t-float32(syncSymbols-1-m)*sps)
	}
	return v
}

func edgeVertex9(c *[9]float32, best int) float32 {
	if best <= 0 || best >= len(c)-1 {
		return 0
	}
	if den := c[best-1] - 2*c[best] + c[best+1]; den < 0 {
		return max32(min32(0.5*(c[best-1]-c[best+1])/den, 0.5), -0.5)
	}
	return 0
}

func (x *softExt) end(c *leanCore) {
	for i := range x.held {
		x.held[i] = false
	}
	x.ch[0].have, x.ch[1].have = 0, 0
	x.ch[0].accN, x.ch[1].accN = 0, 0
	x.ch[0].lateN, x.ch[1].lateN = 0, 0
	x.ch[0].raw, x.ch[1].raw = false, false
	if x.coh != nil {
		x.coh.freqOK = false
	}
	x.drain(c, false, 0)
}

const (
	extErrGain   = 1.0 / 16
	extCoreShare = 0.9
	extErrMax    = 6
)

func (x *softExt) uncoded(b *Burst, core, coh *layer2.RawBurst, update bool) {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	if known && kind == layer2.KindData {
		if x.coreCleaner() && b.Fixed.SlotType().DataType == enums.DataTypeRate1 {
			for i := range 2 * extPayloadHalf {
				p := i
				if i >= extPayloadHalf {
					p += layer2.BurstBits - 2*extPayloadHalf
				}
				q, m := p/8, byte(0x80)>>(p%8)
				b.Fixed[q] = b.Fixed[q]&^m | core[q]&m
			}
		}
		return
	}
	if b.Voice == enums.VoiceBurstUnknown && (!known || kind != layer2.KindVoice) || x.ch[b.Slot&1].raw {
		return
	}
	var nCore, nCoh [layer2.VoiceSlotsPerBurst]int
	for i := range layer2.VoiceSlotsPerBurst {
		for id := softAMBECode + 2*softSmallID(i); id <= softAMBECode+2*softSmallID(i)+1; id++ {
			c := smallCode(id)
			for _, p := range c.pos[:c.n] {
				q, m := p/8, byte(0x80)>>(p%8)
				nCore[i] += int((b.Fixed[q] ^ core[q]) & m >> (7 - p%8))
				nCoh[i] += int((b.Fixed[q] ^ coh[q]) & m >> (7 - p%8))
			}
		}
		if update && min(nCore[i], nCoh[i]) <= extErrMax {
			x.errCore += extErrGain * (float32(nCore[i]) - x.errCore)
			x.errCoh += extErrGain * (float32(nCoh[i]) - x.errCoh)
		}
	}
	x.cur.coreC2 = 0
	for i := range layer2.VoiceSlotsPerBurst {
		x.cur.coreN[i] = 255
		if n := nCore[i]; n >= 0 && n < 255 {
			x.cur.coreN[i] = uint8(n)
		}
		if !x.coreCleaner() || nCore[i] > nCoh[i] {
			continue
		}
		x.cur.coreC2 |= 1 << i
		for _, p := range &softAMBEC2Pos[i] {
			q, m := p/8, byte(0x80)>>(p%8)
			b.Fixed[q] = b.Fixed[q]&^m | core[q]&m
		}
	}
}

func (x *softExt) coreCleaner() bool {
	return x.errCore < extCoreShare*x.errCoh
}

const extPayloadHalf = 98
