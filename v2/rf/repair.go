package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type repairer struct {
	osd            bool
	grid           bool
	fixed          layer2.RawBurst
	bptcRetry      bool
	retryUnchecked bool
	soft           *softDecoder
	checks         [16]func([12]byte) bool
	cur            [4][embBits]int16
	llr            [layer2.BurstBits]int16
	sum            [4][embBits]int16
	bursts         [4]layer2.RawBurst
	raw            bool
}

const (
	embFirstBit = 116
	embBits     = 32
)

const maxCombine = 4

func newRepairer(osd, grid bool) *repairer {
	p := &repairer{soft: newSoftDecoder(), osd: osd, grid: grid}
	for dt := range p.checks {
		p.checks[dt] = softBPTCCheck(enums.DataType(dt))
	}
	return p
}

func (p *repairer) repair(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	if p == nil {
		return
	}
	out := &e.out
	if !out.hasFixed {
		out.fixed, out.fixedCACH, out.hasFixed = out.data, out.cach, true
	}
	e.rec.llr.get(&p.llr)
	llr := &p.llr
	p.bptcRetry = false
	_, kind, known := layer2.SyncSourceOf(out.sync)
	switch {
	case out.voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if !p.raw {
			p.soft.decodeAMBE(llr, &out.fixed)
		}
		if out.voice != enums.VoiceBurstUnknown && out.voice != enums.VoiceBurstA {
			p.soft.decodeEMB(llr, cc, ccOK, &out.fixed)
		}
	case known && kind == layer2.KindData:
		st, res := p.soft.decodeSlotType(llr, cc, ccOK, &out.fixed)
		if res.Uncorrectable {
			break
		}
		switch dt := st.DataType(); dt {
		case enums.DataTypeRate34:
			p.soft.decodeRate34(llr, &out.fixed)
		case enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		default:
			check := p.checks[dt&0xF]
			var r fec.Result
			if p.osd {
				_, r = p.soft.decodeBPTCOSD(llr, check, &out.fixed)
			} else {
				_, r = p.soft.decodeBPTCChase(llr, check, &out.fixed)
			}
			if p.grid && check != nil && r.Uncorrectable {
				p.fixed = out.fixed
				if _, r = p.soft.decodeBPTCGrid(llr, check, &out.fixed); r.Uncorrectable {
					out.fixed = p.fixed
				}
			}
			p.bptcRetry = check != nil && r.Uncorrectable || check == nil && p.retryUnchecked && r.ErrorsCorrected > 0
		}
	}
	if known && e.rec.anchor == out.sync {
		_ = out.fixed.SetSync(out.sync)
	}
	if out.hasCACH {
		p.soft.decodeTACT(&e.rec.cachLLR, &out.fixedCACH)
	}
}

func (p *repairer) voiceBits(e *trackEntry, known *[layer2.BurstBits]bool) bool {
	clear(known[:])
	out := &e.out
	_, kind, ok := layer2.SyncSourceOf(out.sync)
	if out.voice == enums.VoiceBurstUnknown && (!ok || kind != layer2.KindVoice) || p.raw {
		return false
	}
	for id := softAMBECode; id < softSmallCount; id++ {
		c := smallCode(id)
		for _, q := range c.pos[:c.n] {
			known[q] = true
		}
	}
	if out.voice != enums.VoiceBurstUnknown && out.voice != enums.VoiceBurstA {
		c := &softSmallCodes[softEMBCode]
		for _, q := range c.pos[:c.n] {
			known[q] = true
		}
	}
	if e.elcOK {
		for q := embFirstBit; q < embFirstBit+embBits; q++ {
			known[q] = true
		}
	}
	return true
}

func (p *repairer) embeddedLC(c *chanTrack, es *[4]*trackEntry) {
	if p == nil {
		return
	}
	for i, e := range es {
		e.rec.llr.get(&p.llr)
		copy(p.cur[i][:], p.llr[embFirstBit:embFirstBit+embBits])
		if !e.out.hasFixed {
			e.out.fixed, e.out.fixedCACH, e.out.hasFixed = e.out.data, e.out.cach, true
		}
		p.bursts[i] = e.out.fixed
	}
	ok := false
	var lc [9]byte
	if c.accN > 0 {
		for i := range p.sum {
			p.sum[i] = c.acc[i]
			softCombine(p.sum[i][:], p.cur[i][:])
		}
		l, res := p.soft.decodeEmbeddedFrags(&p.sum, &p.bursts)
		if ok = !res.Uncorrectable; ok {
			lc = l
			c.acc = p.sum
			c.accN = min(c.accN+1, maxCombine)
		}
	}
	if !ok {
		l, res := p.soft.decodeEmbeddedFrags(&p.cur, &p.bursts)
		ok, lc = !res.Uncorrectable, l
		c.acc, c.accN = p.cur, 1
	}
	if !ok {
		return
	}
	if r, voice := rawVoiceLC(lc[:]); voice {
		c.raw = r
	}
	for i, e := range es {
		e.out.fixed = p.bursts[i]
		e.elcOK = true
	}
}
