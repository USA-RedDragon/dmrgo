package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// leanExt is a stage that a tier above Tier 0 adds after the detection of
// the core: it takes each burst that the core detects and places, reads
// the log-likelihood ratios of the core for it, and emits it through
// leanCore.emit. Tier 0 never sets one, so that TinyGo links none of its
// code into a DiscReceiver.
type leanExt interface {
	// burst decodes and emits burst b, which the core placed.
	burst(c *leanCore, b *Burst)
	// end emits the bursts that the stage holds, at the end of a stream.
	end(c *leanCore)
	// kept returns the ring of kept samples that the stage keeps in place
	// of the scratch of the core, or nil.
	kept() []int16
	// locked runs when the core locks to a stream, at a SYNC that it
	// sampled again d samples from its timing.
	locked(c *leanCore, d float32)
	// firstSlot runs as the first slot of a stream ends, before the core
	// handles it.
	firstSlot(c *leanCore)
	// place places burst b of slot j, which the core could not, and
	// reports whether it did.
	place(c *leanCore, b *Burst, j int32) bool
	// unplaced keeps burst b, which neither the core nor place placed,
	// for a later burst of its channel to place.
	unplaced(c *leanCore, b *Burst)
	// verify reports whether the last symbols of the core end SYNC p,
	// which the discriminator alone does not show.
	verify(c *leanCore, p enums.SyncPattern) bool
	// scan reports whether a SYNC of source src, data or voice, ended back
	// symbols before the last of the core, while the core searches.
	scan(c *leanCore) (src int, data bool, back int64, ok bool)
}

// extQueue is the most bursts that a softExt holds: the bursts B to D of a
// superframe and the bursts of the other channel between them.
const extQueue = 8

// extLate is the most bursts of a channel that a softExt keeps unplaced:
// the bursts B to F of a superframe before the burst A of the next.
const extLate = 5

// lateBurst is a burst that the core could not place, with the ring of
// symbols of the core and the first symbol of its slot.
type lateBurst struct {
	b    Burst
	ring [leanRing]int8
	base int64
}

// extChan is the embedded LC state of one channel of a softExt.
type extChan struct {
	// frag holds the ratios of the embedded signalling fragments of bursts
	// B to E of the superframe whose burst B is in slot b, and have marks
	// the fragments it holds.
	frag [4][embBits]int16
	have uint8
	b    int64
	// acc sums the ratios of the embedded LC of the earlier superframes of
	// the call that decoded, accN of them.
	acc  [4][embBits]int16
	accN int
	// late holds the lateN bursts of the channel since its last placed
	// burst, oldest first, that the core could not place.
	late  [extLate]lateBurst
	lateN int
}

// softExt decodes every FEC field of a burst with the soft decoders of the
// coherent tiers, from the ratios of the core: C0 and C1 of the voice
// frames by ML, BPTC(196,96) by ordered statistics and its grid, and the
// embedded LC of each superframe by ordered statistics over its four
// fragments, summed with the earlier superframes of the call. It holds the
// bursts B to D of a superframe until burst E, to write the corrected
// fragments into them.
type softExt struct {
	soft   *softDecoder
	checks [16]func([12]byte) bool
	// work is the cost of the decodes from ratios that the stages ran, in
	// trellis steps of the coherent stage, for its budget, and work0 the
	// work of both before the burst under decode.
	work, work0 int64
	// passes is the most detections again of a data burst.
	passes int
	llr    [layer2.BurstBits]int16
	ch     [2]extChan
	q      [extQueue]Burst
	// held marks the bursts of q that wait for their embedded LC.
	held [extQueue]bool
	head int
	n    int
	sum  [4][embBits]int16
	out  [4]layer2.RawBurst
	// acq is the ring of matched filter samples that the core keeps while
	// it searches, two to an element, long enough for lookback slots
	// before the burst of the SYNC that it locks to, and ring holds the
	// symbols of the core while those slots run.
	acq      []int16
	lookback int
	ring     [leanRing]int8
	// lockSym is the last symbol at the lock, the end of its SYNC, lockAt
	// its time from the last kept sample, in samples, and lockN the matched
	// filter samples of the core then.
	lockSym int64
	lockAt  float32
	lockN   int64
	// stages selects the decodes that replace those of Tier 0, and coh,
	// when not nil, detects each burst again coherently, in place of the
	// ratios of the core.
	stages uint8
	coh    *cohStage
	// ph rebuilds the ring of coh from the samples of a DiscReceiver.
	ph *discPhase
	// np, when not nil, detects the bursts from the symbols of the core
	// with the correlation of their noise, in place of symbol by symbol.
	np *npml
	// qcoh marks the bursts of q that coh detected, and qcore holds the
	// hard decisions of the core of each.
	qcoh  [extQueue]bool
	qcore [extQueue]layer2.RawBurst
	// cur is the evidence of options.Comb that uncoded found for the burst
	// under decode, qside that of each burst of q, and side that of the
	// burst that the core emits.
	cur, side combSide
	qside     [extQueue]combSide
	// errCore and errCoh are the mean numbers of bits of C0 and C1 of the
	// voice frames that coh detected whose hard decisions of the core and
	// of coh differ from their decode, for uncoded.
	errCore, errCoh float32
	// known marks the bits of a voice burst that its FEC fixed, for the
	// detection again of coh.
	known [layer2.BurstBits]bool
	// ext and next hold the extrinsic ratios and the new channel ratios of
	// a data pass.
	ext  [layer2.BurstBits]float32
	next [layer2.BurstBits]int16
}

