package rf

import (
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const awarePath = "testdata/rxbench_aware.json"

type awareState struct {
	dec  *softDecoder
	acc  [4][embBits]int16
	accN int
	cur  [4][embBits]int16
	ext  [softBurstBits]float32
	ch   [softBurstBits]int16
}

func awareDescriptions() map[string]string {
	const det = "exact bitwise detection of the payload with the genie, then "
	return map[string]string{
		"ambe":      det + "the AMBE+2 decoder of the tiers",
		"ambe_c0c1": det + "the AMBE+2 decoder of the tiers, the genie also giving C2 and C3",
		"rate12":    det + "the BPTC decoder of the tiers, with their two passes of detection again with grid priors",
		"rate34":    det + "the rate 3/4 decoder of the tiers",
		"rate1":     det + "hard decisions",
		"csbk":      det + "the BPTC decoder of the tiers, with their two passes of detection again with grid priors, and the slot type decoder",
		"lc":        det + "the BPTC decoder of the tiers, with their two passes of detection again with grid priors, and the slot type decoder",
		"header":    det + "the BPTC decoder of the tiers, with their two passes of detection again with grid priors, and the slot type decoder",
		"elc":       det + "the embedded LC decoder of the tiers, which sums the ratios of up to 4 superframes",
		"emb":       det + "the EMB decoder of the tiers with the color code of the stream",
		"slottype":  det + "the slot type decoder of the tiers with the color code of the stream",
		"sync":      "maximum likelihood between the voice and the data SYNC",
		"rate1_bit": "exact bitwise detection of the rate 1 payload bits",
	}
}

func (e *floorEval) knownLLRs(i int) {
	tb := e.truth[i].burst
	for p := range softBurstBits {
		if e.free[p] {
			continue
		}
		e.llr[p] = 32000
		if tb[p/8]&(0x80>>(p%8)) != 0 {
			e.llr[p] = -32000
		}
	}
}

func (e *floorEval) awareDetect(i int, pos ...[]int16) {
	e.setFree(pos...)
	e.detect(i, false)
	e.knownLLRs(i)
}

func (e *floorEval) awareStream() {
	e.sc = map[string]*benchScore{}
	e.conv = map[string][]float64{}
	if e.aw == nil {
		dec := newSoftDecoder()
		e.aw = &awareState{dec: dec}
	}
	e.aw.accN = 0
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
			e.awareVoice(i, cc)
		case "data":
			e.awareData(i, cc)
		}
	}
}

func (e *floorEval) awareVoice(i int, cc enums.ColorCode) {
	ts := e.truth[i]
	d := e.aw.dec
	voice, _, _ := layer2.SyncSourceBS.SyncPatterns()
	want, _ := ts.burst.AMBEFrames()
	for f := range layer2.VoiceSlotsPerBurst {
		sl := softAMBESlot(f)
		if e.want("ambe") {
			got := e.awareAMBE(i, f, sl.id0.bits(), sl.id1.bits(), sl.c2[:])
			e.add("ambe", got == want[f])
		}
		if e.want("ambe_c0c1") {
			got := e.awareAMBE(i, f, sl.id0.bits(), sl.id1.bits())
			e.add("ambe_c0c1", slices.Equal(got[:3], want[f][:3]))
		}
	}
	if ts.pos == enums.VoiceBurstA {
		if e.want("sync") {
			e.add("sync", e.syncOK(i, voice))
		}
		return
	}
	if e.want("emb") {
		e.awareDetect(i, softEMBCode.bits())
		out := ts.burst
		d.decodeEMB(&e.llr, cc, true, &out)
		g, w := out.EMB(), ts.burst.EMB()
		e.add("emb", !g.FEC.Uncorrectable && g.ColorCode == w.ColorCode && g.LCSS == w.LCSS && g.PreemptionAndPowerControlIndicator == w.PreemptionAndPowerControlIndicator)
	}
	if ts.pos >= enums.VoiceBurstB && ts.pos <= enums.VoiceBurstE && e.want("elc") {
		e.awareELC(i, ts)
	}
}

func (e *floorEval) awareAMBE(i, f int, pos ...[]int16) vocoder.AMBEFrame {
	d := e.aw.dec
	e.awareDetect(i, pos...)
	out := e.truth[i].burst
	d.decodeAMBE(&e.llr, &out)
	var pri, lapp [softBurstBits]float32
	w := tierOptions(Tier3).RefinePrior
	sl := softAMBESlot(f)
	for _, ps := range [][]int16{sl.id0.bits(), sl.id1.bits()} {
		for _, p := range ps {
			pri[p] = w
			if out[p/8]&(0x80>>(p%8)) != 0 {
				pri[p] = -w
			}
		}
	}
	e.detectSoft(i, &pri, &lapp)
	for p := range softBurstBits {
		if e.free[p] {
			e.llr[p] = toLLR(lapp[p])
		}
	}
	out = e.truth[i].burst
	d.decodeAMBE(&e.llr, &out)
	got, _ := out.AMBEFrames()
	return got[f]
}

