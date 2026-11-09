package rf

import "math"

const rxBaseChunk = 1024

type rxHost struct {
	lean   *leanRx
	iqd    *iqRx
	bb     []complex64
	rate   int
	slot   int
	format IQFormat
	base   bool
}

func newReceiver(cfg RxConfig, o *options) (*Receiver, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	low, hz, err := cfg.check(o)
	if err != nil {
		return nil, err
	}
	opt := tierOptions(cfg.Tier)
	switch {
	case low:
		opt = iqLowOptions()
	case cfg.Tier != Tier0 && cfg.LookbackSlots > 0:
		opt.ExtLookback = cfg.LookbackSlots
	}
	if o != nil {
		opt = *o
	}
	r := &Receiver{}
	if low && opt.IQLow > 0 {
		r.setLow(cfg, o)
	} else {
		r.format, r.mode = cfg.Format, rxModeFull
		r.sink.on = cfg.OnBursts
		h := &rxHost{rate: cfg.Rate, slot: hz * 3 / 100, format: cfg.Format}
		h.base = (cfg.Format == IQFormatCS16 || cfg.Format == IQFormatCF32) && cfg.Rate == hz
		if h.base {
			h.bb = make([]complex64, rxBaseChunk)
		}
		s := rxSetup{Rate: float64(cfg.Rate), Signed: cfg.Format == IQFormatCS8, Format: cfg.Format, Tier: cfg.Tier,
			BasebandHz: hz, OffsetHz: cfg.OffsetHz, sink: &r.sink, disc: &r.low.d}
		h.build(s, opt)
		r.full = h
	}
	if n := streamBursts(opt.ExtLookback); cfg.OnBursts != nil && n > 0 {
		r.sink.out = make([]Burst, 0, n)
	}
	r.core().hookShortLC(newShortLCOut(cfg.OnShortLC, cfg.Tier))
	return r, nil
}

func (r *Receiver) core() *leanCore {
	if h, ok := r.full.(*rxHost); ok && h.lean != nil {
		return &h.lean.leanCore
	}
	return &r.low.d.leanCore
}

func (r *rxHost) build(s rxSetup, opt options) {
	if opt.IQDisc && s.BasebandHz == basebandRate {
		r.iqd = newIQRx(s, opt)
		return
	}
	r.lean = newLean(s, opt)
}

func (r *rxHost) write(x []byte) {
	if !r.base {
		switch {
		case r.iqd != nil:
			r.iqd.write(x)
		case r.lean != nil:
			r.lean.write(x)
		}
		return
	}
	sb := r.format.bytes()
	for len(x) >= sb {
		m := min(len(x)/sb, len(r.bb))
		for i := range m {
			r.bb[i] = basebandSample(x[i*sb:], r.format)
		}
		r.writeBaseband(r.bb[:m])
		x = x[m*sb:]
	}
}

func (r *rxHost) writeBaseband(x []complex64) {
	switch {
	case r.iqd != nil:
		r.iqd.writeBaseband(x)
	case r.lean != nil:
		r.lean.writeBaseband(x)
	}
}

func (r *rxHost) flush() {
	switch {
	case r.iqd != nil:
		r.iqd.flush()
	case r.lean != nil:
		r.lean.flush()
	}
}

func (r *rxHost) active() bool {
	switch {
	case r.iqd != nil:
		return r.iqd.d.locked()
	case r.lean != nil:
		return r.lean.locked()
	}
	return false
}

func (r *rxHost) offsetHz() float64 {
	switch {
	case r.iqd != nil:
		if x, _ := r.iqd.d.ext.(*softExt); x != nil && x.coh != nil && x.coh.freqOK {
			return x.coh.totalHz
		}
		return float64(r.iqd.conv.hz) + r.iqd.d.offset()/iqUnitsPerHz
	case r.lean != nil:
		return float64(r.lean.offsetHz)
	}
	return 0
}

func (r *rxHost) deviationHz() float64 {
	switch {
	case r.iqd != nil:
		return r.iqd.deviation() / iqUnitsPerHz
	case r.lean != nil:
		return float64(r.lean.devHz)
	}
	return 0
}

func (r *rxHost) maxOutput(n int) int {
	return 4*(n/r.slot+2) + 16
}

const (
	frontLagSeconds = 0.005
	blockLagSeconds = float64(iqAFCBlock) / basebandRate
)

func (r *rxHost) lag() int {
	var s float64
	switch {
	case r.iqd != nil:
		s = r.iqd.d.lagSeconds() + blockLagSeconds
	case r.lean != nil:
		s = r.lean.lagSeconds()
	}
	return int(math.Ceil((s + frontLagSeconds) * float64(r.rate)))
}
