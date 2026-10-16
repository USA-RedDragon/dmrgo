package main

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// repairer rewrites the FEC-coded fields of a burst with the codeword that
// soft-decision decoding of its log-likelihood ratios picks, so that the
// library's hard decoders see a clean codeword.
type repairer struct {
	osd bool
	// grid decodes a checked BPTC(196,96) block that fails its check again
	// with decodeBPTCGrid.
	grid   bool
	fixed  layer2.RawBurst
	soft   *softDecoder
	checks [16]func([12]byte) bool
	cur    [4][embBits]int16
	// llr holds the ratios of the burst under repair.
	llr    [layer2.BurstBits]int16
	sum    [4][embBits]int16
	bursts [4]layer2.RawBurst
}

// Bits of the embedded signalling fragment of a voice burst B to F.
const (
	embFirstBit = 116
	embBits     = 32
)

// maxCombine is the most superframes whose embedded LC log-likelihood
// ratios the repair sums.
const maxCombine = 4

// newRepairer returns a repairer that decodes BPTC(196,96) by ordered
// statistics when osd, and again by iterative decoding of its grid when
// grid and the check fails, or nil when the soft decoders cannot be built.
func newRepairer(osd, grid bool) *repairer {
	d, err := newSoftDecoder()
	if err != nil {
		return nil
	}
	p := &repairer{soft: d, osd: osd, grid: grid}
	for dt := range p.checks {
		p.checks[dt] = softBPTCCheck(enums.DataType(dt))
	}
	return p
}

// repair rewrites the fields of the burst of entry e by its kind, with the
// color code cc of the stream when ccOK. The payload of an idle burst
// carries nothing, so only its slot type is decoded.
func (p *repairer) repair(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	if p == nil {
		return
	}
	out := &e.out
	if !out.HasFixed {
		out.Fixed, out.FixedCACH, out.HasFixed = out.Data, out.CACH, true
	}
	e.rec.llr.get(&p.llr)
	llr := &p.llr
	_, kind, known := layer2.SyncSourceOf(out.Sync)
	switch {
	case out.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		p.soft.decodeAMBE(llr, &out.Fixed)
		if out.Voice != enums.VoiceBurstUnknown && out.Voice != enums.VoiceBurstA {
			p.soft.decodeEMB(llr, cc, ccOK, &out.Fixed)
		}
	case known && kind == layer2.KindData:
		st, res := p.soft.decodeSlotType(llr, cc, ccOK, &out.Fixed)
		if res.Uncorrectable {
			break
		}
		switch dt := st.DataType(); dt {
		case enums.DataTypeRate34:
			p.soft.decodeRate34(llr, &out.Fixed)
		case enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		default:
			check := p.checks[dt&0xF]
			var r fec.Result
			if p.osd {
				_, r = p.soft.decodeBPTCOSD(llr, check, &out.Fixed)
			} else {
				_, r = p.soft.decodeBPTCChase(llr, check, &out.Fixed)
			}
			if p.grid && check != nil && r.Uncorrectable {
				p.fixed = out.Fixed
				if _, r = p.soft.decodeBPTCGrid(llr, check, &out.Fixed); r.Uncorrectable {
					out.Fixed = p.fixed
				}
			}
		}
	}
	if known && e.rec.anchor == out.Sync {
		_ = out.Fixed.SetSync(out.Sync)
	}
	if out.HasCACH {
		p.soft.decodeTACT(&e.rec.cachLLR, &out.FixedCACH)
	}
}

// voiceBits marks the burst bits of entry e that the repair fixed: C0 and
// C1 of its three AMBE+2 frames, the EMB of bursts B to F, the SYNC of a
// burst A. It reports false for a burst that is not voice.
func (p *repairer) voiceBits(e *trackEntry, known *[layer2.BurstBits]bool) bool {
	clear(known[:])
	out := &e.out
	_, kind, ok := layer2.SyncSourceOf(out.Sync)
	if out.Voice == enums.VoiceBurstUnknown && (!ok || kind != layer2.KindVoice) {
		return false
	}
	for i := range p.soft.ambe.slot {
		sl := &p.soft.ambe.slot[i]
		for _, q := range sl.c0.pos[:sl.c0.n] {
			known[q] = true
		}
		for _, q := range sl.c1.pos[:sl.c1.n] {
			known[q] = true
		}
	}
	if out.Voice != enums.VoiceBurstUnknown && out.Voice != enums.VoiceBurstA {
		for _, q := range p.soft.emb.pos[:p.soft.emb.n] {
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

// embeddedLC decodes the embedded LC of the bursts B to E in es, with the
// log-likelihood ratios of the earlier superframes of the call in c added,
// and writes the corrected fragments into the bursts.
func (p *repairer) embeddedLC(c *chanTrack, es *[4]*trackEntry) {
	if p == nil {
		return
	}
	for i, e := range es {
		e.rec.llr.get(&p.llr)
		copy(p.cur[i][:], p.llr[embFirstBit:embFirstBit+embBits])
		if !e.out.HasFixed {
			e.out.Fixed, e.out.FixedCACH, e.out.HasFixed = e.out.Data, e.out.CACH, true
		}
		p.bursts[i] = e.out.Fixed
	}
	ok := false
	if c.accN > 0 {
		for i := range p.sum {
			p.sum[i] = c.acc[i]
			softCombine(p.sum[i][:], p.cur[i][:])
		}
		_, res := p.soft.decodeEmbeddedFrags(&p.sum, &p.bursts)
		if ok = !res.Uncorrectable; ok {
			c.acc = p.sum
			c.accN = min(c.accN+1, maxCombine)
		}
	}
	if !ok {
		_, res := p.soft.decodeEmbeddedFrags(&p.cur, &p.bursts)
		ok = !res.Uncorrectable
		c.acc, c.accN = p.cur, 1
	}
	if !ok {
		return
	}
	for i, e := range es {
		e.out.Fixed = p.bursts[i]
		e.elcOK = true
	}
}