func (e *floorEval) awareELC(i int, ts benchSlot) {
	a := e.aw
	e.awareDetect(i, seqInts(embFirstBit, embBits))
	b := int(ts.pos - enums.VoiceBurstB)
	copy(a.cur[b][:], e.llr[embFirstBit:embFirstBit+embBits])
	if ts.pos != enums.VoiceBurstE {
		return
	}
	var bursts, tb [4]layer2.RawBurst
	for k := range bursts {
		tb[k] = e.truth[i-2*(3-k)].burst
	}
	ok := false
	if a.accN > 0 {
		sum := a.acc
		for k := range sum {
			softCombine(sum[k][:], a.cur[k][:])
		}
		bursts = tb
		if _, res := a.dec.decodeEmbeddedFrags(&sum, &bursts); !res.Uncorrectable {
			ok = true
			a.acc, a.accN = sum, min(a.accN+1, maxCombine)
		}
	}
	if !ok {
		bursts = tb
		_, res := a.dec.decodeEmbeddedFrags(&a.cur, &bursts)
		ok = !res.Uncorrectable
		a.acc, a.accN = a.cur, 1
	}
	var gf, wf [4][4]byte
	for k := range gf {
		gf[k], wf[k] = bursts[k].EmbeddedData(), tb[k].EmbeddedData()
	}
	wantLC, _ := strictEmbeddedLC(wf)
	lc, lcOK := strictEmbeddedLC(gf)
	e.add("elc", ok && lcOK && lc == wantLC)
}

func (e *floorEval) awareData(i int, cc enums.ColorCode) {
	ts := e.truth[i]
	d := e.aw.dec
	_, dsync, _ := layer2.SyncSourceBS.SyncPatterns()
	k := benchDataKey(ts.dt)
	e.awareDetect(i, softSlotTypeCode.bits())
	out := ts.burst
	d.decodeSlotType(&e.llr, cc, true, &out)
	okST := out.SlotType() == ts.burst.SlotType()
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
		e.awareDetect(i, softTrellisTables.pos[:])
		out := ts.burst
		d.decodeRate34(&e.llr, &out)
		a, _ := out.Rate34Payload()
		b, _ := ts.burst.Rate34Payload()
		ok = a == b
	case enums.DataTypeRate1:
		ok = e.awareRate1(i)
	default:
		ok = e.awareBPTC(i, ts.dt)
	}
	e.add(k, ok && okST)
}

func (e *floorEval) awareBPTC(i int, dt enums.DataType) bool {
	const passes, w = 2, 0.7
	a := e.aw
	d := a.dec
	check := softBPTCCheck(dt)
	tb := e.truth[i].burst
	want, _ := tb.BPTCPayload()
	var free []int16
	for j := range softBPTCCode.code().n {
		r := false
		for x := softBPTCData; x < softBPTCInfo; x++ {
			r = r || int(softBPTCCode.code().sys[x]) == j
		}
		if !r {
			free = append(free, softBPTCCode.code().pos[j])
		}
	}
	e.setFree(free)
	var pri, lapp [softBurstBits]float32
	decode := func() (bool, bool) {
		out := tb
		res, r := d.decodeBPTCOSD(&e.llr, check, &out)
		if check != nil && r.Uncorrectable {
			if res2, r2 := d.decodeBPTCGrid(&e.llr, check, &out); !r2.Uncorrectable {
				res, r = res2, r2
			}
		}
		retry := check != nil && r.Uncorrectable || check == nil && r.ErrorsCorrected > 0
		return res.Payload == want, retry
	}
	e.detectSoft(i, &pri, &lapp)
	for _, p := range free {
		e.llr[p] = toLLR(lapp[p])
	}
	e.knownLLRs(i)
	ok, retry := decode()
	a.ch = e.llr
	for range passes {
		if !retry {
			break
		}
		d.bptcExtrinsic(&a.ch, &a.ext)
		for _, p := range free {
			pri[p] = w * a.ext[p]
		}
		e.detectSoft(i, &pri, &lapp)
		for _, p := range free {
			c := lapp[p] - pri[p]
			a.ch[p] = toLLR(c)
			e.llr[p] = toLLR(c + a.ext[p])
		}
		ok, retry = decode()
	}
	return ok
}

func (e *floorEval) awareRate1(i int) bool {
	if e.g2 == nil {
		e.g2 = newGenieTrellis(2, 2*slotSymbols)
	}
	e.g = e.g2
	defer func() { e.g = e.g1 }()
	tb := e.truth[i].burst
	e.setFree(e.r1pos[:])
	e.detect(i, false)
	ok := true
	for _, p := range e.r1pos {
		good := (e.llr[p] < 0) == (tb[p/8]&(0x80>>(p%8)) != 0)
		e.add("rate1_bit", good)
		ok = ok && good
	}
	return ok
}
