//go:build !tinygo

package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const discDefaultTier = Tier1

func (r *discRx) addSoft(opt *options) error {
	r.addExt(opt)
	return nil
}

func (r *discRx) addExt(opt *options) {
	var ph *discPhase
	if opt.CohExt {
		hz := r.hz()
		r.setup(discSetup{Hz: discRate, OnBurst: r.onBurst, opt: opt, pol: r.f.polBits&polOn != 0}, r.f.dec)
		r.lagTicks -= int32(math.Round(float64(discRate-hz) * 256 / float64(hz)))
		ph = newDiscPhase(r, hz, opt.cohRing())
	}
	x := softExtOf(opt, 2*r.tm.step)
	if ph != nil {
		x.ph = ph
		x.setCoh(newCohStageOf(opt))
		x.coh.src, x.coh.ph, ph.coh = ph, ph, x.coh
		ph.useHeld = opt.CohExact
		x.coh.easySNR = discEasySNR
		r.signMiss = opt.signMiss()
	}
	r.ext = x
}

func (r *discRx) attachFix(restore bool) {
	e, _ := r.ext.(*softExt)
	if !restore || e == nil {
		return
	}
	a := newACStage(r.inputHz())
	e.ac = a
	on := r.onBurst
	if on == nil {
		on = r.sink.put
	}
	r.onBurst = func(b *Burst) {
		a.stamp(b)
		on(b)
	}
}

func (r *discRx) fixLag() float64 {
	if e, _ := r.ext.(*softExt); e != nil && e.ac != nil {
		return e.ac.lag
	}
	return 0
}

func (r *discRx) write(x []int16) {
	if e, _ := r.ext.(*softExt); e != nil && e.ac != nil {
		e.ac.write(r, x)
		return
	}
	r.writeCore(x)
}

func (r *discRx) writeN(x []int16) int {
	if e, _ := r.ext.(*softExt); e != nil && (e.ac != nil || e.ph != nil) {
		r.write(x)
		return len(x)
	}
	return r.runN(x)
}

func (r *discRx) writeIQ(x []int16) { r.write(x) }

func (r *discRx) writeCore(x []int16) {
	e, _ := r.ext.(*softExt)
	if e == nil || e.ph == nil {
		r.run(x)
		return
	}
	for len(x) > 0 {
		n, y := e.ph.input(x)
		r.run(y)
		x = x[n:]
	}
}

func (r *discRx) inputHz() int {
	if e, _ := r.ext.(*softExt); e != nil && e.ph != nil {
		return e.ph.inputHz()
	}
	return r.hz()
}

func (r *leanCore) hasExt() bool { return r.ext != nil }

func (r *leanCore) isReplaying() bool { return r.replaying }

func (r *leanCore) isReplayDry() bool { return r.replayDry }

func (r *leanCore) cohVerify() bool {
	x, ok := r.ext.(*softExt)
	return ok && x.coh != nil
}

func (r *leanCore) earlyOK(b *Burst) bool { return checkedData(b) }

// checkedData reports whether a burst is a data burst whose slot type
// decodes clean, whose BPTC(196,96) block is a codeword, and whose type
// carries a CRC that passes. Only such a burst may start before the input:
// a guessed symbol there cannot get past the CRC.
func checkedData(b *Burst) bool {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	if !known || kind != layer2.KindData {
		return false
	}
	d, _ := b.Decoded()
	st := d.SlotType()
	if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected != 0 {
		return false
	}
	info, _, res := bptc.Decode196(d.Info())
	if res.Uncorrectable || res.ErrorsCorrected != 0 {
		return false
	}
	ok, checked := bptcCheck(st.DataType, &info)
	return ok && checked
}
