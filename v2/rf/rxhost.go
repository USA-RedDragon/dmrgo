package rf

import "math"

const rxBaseChunk = 1024

type rxHost struct {
	e      *engine
	lean   *leanRx
	iqd    *iqRx
	iq0    *iq0Rx
	cb     *combRx
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
	case (cfg.Tier == Tier1 || cfg.Tier == Tier2) && cfg.LookbackSlots > 0:
		opt.ExtLookback = cfg.LookbackSlots
	}
	if o != nil {
		opt = *o
	}
	if !opt.tabled() || opt.IQFront > 0 && opt.CohExt {
		return nil, errRxOpt
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
			BasebandHz: hz, OffsetHz: cfg.OffsetHz, LookbackSlots: cfg.LookbackSlots, sink: &r.sink, disc: &r.low.d}
		h.build(s, opt, low)
		r.full = h
	}
	look := opt.ExtLookback
	if opt.Coherent {
		look = cfg.LookbackSlots
		if look == 0 {
			look = 16
		}
	}
	if n := streamBursts(look); cfg.OnBursts != nil && n > 0 {
		r.sink.out = make([]Burst, 0, n)
	}
	if err := r.sink.attachShortLC(cfg.OnShortLC, cfg.Tier, true, errRxShortLC); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rxHost) build(s rxSetup, opt options, low bool) {
	switch {
	case low:
		r.iq0 = newIQ0Rx(s, opt)
	case !opt.Coherent:
		switch {
		case opt.IQDisc && s.BasebandHz == basebandRate && opt.IQFront > 0 && !opt.SoftExt && s.Rate == float64(s.BasebandHz):
			newIQ0Receiver(r, s, opt)
		case opt.IQDisc && s.BasebandHz == basebandRate:
			r.iqd = newIQRx(s, opt)
		default:
			r.lean = newLean(s, opt)
		}
	default:
		if s.LookbackSlots == 0 {
			s.LookbackSlots = 16
		}
		if opt.Comb {
			r.cb = newComb(s, opt)
			r.e = r.cb.t3
			return
		}
		r.e = newEngine(s, opt)
	}
}

func newIQ0Receiver(r *rxHost, cfg rxSetup, opt options) {
	if opt.IQFront == 4 {
		r.iq0 = newIQ0Rx(cfg, opt)
		return
	}
	r.iqd = newIQ0(cfg, opt)
}

func (r *rxHost) write(x []byte) {
	if !r.base {
		switch {
		case r.cb != nil:
			r.cb.write(x)
		case r.iq0 != nil:
			r.iq0.write(x)
		case r.iqd != nil:
			r.iqd.write(x)
		case r.lean != nil:
			r.lean.write(x)
		case r.e != nil:
			r.e.write(x)
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
	case r.cb != nil:
		r.cb.writeBaseband(x)
	case r.iq0 != nil:
		r.iq0.writeBaseband(x)
	case r.iqd != nil:
		r.iqd.writeBaseband(x)
	case r.lean != nil:
		r.lean.writeBaseband(x)
	case r.e != nil:
		r.e.writeBaseband(x)
	}
}

func (r *rxHost) flush() {
	switch {
	case r.cb != nil:
		r.cb.flush()
	case r.iq0 != nil:
		r.iq0.flush()
	case r.iqd != nil:
		r.iqd.flush()
	case r.lean != nil:
		r.lean.flush()
	case r.e != nil:
		r.e.flush()
	}
}

func (r *rxHost) active() bool {
	switch {
	case r.cb != nil:
		return r.cb.t3.locked || r.cb.t2.d.locked
	case r.iq0 != nil:
		return r.iq0.d.locked
	case r.iqd != nil:
		return r.iqd.d.locked
	case r.lean != nil:
		return r.lean.locked
	case r.e != nil:
		return r.e.locked
	}
	return false
}

func (r *rxHost) offsetHz() float64 {
	switch {
	case r.iq0 != nil:
		return float64(r.iq0.hz) + r.iq0.d.offset()/iqUnitsPerHz
	case r.iqd != nil:
		return float64(r.iqd.conv.hz) + r.iqd.d.offset()/iqUnitsPerHz
	case r.lean != nil:
		return float64(r.lean.offsetHz)
	case r.e != nil:
		return r.e.offsetHz
	}
	return 0
}

func (r *rxHost) deviationHz() float64 {
	switch {
	case r.iq0 != nil:
		return r.iq0.d.deviation() / iqUnitsPerHz
	case r.iqd != nil:
		return r.iqd.deviation() / iqUnitsPerHz
	case r.lean != nil:
		return float64(r.lean.devHz)
	case r.e != nil:
		return r.e.devHz
	}
	return 0
}

func (r *rxHost) maxOutput(n int) int {
	held := 16
	if r.e != nil {
		held += 2 * len(r.e.slots)
	}
	m := 4*(n/r.slot+2) + held
	if r.cb != nil {
		m = 2*m + cap(r.cb.pend[0]) + cap(r.cb.pend[1])
	}
	return m
}

const (
	frontLagSeconds = 0.005
	blockLagSeconds = float64(iqAFCBlock) / basebandRate
	t2LagSeconds    = 0.3
)

func (r *rxHost) lag() int {
	var s float64
	switch {
	case r.cb != nil:
		s = r.e.holdSeconds() + combT2Hold + t2LagSeconds
	case r.iq0 != nil:
		s = r.iq0.d.lagSeconds() + blockLagSeconds
	case r.iqd != nil:
		s = r.iqd.d.lagSeconds() + blockLagSeconds
	case r.lean != nil:
		s = r.lean.lagSeconds()
	case r.e != nil:
		s = r.e.holdSeconds() + t2LagSeconds
	}
	return int(math.Ceil((s + frontLagSeconds) * float64(r.rate)))
}

func (r *engine) holdSeconds() float64 {
	return float64(r.opt.passSlots()+r.cfg.LookbackSlots+maxGapSlots+16) * slotSeconds
}
