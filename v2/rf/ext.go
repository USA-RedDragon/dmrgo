package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/c2mode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type leanExt interface {
	keepFit(q int8)
	noteStrobe(c *leanCore)
	noteGrid(c *leanCore, d float32, n int)
	burst(c *leanCore, b *Burst)
	end(c *leanCore)
	kept() []int16
	locked(c *leanCore, d float32)
	firstSlot(c *leanCore)
	place(c *leanCore, b *Burst, j int32) bool
	unplaced(c *leanCore, b *Burst)
	dropped(c *leanCore)
	verify(c *leanCore, p enums.SyncPattern, rho2 float32) bool
	scan(c *leanCore) (src int, data bool, back int64, ok bool)
	lockShift(c *leanCore, p enums.SyncPattern, back int64) (float32, bool)
	weakAt(end int64)
	anchorProves(end int64) bool
	replayLock(j int32, b *Burst)
	replayFits(c *leanCore, j int32, pos enums.VoiceBurstType) bool
	replayPos(c *leanCore, j int32, b *Burst) (enums.VoiceBurstType, bool)
	seekAgrees(j int32, cc enums.ColorCode, lcss enums.LCSS) bool
	lockFit(c *leanCore, sym, t int64, q float32)
	syncFit(c *leanCore, sym, t int64, q, w float32)
	liveFit(c *leanCore, sym, t int64, q, w float32)
	rateSettled(c *leanCore) bool
	rateAgrees(c *leanCore, base int64) bool
	lineShift(c *leanCore, sym, fit int64) (float32, bool)
	grid(c *leanCore, start int64) (int64, float32, bool)
	rateBound(c *leanCore, base int64) (float32, bool)
	replayTicks(c *leanCore, start int64) (int64, int16, float32, bool)
	stepped()
	lockTune(c *leanCore, p enums.SyncPattern, src int, verified bool) bool
	replaySpread() float32
}

const extQueue = 8

const extEarlySymbols = 3 * slotSymbols

const extLate = 5

type lateBurst struct {
	b    Burst
	ring [leanRing]int8
	base int64
	lean bool
}

type extChan struct {
	frag  [4][embBits]int16
	have  uint8
	b     int64
	elc   elcState
	late  [extLate]lateBurst
	lateN int
	raw   bool
	c2    c2mode.Detector
}