// The stages of a softExt.
const (
	// extAMBE decodes C0 and C1 of each voice frame by soft ML.
	extAMBE = 1 << iota
	// extOSD decodes a BPTC(196,96) block whose check fails by ordered
	// statistics, and again over its grid.
	extOSD
	// extELC decodes the embedded LC of each superframe over its four
	// fragments, summed with the earlier superframes of the call.
	extELC
	// extAll is every stage.
	extAll = extAMBE | extOSD | extELC
)

// newSoftExt returns a softExt with every stage that decodes lookback
// slots before the SYNC of each lock, at sps matched filter samples per
// symbol.
func newSoftExt(lookback int, sps float32) *softExt {
	return newSoftExtStages(extAll, lookback, sps)
}

// softExtOf returns the softExt of the settings opt at sps matched filter
// samples per symbol.
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

// newSoftExtStages returns a softExt with the stages st that decodes
// lookback slots before the SYNC of each lock, at sps matched filter
// samples per symbol.
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
	if x.np != nil && c.anchor != 0 {
		x.np.fitSync(c, c.anchor, c.base)
	}
	// The decode of Tier 0 first; the stages then replace the fields that
	// they decode better.
	cc, ccOK := c.colorCode()
	ch := &x.ch[b.Slot&1]
	x.placeLate(c, ch, b, cc, ccOK)
	c.repair(&c.fec, b, cc, ccOK)
	out := &b.Fixed
	if x.coh != nil {
		// With a budget, a data block that passes its check from the core
		// takes none of it.
		s := x.coh
		s.fitOnly = s.budget > 0 && x.checked(b) || s.voiceOnly && !isVoiceBurst(b)
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
		if x.stages&extAMBE != 0 {
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

// cohBurst decodes burst b of the core c from its coherent detection, with
// the color code cc of the stream when ccOK, and reports whether the stage
// detected it.
func (x *softExt) cohBurst(c *leanCore, b *Burst, ch *extChan, cc enums.ColorCode, ccOK bool) bool {
	if !x.coh.detect(c, b, &x.llr) {
		return false
	}
	fixed, data := b.Fixed, b.Data
	x.repairAll(c, b, cc, ccOK)
	coh := b.Data
	if x.voiceKnown(b) && !x.coh.easy && x.coh.refine(&b.Fixed, &x.known, &x.llr) {
		x.repairAll(c, b, cc, ccOK)
	} else {
		x.dataPasses(c, b, cc, ccOK)
	}
	x.uncoded(b, &data, &coh, true)
	if x.bptcRetry(b) && x.affords(extCoreCredit) {
		x.coreData(c, b, fixed, data)
	}
	x.coh.trackBurst(&b.Fixed)
	hold := false
	if _, kind, known := layer2.SyncSourceOf(b.Sync); b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice {
		hold = x.fragment(ch, b)
	} else {
		ch.have, ch.accN = 0, 0
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
	// The last burst waits for the run from the SYNC of the next.
	x.drain(c, false, 1)
	return true
}

// data decodes the data burst out of core c again: a BPTC(196,96) block
// whose check fails by ordered statistics and over its grid, and with np a
// rate 3/4 block from its ratios, which the core decodes from those of
// each symbol alone.
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

// decodeBPTC decodes the BPTC(196,96) block of data type dt of out from
// x.llr by ordered statistics and, when its check fails, again over its
// grid, which leaves the decode by ordered statistics when it fails too.
// It reports false when both fail, and adds their cost to x.work.
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

// The costs of the decodes from ratios, in trellis steps of the coherent
// stage, that the profile of a weak stream measures: the AMBE+2 decode of
// a voice burst, the ordered statistics decode of a BPTC(196,96) block
// that its hard decisions do not decode, and the grid decode of one.
const (
	extWorkAMBE = 70
	extWorkOSD  = 300
	extWorkGrid = 120
)

// back decodes burst j of the queue again, when coh detected it, with the
// ratios of its second half averaged with those of the run from the SYNC
// of the burst after it. The embedded LC that its superframe decoded
// stays.
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
		// A data block that passed its check keeps that decode.
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

// Settings of dataPasses: the number of passes and the weight of the
// extrinsic ratios in the priors, as the engine of Tier3 has them.
const (
	cohDataPasses = 2
	cohDataPrior  = 0.7
)

// dataPasses detects a BPTC(196,96) data burst b whose block fails its
// check, or without one needed corrections, again with the extrinsic
// ratios of its grid decode as priors, up to x.passes times, as
// the engine of Tier3 does, and decodes it from the new ratios.
func (x *softExt) dataPasses(c *leanCore, b *Burst, cc enums.ColorCode, ccOK bool) {
	ch := x.llr
	// The passes take their priors from the BPTC block alone, so the slot
	// type keeps the decode of the first detection: the priors of a block
	// that does not decode mislead the detection of the bits beside it.
	fixed, data, st := b.Fixed, b.Data, ch
	for range x.passes {
		if !x.bptcRetry(b) || !x.affords(extPassCredit) {
			break
		}
		x.soft.bptcExtrinsic(&ch, &x.ext)
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
	// Nor do the priors of a block that does not decode carry into the
	// runs of the next burst.
	if x.bptcRetry(b) {
		x.coh.mineOK = false
	}
	// The 20 bits of the slot type are the 10 on each side of the SYNC.
	for i := range 20 {
		q := 98 + i + i/10*48
		m := byte(0x80) >> (q % 8)
		b.Fixed[q/8] = b.Fixed[q/8]&^m | fixed[q/8]&m
		b.Data[q/8] = b.Data[q/8]&^m | data[q/8]&m
		x.llr[q] = st[q]
	}
}

// coreData decodes the data burst b, whose block failed its check after
// the coherent detection, again from fixed and data, the decode of the
// core, as a tier without the coherent stage does, and keeps that when its
// block passes the check: the carrier and the timing of the coherent
// detection start from the first SYNC fits of a stream. The ratios of the
// coherent detection stay for the detection of the bursts after it.
//
// It is not inlined, so that its copies are on the stack only while it
// runs.
//
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

// affords reports whether the credit of the coherent stage, less the work
// of the burst under decode so far, holds n slots of its budget, or the
// stage has no budget. A weak stream that spends its credit decodes each
// data burst once from its coherent detection: the detections again and
// the decode from the ratios of the core cost the most, for the least.
func (x *softExt) affords(n float64) bool {
	s := x.coh
	return s.budget <= 0 || s.credit-float64(s.work+x.work-x.work0) >= n*s.budget
}

// The credit, in slots of budget, that a detection again of a data burst
// and a decode from the ratios of the core need.
const (
	extPassCredit = 2
	extCoreCredit = 3
)

// checked reports whether burst b is a BPTC(196,96) data burst whose block
// passes its check.
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

// bptcRetry reports whether burst b is a BPTC(196,96) data burst whose
// block fails its check, or has none and differs from its hard decisions.
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

// isVoiceBurst reports whether b is a voice burst, by its SYNC or its EMB.
func isVoiceBurst(b *Burst) bool {
	_, kind, ok := layer2.SyncSourceOf(b.Sync)
	return b.Voice != enums.VoiceBurstUnknown || ok && kind == layer2.KindVoice
}

// voiceKnown marks in known the bits of voice burst b that its FEC fixed:
// C0 and C1 of its three frames, its EMB but in burst A, and its SYNC in
// burst A. It reports false for a burst that is not voice.
func (x *softExt) voiceKnown(b *Burst) bool {
	_, kind, ok := layer2.SyncSourceOf(b.Sync)
	if b.Voice == enums.VoiceBurstUnknown && (!ok || kind != layer2.KindVoice) {
		return false
	}
	clear(x.known[:])
	for id := softAMBECode; id < softSmallCount; id++ {
		c := &softSmallCodes[id]
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

// repairAll sets the bits of burst b to the hard decisions of llr and
// decodes every FEC field from llr with the soft decoders, with the color
// code cc of the stream when ccOK; the CACH keeps the decode of the core.
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
		x.soft.decodeAMBE(&x.llr, out)
		x.work += extWorkAMBE
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

// fill sets llr to the log-likelihood ratios of the burst under detection,
// whose hard decisions are hard. A ratio of 0, where the floor of the
// likelihoods holds both sides, takes the sign of the hard decision: the
// decoders read the bits that no code covers from the signs.
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

// fragment keeps the embedded signalling fragment of voice burst b of
// channel ch, and reports whether b waits for burst E of its superframe.
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

// embeddedLC decodes the embedded LC of the superframe of ch whose burst E
// is in slot e, and writes the corrected fragments into its bursts.
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
	if ch.accN > 0 {
		for i := range x.sum {
			x.sum[i] = ch.acc[i]
			softCombine(x.sum[i][:], ch.frag[i][:])
		}
		if _, res := x.soft.decodeEmbeddedFrags(&x.sum, &x.out); !res.Uncorrectable {
			ok = true
			ch.acc, ch.accN = x.sum, min(ch.accN+1, maxCombine)
		}
	}
	if !ok {
		_, res := x.soft.decodeEmbeddedFrags(&ch.frag, &x.out)
		ok = !res.Uncorrectable
		ch.acc, ch.accN = ch.frag, 1
	}
	for i, b := range ps {
		if ok {
			b.Fixed = x.out[i]
		}
	}
	x.release(ch)
}

// release lets the held bursts of the channel of ch go.
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

// push adds burst b to the queue, held when hold, emitting the oldest
// burst when the queue is full.
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

// drain emits the bursts at the head of the queue that wait for nothing,
// or only the head when one is true.
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

// unplaced keeps burst b with the symbols of the core, the last extLate
// of its channel, so that a voice burst of the channel after it places it:
// a receiver that comes into a call in its superframe places by its EMB
// only the bursts B and E, and the bursts before them would be lost.
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

// placeLate places the bursts of ch that the core could not place before
// burst b, which it placed: when b is a voice burst, each as the burst of
// the superframe of b, or of the one before for a burst A, that its slot
// gives. It detects each again, with coh from its SYNC or the EMB of its
// place, or from its symbols with the levels of the core now, decodes it
// and emits it after the bursts of the other channel that came since.
// The emitted bursts carry no CACH, which would come out of order.
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
	// The bursts of a superframe whose burst A is b are those of the
	// superframe before.
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
			lb.Voice, lb.Sync, lb.Distance = enums.VoiceBurstUnknown, voice, syncDist(lb.Data, voice)
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

// latePos returns the place of the burst of slot s in the superframe of
// the voice burst at place pos of slot j, A to F, or past F for the one
// before a burst A, and whether it has one.
func latePos(pos enums.VoiceBurstType, j, s int64) (enums.VoiceBurstType, bool) {
	for v, k := pos-1, j-2; v >= enums.VoiceBurstA && k >= s; v, k = v-1, k-2 {
		if k == s {
			return v, true
		}
	}
	return 0, false
}

// lateLCSS returns the LCSS of the EMB of voice burst v, B to F, in a
// call whose embedded signalling carries the LC.
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

// cohLate detects late burst lb with coh into llr, and keeps the state of
// coh, which follows the bursts in order.
//
// It is not inlined, so that the copy of the state is on the stack only
// while it runs.
//
//go:noinline
func (x *softExt) cohLate(c *leanCore, lb *Burst) bool {
	saved := *x.coh
	x.coh.fitOnly = false
	ok := x.coh.detect(c, lb, &x.llr)
	gen, stamp := x.coh.gen, x.coh.symStamp
	*x.coh = saved
	// The samples that the stage keeps are those of lb now.
	x.coh.gen, x.coh.symStamp, x.coh.symKey = gen+1, stamp+1, cohSymKey{}
	return ok
}

func (x *softExt) place(c *leanCore, b *Burst, j int32) bool {
	return x.coh != nil && x.coh.place(c, b, j)
}

// scan scans with coh on I/Q. On audio the scan finds no SYNC that the
// search of the core with verify misses, at about a tenth of the time of
// the search.
func (x *softExt) scan(c *leanCore) (int, bool, int64, bool) {
	if x.coh == nil || x.ph != nil {
		return 0, false, 0, false
	}
	return x.coh.scan(c)
}

// verify tests the SYNC that the core does not find coherently, but not
// one with more than a quarter of its symbols at the limits of the ring:
// the front end scaled it for a weaker input, as clipped finds, and the
// stream that the core then locks to keeps that scale. The coherent test
// sees the phase, which the clipping keeps.
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

// locked notes the time of the SYNC that the core locked to, d samples
// from the timing loop, in the kept samples, which the core stops keeping
// unless it hunts.
func (x *softExt) locked(c *leanCore, d float32) {
	x.lockSym, x.lockAt, x.lockN = c.ns-1, c.lastAt+d, c.n
	if x.coh != nil {
		// A new stream may come from another transmitter, at another
		// carrier offset.
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

// firstSlot decodes the lookback slots before the burst of the SYNC that
// the core locked to, as that burst ends: it samples their symbols from
// the kept samples at the timing of the SYNC and runs each through the
// slot handling of the core, as if the core had been locked there. A
// first run only votes for the color code, with the burst of the SYNC
// too, so that the voice bursts without a SYNC place by their EMB; the
// second decodes. The slots of the stream count from the first of them.
func (x *softExt) firstSlot(c *leanCore) {
	// A lock that a stage forced on a SYNC field that the levels do not
	// fit, at the least deviation scale, is mostly one on noise. Its slots
	// would show the noise before it at that scale, all clicks, and the
	// SYNC distance of noise passes a burst A now and then.
	if x.lookback == 0 || c.forced && c.devScale <= leanMinScale {
		return
	}
	sps := 2 * c.tm.step
	last := x.lockSym
	// A lock that a stage forced hunts from the start, and keeps the
	// samples after the SYNC too.
	at := x.lockAt
	if c.hunt && c.acqFrom <= x.lockN {
		at -= float32(c.n - x.lockN)
	}
	// The oldest symbol that a slot reads, one before it for the SYNC
	// search, must lie in the kept samples with the four of the
	// interpolation.
	room := float32(c.acqKept()-4) - at
	n := 0
	for n < x.lookback {
		oldest := c.first - int64(n+1)*slotSymbols - 1
		if oldest < 0 || float32(last-oldest)*sps > room {
			break
		}
		n++
	}
	if n == 0 {
		return
	}
	x.ring = c.ring
	first, eAcc, eSyms, hold, slotEnd := c.first, c.eAcc, c.eSyms, c.tm.hold, c.slotEnd
	levels := [3]float32{c.dcHz, c.devScale, c.discVar}
	// The slots before the lock are mostly noise, whose clicks would floor
	// the ratios of the bursts of the stream.
	clicks := c.clicks
	c.replaying = true
	for _, dry := range [2]bool{true, false} {
		votes := c.votes
		c.resetChans()
		c.votes = votes
		c.dcHz, c.devScale, c.discVar, c.clicks = levels[0], levels[1], levels[2], clicks
		c.next, c.lastAnch, c.lost = 0, leanNoSlot, 0
		if !c.bs {
			// An MS sends in the slots of the SYNC of the lock, two
			// apart: a SYNC that noise shows in another would take them.
			c.lastAnch = int32(n)
		}
		c.first = first - int64(n)*slotSymbols
		c.slotEnd = c.first + slotSymbols
		c.replayDry = dry
		// The first run also votes with the slot of the SYNC, whose
		// symbols the ring holds.
		m := n
		if dry {
			m++
		}
		for j := range m {
			if j == n {
				c.ring = x.ring
			} else {
				base := c.first + int64(j)*slotSymbols
				for k := base - 1; k <= base+slotSymbols; k++ {
					v := c.keptAt(at - float32(last-k)*sps)
					c.ring[k%leanRing] = int8(max(min(roundf(v), math.MaxInt8), math.MinInt8))
				}
			}
			c.eAcc, c.eSyms = 0, 0
			c.slot()
		}
	}
	c.replaying, c.replayDry = false, false
	if c.lastAnch == int32(n) {
		// No slot before the lock anchored.
		c.lastAnch = leanNoSlot
	}
	c.clicks = clicks
	c.ring = x.ring
	c.first = first - int64(n)*slotSymbols
	c.slotEnd = slotEnd
	c.eAcc, c.eSyms, c.tm.hold = eAcc, eSyms, hold
}

func (x *softExt) end(c *leanCore) {
	for i := range x.held {
		x.held[i] = false
	}
	x.ch[0].have, x.ch[1].have = 0, 0
	x.ch[0].accN, x.ch[1].accN = 0, 0
	x.ch[0].lateN, x.ch[1].lateN = 0, 0
	x.drain(c, false, 0)
}

// extErrGain is the weight of each voice frame in the mean numbers of bit
// errors of uncoded, and extCoreShare the most share of the mean of coh
// that the mean of the core may reach for uncoded to take its bits. A frame
// counts in the means only when the hard decisions of the core or of coh
// differ from its decode in at most extErrMax bits of C0 and C1, the
// errors that their Golay codes correct together: otherwise the decode is
// mostly wrong, as for a burst that the core placed by an EMB that noise
// passed after the end of a transmission, and the counts are those of
// noise against noise, in which the core made twice the errors of coh.
const (
	extErrGain   = 1.0 / 16
	extCoreShare = 0.9
	extErrMax    = 6
)

// uncoded sets the bits without FEC of burst b, which coh detected, to the
// hard decisions of the core where the FEC shows that the core makes fewer
// errors: the core detects each symbol from the frequency alone, while coh
// follows the phase of the carrier, which a fast fade turns too quickly
// for it, so that it makes errors that FEC corrects in C0 and C1 of a
// voice frame but not in its uncoded bits C2 and C3, or in a rate 1 data
// block. While the hard decisions of C0 and C1 of the core differ less
// from their decode than those of coh do over the frames before, by
// coreCleaner, a voice frame takes C2 and C3 of the core, unless its own
// C0 and C1 of the core differ more, and a rate 1 data burst takes its
// payload of the core. The
// counts of one frame alone do not decide: in noise the errors of each
// frame are a draw of the same rate, so that they say little of those in
// C2 and C3. core and coh hold the hard decisions of the core and of coh;
// update adds the frames of b to the means.
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
	if b.Voice == enums.VoiceBurstUnknown && (!known || kind != layer2.KindVoice) {
		return
	}
	var nCore, nCoh [layer2.VoiceSlotsPerBurst]int
	for i := range layer2.VoiceSlotsPerBurst {
		for id := softAMBECode + 2*softSmallID(i); id <= softAMBECode+2*softSmallID(i)+1; id++ {
			c := &softSmallCodes[id]
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

// coreCleaner reports whether the mean number of errors of the core is
// under extCoreShare of that of coh. On a clean signal both means hold the
// same few errors: on the XPR 8400 recordings a core ahead by 3% took C2
// and C3 wrong. Through a fade the mean of the core is mostly under a half
// of that of coh.
func (x *softExt) coreCleaner() bool {
	return x.errCore < extCoreShare*x.errCoh
}

// extPayloadHalf is the number of payload bits of a data burst on each
// side of its slot type and SYNC.
const extPayloadHalf = 98