type softExt struct {
	fit             [extFitSamples]int8
	fitHead, fitN   int32
	seekPend        [2]seekNote
	strobes         *[extStrobes]int64
	vP              enums.SyncPattern
	vEnd            int64
	vHz             float64
	vOK             bool
	soft            *softDecoder
	checks          [16]func([12]byte) bool
	work, work0     int64
	passes          int
	llr             [layer2.BurstBits]int16
	pre             [layer2.BurstBits]int16
	preOK           bool
	cachLLR         [layer2.CACHBits]int16
	ch              [2]extChan
	q               [extQueue]Burst
	held            [extQueue]bool
	early           [extQueue]bool
	earlyStart      [extQueue]int64
	timing          bool
	atN             int64
	atAt            float32
	anc, rate       [2]extAnchor
	spread          float32
	ancBase         int64
	proofCC         enums.ColorCode
	weakEnd         int64
	proofs          uint8
	head            int
	n               int
	sum             [4][embBits]int16
	out             [4]layer2.RawBurst
	acq             []int16
	refill          func(*leanCore, int64)
	rt              lockRetune
	pow             *iqPower //nolint:unused // the I/Q power of the soft stages under TinyGo
	rs              *audioRate
	slc             *shortLCOut
	refillCoh       func(float64)
	settleCoh       func()
	lookback        int
	ring            [leanRing]int8
	lockSym         int64
	replayN         int32
	replayP         enums.SyncPattern
	lockAt          float32
	stretch         float32
	stretchSig      float32
	lockN           int64
	stages          uint8
	coh             *cohStage
	saver           cohSaver
	ph              *discPhase
	ac              *acStage
	np              *npml
	qcoh            [extQueue]bool
	qc2             [extQueue]uint8
	qcore           [extQueue]layer2.RawBurst
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
	if c.unproven {
		x.prove(c, b)
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
		s.fitOnly = (s.budget > 0 || s.ex != nil) && x.checked(b) && s.leanInBand()
		x.work0 = s.work + x.work
		done := x.cohBurst(c, b, ch, cc, ccOK)
		if s.budget > 0 && done {
			s.credit -= float64(s.work + x.work - x.work0)
		}
		if done {
			return
		}
		if s.fitOnly && s.ex != nil && s.fitReady {
			x.checkedData(c, b)
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
		ch.have = 0
		ch.elc.reset()
		if x.stages&extOSD != 0 {
			x.data(c, b)
		}
		noteCallLC(&ch.raw, out, x.checks[enums.DataTypeVoiceLCHeader])
		ch.elc.callLC(out, x.checks[enums.DataTypeVoiceLCHeader])
	default:
		ch.have = 0
	}
	x.embFit(c, b)
	x.push(c, b, hold, c.base+cachSymbols)
	if b.Voice == enums.VoiceBurstE && ch.have == 0xf {
		x.embeddedLC(ch, b.Slot)
		ch.have = 0
	}
	x.drain(c, false, 0)
	if x.coh != nil && x.coh.budget > 0 {
		x.coh.credit -= float64(x.coh.work + x.work - x.work0)
	}
}

func (x *softExt) cohCACH(c *leanCore, b *Burst) {
	if c.preCut {
		return
	}
	if !x.coh.cachLLR(&x.cachLLR) {
		return
	}
	var hard [3]byte
	sc := &softSmallCodes[softTACTCode]
	f := &c.fec
	var tact uint32
	for p, v := range &x.cachLLR {
		if v < 0 {
			hard[p/8] |= 0x80 >> (p % 8)
		}
	}
	for j := range sc.n {
		v := x.cachLLR[sc.pos[j]]
		f.l[j] = v
		if v < 0 {
			tact |= 1 << j
		}
	}
	b.CACH, b.FixedCACH = hard, hard
	_, cw, _ := decodeGathered(f, softTACTCode, tact, 0, 0)
	putTACT(b, cw)
	if x.slc != nil {
		x.slc.noteCoh(b.Seconds, b.CACH, &x.cachLLR)
	}
}

func (x *softExt) checkedData(c *leanCore, b *Burst) {
	llr := x.llr
	if !x.coh.runAll(&x.llr) {
		x.llr = llr
		return
	}
	var hard layer2.RawBurst
	for p, v := range &x.llr {
		if v < 0 {
			hard[p/8] |= 0x80 >> (p % 8)
		}
	}
	x.llr = llr
	if burstDistance(&hard, &b.Fixed) < burstDistance(&b.Data, &b.Fixed) {
		b.Data = hard
	}
	x.cohCACH(c, b)
}

func burstDistance(a, b *layer2.RawBurst) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
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
	x.pre, x.preOK = x.llr, true
	fixed, data := b.Fixed, b.Data
	x.repairAll(c, b, cc, ccOK)
	coh := b.Data
	if c.unproven {
		x.prove(c, b)
	}
	if x.voiceKnown(b) && x.coh.refine(&b.Fixed, &x.known, &x.llr) {
		x.repairAll(c, b, cc, ccOK)
	} else {
		x.dataPasses(c, b, cc, ccOK)
	}
	x.uncoded(b, &data, &coh, true)
	if x.bptcRetry(b) && x.affords(extCoreCredit) {
		x.coreData(c, b, fixed, data)
	}
	x.cohCACH(c, b)
	x.coh.trackBurst(&b.Fixed)
	if x.coh.ex != nil && burstDistance(&data, &b.Fixed) < burstDistance(&b.Data, &b.Fixed) {
		b.Data = data
	}
	hold := false
	if _, kind, known := layer2.SyncSourceOf(b.Sync); b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice {
		hold = x.fragment(ch, b)
	} else {
		ch.have = 0
		ch.elc.reset()
		noteCallLC(&ch.raw, &b.Fixed, x.checks[enums.DataTypeVoiceLCHeader])
		ch.elc.callLC(&b.Fixed, x.checks[enums.DataTypeVoiceLCHeader])
	}
	if x.coh.ex != nil && data != b.Data && x.embPick(c, b, &data) {
		takeEmb(b, &data)
	}
	x.embFit(c, b)
	i := x.push(c, b, hold, c.base+cachSymbols)
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
	if !x.checked(b) {
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
		case enums.DataTypeIdle:
			p := pdu.PRFillBits()
			putBPTC(out, &p, 0)
		case enums.DataTypeRate1, enums.DataTypeReserved:
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
	pre := &x.llr
	if x.preOK {
		pre = &x.pre
	}
	x.preOK = false
	copy(ch.elc.test[i][:], pre[embFirstBit:embFirstBit+embBits])
	ch.elc.noteRef(pre, &b.Fixed, i)
	ch.have |= 1 << i
	return i < 3
}

func (x *softExt) embeddedLC(ch *extChan, e int64) {
	var ps [4]*Burst
	var qi [4]int
	for k := range x.n {
		j := (x.head + k) % extQueue
		b := &x.q[j]
		if i := (b.Slot - ch.b) / 2; b.Slot&1 == e&1 && b.Slot >= ch.b && b.Slot <= e && (b.Slot-ch.b)%2 == 0 {
			ps[i], qi[i] = b, j
		}
	}
	for i, b := range ps {
		if b == nil {
			x.release(ch)
			return
		}
		x.out[i] = b.Fixed
	}
	lc, ok := x.soft.decodeELC(&ch.frag, &ch.elc, &x.sum, &x.out)
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
			x.qc2[qi[i]] = c2Voice
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

func (x *softExt) weakAt(end int64) { x.weakEnd = end }

func (x *softExt) anchorProves(end int64) bool {
	d := end - x.weakEnd
	return d < -2 || d > 2
}

func (x *softExt) prove(c *leanCore, b *Burst) {
	var cc enums.ColorCode
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	switch {
	case known && kind == layer2.KindData:
		st := b.Data.SlotType()
		if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected > leanProofErrors {
			return
		}
		cc = st.ColorCode
	case b.Voice > enums.VoiceBurstA && b.Voice <= enums.VoiceBurstF:
		emb := embOf(&b.Data)
		if emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > leanProofErrors {
			return
		}
		cc = emb.ColorCode
	default:
		return
	}
	if k, ok := c.colorCode(); ok {
		c.unproven = k != cc
		return
	}
	if x.proofs > 0 && x.proofCC == cc {
		c.unproven = false
		return
	}
	if x.proofs == 0 {
		x.proofCC = cc
	}
	x.proofs++
}

func (x *softExt) push(c *leanCore, b *Burst, hold bool, start int64) int {
	if x.n == extQueue && c.unproven {
		x.head = (x.head + 1) % extQueue
		x.n--
	}
	if x.n == extQueue {
		x.held[x.head], x.early[x.head] = false, false
		x.drain(c, true, 0)
	}
	i := (x.head + x.n) % extQueue
	known := x.timed(x.ancParity(c, start-cachSymbols))
	x.q[i], x.held[i], x.qcoh[i], x.qc2[i] = *b, hold, false, c2MarkOf(b, x.ch[b.Slot&1].raw)
	x.early[i], x.earlyStart[i] = x.timing && !known, start
	x.n++
	x.retime(c)
	return i
}

func (x *softExt) retime(c *leanCore) {
	for k := range x.n {
		i := (x.head + k) % extQueue
		if !x.early[i] {
			continue
		}
		start := x.earlyStart[i]
		if t, e, ok := x.grid(c, start); ok {
			x.q[i].Seconds, x.q[i].SecondsSymbols, x.q[i].SecondsError = tickSeconds(t-int64(c.lagTicks)), 0, stampSeconds(e)
		} else if !x.timed(x.ancParity(c, start-cachSymbols)) && c.ns-start <= extEarlySymbols {
			continue
		}
		x.early[i] = false
	}
}

func (x *softExt) drain(c *leanCore, one bool, keep int) {
	for x.n > keep && !x.held[x.head] && !x.early[x.head] && !c.unproven {
		c2Note(&x.ch[x.q[x.head].Slot&1].c2, &x.q[x.head], x.qc2[x.head])
		c.emit(&x.q[x.head])
		x.head = (x.head + 1) % extQueue
		x.n--
		if one {
			return
		}
	}
}

func (x *softExt) kept() []int16 { return x.acq }

// extFitSamples is the size of the ring of recent samples that the EMB
// timing fit of a burst reads. The core keeps its acquisition samples only
// while it searches or hunts, so the fit has its own ring, written at every
// sample, apart from the replay lookback.
const extFitSamples = 256

func (x *softExt) keepFit(q int8) {
	x.fit[x.fitHead] = q
	x.fitHead = (x.fitHead + 1) & (extFitSamples - 1)
	x.fitN = min(x.fitN+1, extFitSamples)
}

func (x *softExt) fitKept(c *leanCore, n int) bool {
	return float32(n)*2*c.tm.step+4-c.lastAt <= float32(x.fitN)
}

func (x *softExt) fillFit(c *leanCore, win *syncWindow, b0 int32) {
	win.b0, win.off = b0, c.off*(1.0/discStepHz)
	h := x.fitHead - 1 - b0
	for k := range win.w {
		win.w[k] = x.fit[h&(extFitSamples-1)]
		h--
	}
}

func (x *softExt) unplaced(c *leanCore, b *Burst) {
	ch := &x.ch[b.Slot&1]
	if ch.lateN == extLate {
		copy(ch.late[:], ch.late[1:])
		ch.lateN--
	}
	l := &ch.late[ch.lateN]
	ch.lateN++
	l.b, l.ring, l.base, l.lean = *b, c.ring, c.base, c.backfill
	x.drain(c, false, 0)
}

func (x *softExt) dropped(c *leanCore) {
	x.drain(c, false, 0)
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
		late := x.coh != nil && !l.lean && x.cohLate(c, lb)
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
		x.push(c, lb, hold, l.base+cachSymbols)
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

const extStrobes = 256

// noteStrobe keeps the strobe time of the newest symbol, for the coherent
// stage to place the symbols of a burst where the timing loop sampled them.
func (x *softExt) noteStrobe(c *leanCore) {
	if x.strobes != nil {
		x.strobes[(c.ns-1)&(extStrobes-1)] = c.strobeTicks(c.lastAt)
	}
}

// noteGrid keeps the times of the n newest symbols after resampleStart has
// sliced them again on the nominal grid that ends at the strobe moved by d.
func (x *softExt) noteGrid(c *leanCore, d float32, n int) {
	if x.strobes == nil {
		return
	}
	sps := 2 * c.tm.step
	for i := range n {
		j := c.ns - int64(n) + int64(i)
		x.strobes[j&(extStrobes-1)] = c.strobeTicks(c.lastAt + d - float32(n-1-i)*sps)
	}
}

func (x *softExt) setCoh(s *cohStage) {
	x.coh = s
	x.strobes = new([extStrobes]int64)
	s.strobes = x.strobes
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

func (x *softExt) verify(c *leanCore, p enums.SyncPattern, rho2 float32) bool {
	if x.coh == nil {
		return false
	}
	n := 0
	for k := c.ns - syncSymbols; k < c.ns; k++ {
		if q := c.ring[k%leanRing]; q == math.MaxInt8 || q == math.MinInt8 {
			n++
		}
	}
	if g, _, _ := c.fitSync(c.ns-1, p); 4*n > syncSymbols || g <= leanMinScale {
		return false
	}
	return x.coh.verify(c, p, rho2)
}

func (x *softExt) lockShift(c *leanCore, p enums.SyncPattern, back int64) (float32, bool) {
	if x.coh == nil {
		return 0, false
	}
	x.vP, x.vEnd = p, c.ns-1-back
	d, ok := x.coh.lockShift(c, p, back)
	if math.Abs(d) < cohWarmStepSym/2 {
		return 0, false
	}
	return float32(d), ok
}

func (x *softExt) locked(c *leanCore, d float32) {
	x.lockSym, x.lockAt, x.lockN = c.ns-1, c.lastAt+d, c.n
	x.spread = float32(math.Inf(1))
	voice, dp, _ := sourceAt(int(c.src)).SyncPatterns()
	for _, p := range [2]enums.SyncPattern{voice, dp} {
		if g, _, res := c.fitSync(c.ns-1, p); g > 0 {
			x.spread = min(x.spread, sqrtf(res)/(2*g*deviationHz))
		}
	}
	x.early, x.timing, x.stretch = [extQueue]bool{}, !c.bs, 0
	x.vOK = false
	x.proofs = 0
	if x.coh != nil {
		x.vHz, x.vOK, x.coh.vOK, x.coh.vTauOK = x.coh.vHz, x.coh.vOK, false, false
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
	x.refitScale(c)
	if a := c.audio; a != nil {
		c.fitHz += d / a.gain
	}
}

func (x *softExt) refitScale(c *leanCore) {
	if c.ns-x.vEnd+syncSymbols > leanRing {
		return
	}
	var syms [syncSymbols]int8
	syncSyms(x.vP, &syms)
	j := ringPos(x.vEnd - syncSymbols + 1)
	var sxy, spp float32
	for k := range syncSymbols {
		sxy += float32(syms[k]) * (float32(c.ring[j])*discStepHz + c.off - c.dcHz)
		spp += float32(syms[k]) * float32(syms[k])
		if j++; j == leanRing {
			j = 0
		}
	}
	if g := sxy / spp / deviationHz; g > 0 {
		c.devScale = max(g, leanMinScale)
		c.tm.dev = c.devScale * deviationHz
	}
}

func (x *softExt) replaySlots(c *leanCore, last int64, at, sps float32) int32 {
	room := float32(c.acqKept()-4) - at
	_, an := c.acqBuf()
	fresh := c.acqFrom == 0 && c.acqKept() < an
	n := int32(0)
	for int(n) < x.lookback {
		oldest := c.first - int64(n+1)*slotSymbols - 1
		start := oldest + 1
		if !c.bs || fresh && start < 0 {
			start += cachSymbols
			oldest = max(oldest, start-1)
		}
		if start < 0 || float32(last-oldest)*sps > room {
			break
		}
		n++
	}
	return n
}

func (x *softExt) stepped() {
	if x.ph == nil || x.coh == nil {
		return
	}
	x.ph.stepped()
	x.coh.freqOK = false
	for i := range x.coh.radios {
		x.coh.radios[i].freqOK = false
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
	x.atN = x.lockN
	if c.hunt && c.acqFrom <= x.lockN {
		at -= float32(c.n - x.lockN)
		x.atN = c.n
	}
	x.atAt = at
	n32 := x.replaySlots(c, last, at, sps)
	n := int(n32)
	if n == 0 {
		return
	}
	x.refillLock(c)
	if x.coh != nil {
		x.coh.warmFreq(c, c.first, n)
	}
	x.stretchSig = 0
	stretch := x.clockStretch(c, n, last, at, sps)
	if stretch == 0 && x.coh != nil {
		stretch = float32(x.coh.warmStretch)
	}
	x.stretch = stretch
	if stretch != 0 {
		c.tm.rate = stretch * sps
	}
	x.ring = c.ring
	first, eAcc, eSyms, hold, slotEnd := c.first, c.eAcc, c.eSyms, c.tm.hold, c.slotEnd
	ms := c.ms
	levels := [3]float32{c.dcHz, c.devScale, c.discVar}
	clicks := c.clicks
	c.replaying = true
	x.replayN, x.replayP = n32, 0
	for _, dry := range [2]bool{true, false} {
		votes := c.votes
		c.resetChans()
		c.votes = votes
		c.dcHz, c.devScale, c.discVar, c.clicks = levels[0], levels[1], levels[2], clicks
		c.next, c.lastAnch, c.lost = 0, leanNoSlot, 0
		c.ms.anch, c.ms.off = [2]int32{leanNoSlot, leanNoSlot}, [2]int16{}
		if !c.bs {
			c.lastAnch = n32
			c.ms.anch[n&1] = n32
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
			c.replayDeep = n-j > leanReplayNear
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
	c.replaying, c.replayDry, c.replayDeep = false, false, false
	if c.lastAnch == n32 {
		c.lastAnch = leanNoSlot
	}
	c.clicks = clicks
	rs := &c.ms
	ms.anch[0], ms.anch[1] = leanNoSlot, leanNoSlot
	for i := range rs.anch {
		if a := rs.anch[i]; a != leanNoSlot && a != n32 {
			ms.anch[i], ms.lv[i], ms.lvN[i] = a, rs.lv[i], rs.lvN[i]
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
	x.settleLock()
	x.lockHz(c)
	x.vOK = false
}

func (x *softExt) refillLock(c *leanCore) {
	if x.refillCoh == nil || c.forced {
		return
	}
	hz := x.vHz
	if !x.vOK {
		hz, _ = x.coh.src.cohLockHz()
	}
	x.refillCoh(hz)
}

func (x *softExt) settleLock() {
	if x.settleCoh != nil {
		x.settleCoh()
	}
}

func (x *softExt) replayLock(j int32, b *Burst) {
	if j == x.replayN {
		x.replayP = b.Sync
	}
}

func (x *softExt) replayFits(c *leanCore, j int32, pos enums.VoiceBurstType) bool {
	if c.replayDry {
		return true
	}
	d := x.replayN - j
	if d&1 != 0 {
		return true
	}
	voice, _, _ := sourceAt(int(c.src)).SyncPatterns()
	return x.replayP == voice && d%int32(2*(enums.VoiceBurstF-enums.VoiceBurstA+1)) == 2*int32(enums.VoiceBurstF-pos+1)
}

// replayPos gives the voice position of a replayed burst in the slot
// parity of a lock on the stream's data SYNC, as the voice terminator that
// follows burst F has: burst F two slots before it, back to burst B ten slots
// before it. The burst counts only when its EMB carries the stream's color
// code and the LCSS of that position (ETSI TS 102 361-1 clause 7.1.3), with
// at most one corrected bit, so the slots before a data lock that is not a
// terminator take no position.
func (x *softExt) replayPos(c *leanCore, j int32, b *Burst) (enums.VoiceBurstType, bool) {
	_, data, _ := sourceAt(int(c.src)).SyncPatterns()
	d := x.replayN - j
	if c.replayDry || x.replayP != data || d&1 != 0 || d < 2 || d > 2*int32(enums.VoiceBurstF-enums.VoiceBurstB+1) {
		return 0, false
	}
	pos := enums.VoiceBurstF - enums.VoiceBurstType(d/2-1)
	want, _ := pos.LCSS()
	cc, ok := c.colorCode()
	emb := embOf(&b.Data)
	if !ok || emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > 1 || emb.ColorCode != cc || emb.LCSS != want {
		return 0, false
	}
	return pos, true
}

type seekNote struct {
	j    int32
	cc   enums.ColorCode
	lcss enums.LCSS
	ok   bool
}

// seekAgrees notes the EMB of a seeked MS slot that is not enough alone (one
// corrected bit, or no color code vote yet) and tells whether the EMB of the
// same talker's slot before it, two slots back, carries the same color code
// and an LCSS that the voice superframe can follow with this one.
func (x *softExt) seekAgrees(j int32, cc enums.ColorCode, lcss enums.LCSS) bool {
	p := &x.seekPend[j&1]
	ok := p.ok && p.j == j-2 && p.cc == cc && lcssFollows(p.lcss, lcss)
	*p = seekNote{j: j, cc: cc, lcss: lcss, ok: !ok}
	return ok
}

func lcssFollows(a, b enums.LCSS) bool {
	return a == enums.LCSSFirstFragment && b == enums.LCSSContinuationFragment ||
		a == enums.LCSSContinuationFragment && (b == enums.LCSSContinuationFragment || b == enums.LCSSLastFragment)
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

const (
	replayTimeVarBS = 0.01565
	replayTimeVarMS = 0.01924
)

func (x *softExt) clockStretch(c *leanCore, n int, last int64, at, sps float32) float32 {
	e, m := x.replayStretch(c, n, last, at, sps)
	if e == 0 && m < 2 {
		e = blindStretch(c, n, last, at, sps)
	}
	return e
}

const (
	blindSlip = 0.45
	blindStep = 0.2
	blindEye  = 0.3
)

func blindMixed(k, m int) bool {
	return 5*k >= m && 5*k <= 4*m
}

type blindFit struct {
	c            *leanCore
	first, last  int64
	room, d, ofs float32
	at, sps      float32
	good         uint32
	j0, n        int64
}

func (f *blindFit) eye(j int64, s float32) (e float32, m, outer, pos int) {
	base := f.first + j*slotSymbols
	for k := base + cachSymbols; k < base+slotSymbols; k++ {
		back := float32(f.last-k) * f.sps * (1 + s)
		if back > f.room {
			continue
		}
		v := f.c.keptAt(f.at-back)*discStepHz + f.ofs
		y := abs32(v)
		e += min32(abs32(y-f.d), abs32(y-3*f.d))
		m++
		if y > 2*f.d {
			outer++
		}
		if v > 0 {
			pos++
		}
	}
	return e / f.d, m, outer, pos
}

func (f *blindFit) total(s float32) float32 {
	var t float32
	for j := f.j0; j < f.n; j++ {
		if f.good>>(j-f.j0)&1 != 0 {
			e, _, _, _ := f.eye(j, s)
			t += e
		}
	}
	return t
}

func blindStretch(c *leanCore, n int, last int64, at, sps float32) float32 {
	f := blindFit{c: c, first: c.first - int64(n)*slotSymbols, last: last, room: float32(c.acqKept()-4) - at, d: c.devScale * deviationHz, ofs: c.off - c.dcHz, at: at, sps: sps, n: int64(n)}
	if f.d <= 0 {
		return 0
	}
	near, far := float32(0), float32(0)
	f.j0 = max(int64(n)-32, 0)
	for j := f.j0; j < int64(n); j++ {
		if !c.bs && (int64(n)-j)&1 != 0 {
			continue
		}
		span := float32(last - f.first - j*slotSymbols)
		step := blindStep / span
		top := float32(math.MaxFloat32)
		cnt, out, pos := 0, 0, 0
		for s := -float32(leanClockMax); s <= leanClockMax; s += step {
			if e, m, o, p := f.eye(j, s); m > 0 && e/float32(m) < top {
				top, cnt, out, pos = e/float32(m), m, o, p
			}
		}
		if cnt >= slotSymbols/2 && top < blindEye && blindMixed(out, cnt) && blindMixed(pos, cnt) {
			f.good |= 1 << (j - f.j0)
			near, far = span, max(far, span)
		}
	}
	if f.good == 0 {
		return 0
	}
	lim := min32(leanClockMax, blindSlip/near)
	step := blindStep / far
	best, top := float32(0), f.total(0)
	for s := -lim; s <= lim; s += step {
		if v := f.total(s); v < top {
			best, top = s, v
		}
	}
	lo := best - step
	for i := range 17 {
		s := lo + float32(i)*step/8
		if v := f.total(s); v < top {
			best, top = s, v
		}
	}
	if abs32(best)*far < 0.1 {
		return 0
	}
	return best
}

func (x *softExt) replayStretch(c *leanCore, n int, last int64, at, sps float32) (float32, int) {
	voice, data, _ := sourceAt(int(c.src)).SyncPatterns()
	var syms [2][syncSymbols]int8
	syncSyms(voice, &syms[0])
	syncSyms(data, &syms[1])
	first := c.first - int64(n)*slotSymbols
	var sw, sx, sy, sxx, sxy float32
	tv := float32(replayTimeVarMS)
	if c.bs {
		tv = replayTimeVarBS
	}
	m := 0
	for j := int64(-1); j < int64(n); j++ {
		span := float32(0)
		if j >= 0 {
			span = float32(last-first-j*slotSymbols-leanSyncEnd) * sps
			if span <= 0 || !c.bs && (int64(n)-j)&1 != 0 {
				continue
			}
		}
		reach := 4 + int(leanClockMax*span*8/sps)
		best, p, r := keptSyncPeak(c, &syms, at-span, sps, reach)
		if best <= -reach || best >= reach {
			if j < 0 {
				return 0, m
			}
			continue
		}
		var cv [3]float32
		for i := range cv {
			cv[i] = keptSyncCorr(c, &syms[p], at-span+float32(best+i-1)*sps/8, sps)
		}
		off := (float32(best) + edgeVertex3(&cv)) * sps / 8
		if j >= 0 {
			x.syncFit(c, first+j*slotSymbols+leanSyncEnd, c.ticksAt(x.atN, at-span+keptMMZero(c, &syms[p], at-span, off, sps)), r, 1)
		}
		w := r / (max32(1-r, 1e-6) * tv * sps * sps)
		sw, sx, sy, sxx, sxy = sw+w, sx+w*span, sy+w*off, sxx+w*span*span, sxy+w*span*off
		m++
	}
	if m < 2 {
		return 0, m
	}
	vx := sxx - sx*sx/sw
	if vx <= 0 {
		return 0, m
	}
	b := (sxy - sx*sy/sw) / vx
	if e := -b; b*b*vx > leanClockSig*leanClockSig && abs32(e) <= leanClockMax {
		x.stretchSig = 1 / sqrtf(vx)
		return e, m
	}
	return 0, m
}

func keptSyncPeak(c *leanCore, syms *[2][syncSymbols]int8, end, sps float32, w int) (best, pat int, top float32) {
	best = -w
	oldest := float32(c.acqKept() - 4)
	for p := range syms {
		for i := -w; i <= w; i++ {
			t := end + float32(i)*sps/8
			if float32(syncSymbols-1)*sps-t > oldest {
				continue
			}
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
	return best, pat, top
}

func keptSyncCorr(c *leanCore, syms *[syncSymbols]int8, t, sps float32) float32 {
	var v float32
	for m, q := range syms {
		v += float32(q) * c.keptAt(t-float32(syncSymbols-1-m)*sps)
	}
	return v
}

func edgeVertex3(c *[3]float32) float32 {
	if den := c[0] - 2*c[1] + c[2]; den < 0 {
		return max32(min32(0.5*(c[0]-c[2])/den, 0.5), -0.5)
	}
	return 0
}

func (x *softExt) end(c *leanCore) {
	for i := range x.held {
		x.held[i], x.early[i] = false, false
	}
	x.ch[0].have, x.ch[1].have = 0, 0
	x.ch[0].lateN, x.ch[1].lateN = 0, 0
	x.ch[0].raw, x.ch[1].raw = false, false
	x.ch[0].elc, x.ch[1].elc = elcState{}, elcState{}
	if c.unproven {
		x.n = 0
	}
	x.drain(c, false, 0)
	x.ch[0].c2.Reset()
	x.ch[1].c2.Reset()
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
	for i := range layer2.VoiceSlotsPerBurst {
		if !x.coreCleaner() || nCore[i] > nCoh[i] {
			continue
		}
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

const (
	extAnchorSpan      = 16 * slotSymbols
	extAnchorReach     = 2 * slotSymbols
	extAnchorRho2      = 0.985
	extAnchorJump      = 0.25
	extAnchorSig       = 0.0034
	extSyncMid         = -(syncSymbols - 1) / 2.0
	extSettleFits      = 3
	extEmbWeight       = 1.0 / 32
	extReplaySpread    = 8
	extReplaySpreadMin = 0.5
	extEmbGate         = 4 * 4e-6 * symbolRate
	extPullSig         = 3
	extRateFits        = 3
	extSettleSpan      = 4 * slotSymbols
)

type extAnchor struct {
	ref, tref int64
	n, w      float32
	sx, sy    float32
	sxx, sxy  float32
	lo, hi    float32
}

func (x *softExt) lockFit(c *leanCore, sym, t int64, q float32) {
	x.ancBase, x.anc, x.rate = c.first, [2]extAnchor{}, [2]extAnchor{}
	x.syncFit(c, sym, t, q, 1)
}

func (x *softExt) ancSlot(base int64) int64 {
	k := base - x.ancBase + slotSymbols/2
	q := k / slotSymbols
	if k < 0 && k%slotSymbols != 0 {
		q--
	}
	return q
}

func (x *softExt) ancParity(c *leanCore, base int64) int {
	if c.bs {
		return 0
	}
	return int(x.ancSlot(base) & 1)
}

func (x *softExt) syncFit(c *leanCore, sym, t int64, q, w float32) {
	if q < extAnchorRho2 {
		return
	}
	xs := x.ancSlot(sym-leanSyncEnd)*slotSymbols + leanSyncEnd
	t -= (syncSymbols - 1) * samplesPerSy << 7
	p := x.ancParity(c, sym-leanSyncEnd)
	if w < 1 && !x.rate[p].fits(xs, t) {
		return
	}
	v := q / (leanRefitVar*(1-q) + q*extAnchorSig*extAnchorSig)
	x.rate[p].add(xs, t, v)
	if w == 1 {
		x.anc[p].add(xs, t, v)
	}
}

func (a *extAnchor) add(xs, t int64, w float32) {
	if a.n > 0 {
		if p, ok := a.at(xs, 0); !ok || abs64(t-p) > int64(extAnchorJump*samplesPerSy*256) {
			a.n = 0
		}
	}
	if a.n == 0 {
		*a = extAnchor{ref: xs, tref: t}
	}
	k := xs - a.ref
	if k < -extAnchorSpan || k > extAnchorSpan {
		return
	}
	v := float32(k)
	y := float32(t - a.tref - k*samplesPerSy<<8)
	a.n, a.w, a.sx, a.sy, a.sxx, a.sxy = a.n+1, a.w+w, a.sx+w*v, a.sy+w*y, a.sxx+w*v*v, a.sxy+w*v*y
	a.lo, a.hi = min(a.lo, v), max(a.hi, v)
}

func (a *extAnchor) line() (c, b float32, ok bool) {
	if a.n < 2 || a.hi-a.lo < 2*slotSymbols {
		return a.sy / max(a.w, 1e-6), 0, false
	}
	b = (a.w*a.sxy - a.sx*a.sy) / (a.w*a.sxx - a.sx*a.sx)
	return (a.sy - b*a.sx) / a.w, b, abs32(b) <= leanClockMax*(samplesPerSy<<8)
}

func (a *extAnchor) at(xs int64, frac float32) (int64, bool) {
	k := xs - a.ref
	if v := float32(k); v > a.hi+extAnchorReach || v < a.lo-extAnchorReach {
		if _, _, ok := a.line(); !ok {
			return 0, false
		}
	}
	c, b, _ := a.line()
	u := float32(k) + frac
	return a.tref + k*samplesPerSy<<8 + int64(roundf(frac*samplesPerSy*256+c+b*u)), true
}

func (a *extAnchor) clocked() bool {
	_, b, ok := a.line()
	v := a.sxx - a.sx*a.sx/a.w
	sig := float32(samplesPerSy * 256)
	return ok && b*b*v > leanClockSig*leanClockSig*sig*sig
}

// grid gives the start of the burst at start from the timing line of its
// slot parity, and the error of that start that the uncertainty of the line
// slope gives over the distance from the centroid of the fits, in ticks.
func (x *softExt) grid(c *leanCore, start int64) (int64, float32, bool) {
	p := x.ancParity(c, start-cachSymbols)
	a := &x.anc[p]
	xs := x.ancSlot(start-cachSymbols)*slotSymbols + leanSyncEnd
	if v := float32(xs - a.ref); a.n == 0 || v > a.hi+extAnchorReach || v < a.lo-extAnchorReach {
		return 0, 0, false
	}
	k := xs - a.ref
	u := float32(k) + cachSymbols - leanSyncEnd - extSyncMid
	lever := abs32(u - a.sx/a.w)
	if a.clocked() {
		t, ok := a.at(xs, cachSymbols-leanSyncEnd-extSyncMid)
		return t, a.slopeSig() * lever, ok
	}
	r := &x.rate[p]
	if r.n < extRateFits || !r.clocked() {
		return 0, 0, false
	}
	_, b, _ := r.line()
	return a.tref + k*samplesPerSy<<8 + int64(roundf((cachSymbols-leanSyncEnd-extSyncMid)*samplesPerSy*256+a.sy/a.w+b*(u-a.sx/a.w))), r.slopeSig() * lever, true
}

// replayTicks gives the start of a burst that the late-entry replay detects,
// from the strobe of the lock that the replay resamples from, at the period
// the replay resamples at. With a stretch fitted to the replayed SYNCs it
// also gives the error that the uncertainty of that fit makes, in ticks;
// without one, the symbols projected at a period of unknown error.
func (x *softExt) replayTicks(c *leanCore, start int64) (int64, int16, float32, bool) {
	if !c.replaying {
		return 0, 0, 0, false
	}
	k := float32(x.lockSym - start)
	sps := 2 * c.tm.step
	t := c.ticksAt(x.atN, x.atAt-k*sps*(1+x.stretch))
	if x.stretch == 0 || x.stretchSig == 0 {
		return t, int16(k), 0, true
	}
	return t, 0, x.stretchSig * k * sps * float32(c.mfStep) * 256, true
}

func (a *extAnchor) slopeSig() float32 {
	return samplesPerSy * 256 / sqrtf(max32(a.sxx-a.sx*a.sx/a.w, 1e-6))
}

func (x *softExt) rateBound(c *leanCore, base int64) (float32, bool) {
	d, sig, ok := x.rate[x.ancParity(c, base)].rateError(c)
	if !ok {
		return x.stretchSig * 2 * c.tm.step * float32(c.mfStep) * 256, x.stretch != 0 && c.tm.rate == x.stretch*2*c.tm.step
	}
	return abs32(d) + sig, d*d <= leanClockSig*leanClockSig*sig*sig
}

func (x *softExt) timed(p int) bool {
	_, _, ok := x.anc[p].line()
	r := &x.rate[p]
	return ok || r.n >= extRateFits && r.clocked()
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func keptMMZero(c *leanCore, syms *[syncSymbols]int8, end, off, sps float32) float32 {
	var win syncWindow
	b0 := max(int32(-end-off-sps/2)-2, 0)
	if b0+leanSyncWindow > c.acqKept() {
		return off
	}
	c.fillWindow(&win, b0)
	return min(win.mmZero(syms, end, off, sps), -end)
}

func (x *softExt) liveFit(c *leanCore, sym, t int64, q, w float32) {
	x.syncFit(c, sym, t, q, w)
	p := x.ancParity(c, sym-leanSyncEnd)
	if c.bs {
		return
	}
	d, sig, ok := x.rate[p].rateError(c)
	if !ok || d*d <= leanClockSig*leanClockSig*sig*sig {
		return
	}
	tm := &c.tm
	v := tm.rate + d/(float32(c.mfStep)*256)
	tm.owed += (v - tm.rate) * leanClockLag
	tm.baseN, tm.baseC = tm.baseN+tm.nall, tm.baseC+tm.corr
	tm.corr, tm.corr2, tm.nc, tm.nall = 0, 0, 0, 0
	tm.rate = v
	tm.mOK = false
	tm.estS, tm.estW, tm.estN, tm.learned = 0, 0, 0, true
}

func (a *extAnchor) rateError(c *leanCore) (d, sig float32, ok bool) {
	_, b, ok := a.line()
	if !ok {
		return 0, 0, false
	}
	v := a.sxx - a.sx*a.sx/a.w
	return b - c.tm.rate*float32(c.mfStep)*256, samplesPerSy * 256 / sqrtf(v), true
}

func (x *softExt) rateSettled(c *leanCore) bool {
	for i := range x.anc {
		a := &x.rate[i]
		if a.n < extSettleFits || a.hi-a.lo < extSettleSpan {
			continue
		}
		if d, sig, ok := a.rateError(c); ok && d*d <= leanClockSig*leanClockSig*sig*sig {
			return true
		}
	}
	return false
}

func (x *softExt) rateAgrees(c *leanCore, base int64) bool {
	d, sig, ok := x.rate[x.ancParity(c, base)].rateError(c)
	if !ok {
		return x.stretch != 0 && c.tm.rate == x.stretch*2*c.tm.step
	}
	return d*d <= leanClockSig*leanClockSig*sig*sig
}

func (x *softExt) embFit(c *leanCore, b *Burst) {
	if e := embOf(&b.Data); e.FEC.Uncorrectable || e.FEC.ErrorsCorrected != 0 {
		return
	}
	if sym, t, q, ok := x.embTiming(c, b, &b.Data); ok {
		x.liveFit(c, sym, t, q, extEmbWeight)
	}
}

const syncFieldBit = (leanSyncEnd - cachSymbols - (syncSymbols - 1)) * 2

// embTiming fits the timing of the SYNC field of the voice burst b, which
// holds the EMB and an embedded signalling fragment, to the symbols of data,
// and gives the symbol of the fit, its time in ticks and its rho2.
func (x *softExt) embTiming(c *leanCore, b *Burst, data *layer2.RawBurst) (int64, int64, float32, bool) {
	if c.bs || c.isReplaying() || b.Voice <= enums.VoiceBurstA || b.Voice > enums.VoiceBurstF || !x.timing {
		return 0, 0, 0, false
	}
	const back = slotSymbols - 1 - leanSyncEnd
	if c.ns-1 != c.base+slotSymbols-1 || !x.fitKept(c, back+syncSymbols+4) {
		return 0, 0, 0, false
	}
	var syms [syncSymbols]int8
	for i := range syncSymbols {
		k := syncFieldBit + i*2
		syms[i] = symbolOf(unpacked.Bit(data[k/8]>>(7-k%8)&1), unpacked.Bit(data[(k+1)/8]>>(7-(k+1)%8)&1))
	}
	sps := 2*c.tm.step + c.tm.rate
	end := c.lastAt - float32(back)*sps
	var win syncWindow
	x.fillFit(c, &win, max(int32(-end-sps/2)-2, 0))
	var cv [7]float32
	best := win.peak(&syms, &cv, end, sps, true)
	if best == 0 || best == len(cv)-1 {
		return 0, 0, 0, false
	}
	d := float32(best-3)*sps/4 + edgeVertex(&cv, best)*sps/4
	z := min(win.mmZero(&syms, end, d, sps), -end)
	sv, svv, sxv, sp := win.sums(&syms, end+z, sps)
	var spp float32
	for _, q := range syms {
		spp += float32(q) * float32(q)
	}
	const n = syncSymbols
	sxy, sxx, pp := sxv-sp*sv/n, svv-sv*sv/n, spp-sp*sp/n
	if sxy <= 0 || sxx <= 0 || pp <= 0 {
		return 0, 0, 0, false
	}
	return c.ns - 1 - back, c.strobeTicks(end + z), sxy * sxy / (sxx * pp), true
}

// embPick reports whether the SYNC field of alt, the other detection of the
// voice burst b, is the better one: its EMB decodes with fewer corrected
// bits, or as few and the field fits the received symbols closer. Fixed
// cannot judge the embedded signalling bits of a single burst.
func (x *softExt) embPick(c *leanCore, b *Burst, alt *layer2.RawBurst) bool {
	if b.Voice <= enums.VoiceBurstA || b.Voice > enums.VoiceBurstF {
		return false
	}
	e, a := embOf(&b.Data), embOf(alt)
	switch {
	case a.FEC.Uncorrectable:
		return false
	case e.FEC.Uncorrectable:
		return true
	case a.FEC.ErrorsCorrected != e.FEC.ErrorsCorrected:
		return a.FEC.ErrorsCorrected < e.FEC.ErrorsCorrected
	}
	_, _, qa, okA := x.embTiming(c, b, alt)
	_, _, qe, okE := x.embTiming(c, b, &b.Data)
	return okA && (!okE || qa > qe)
}

// takeEmb copies the SYNC field of src, the EMB and the embedded signalling
// fragment, into b.Data, and into b.Fixed where it held the field of b.Data.
func takeEmb(b *Burst, src *layer2.RawBurst) {
	for k := syncFieldBit; k < syncFieldBit+2*syncSymbols; k++ {
		m := byte(0x80) >> (k % 8)
		if b.Fixed[k/8]&m == b.Data[k/8]&m {
			b.Fixed[k/8] = b.Fixed[k/8]&^m | src[k/8]&m
		}
		b.Data[k/8] = b.Data[k/8]&^m | src[k/8]&m
	}
}

func (x *softExt) lineShift(c *leanCore, sym, fit int64) (float32, bool) {
	if !x.rateAgrees(c, sym-leanSyncEnd) {
		return 0, false
	}
	a := &x.anc[x.ancParity(c, sym-leanSyncEnd)]
	t, ok := a.at(x.ancSlot(sym-leanSyncEnd)*slotSymbols+leanSyncEnd, -extSyncMid)
	if !ok || float32(abs64(fit-t)) > extPullSig*extAnchorSig*samplesPerSy*256 {
		return 0, false
	}
	return float32(t-c.strobeTicks(c.lastAt)) / (float32(c.mfStep) * 256), true
}

func (a *extAnchor) fits(xs, t int64) bool {
	if _, _, ok := a.line(); !ok {
		return true
	}
	p, ok := a.at(xs, 0)
	return ok && float32(abs64(t-p)) <= extEmbGate*samplesPerSy*256
}

func (x *softExt) replaySpread() float32 {
	return max(extReplaySpread*x.spread, extReplaySpreadMin)
}
