package rf

import (
	"errors"
	"testing"
)

type rxBoundSignal struct {
	bb48, bb24, bb96 []complex64
}

func newRxBoundSignal(t testing.TB) *rxBoundSignal {
	t.Helper()
	slots, _ := benchStream(t, 2)
	x := noisyBaseband(slots, 900, 25, 3)
	s := &rxBoundSignal{bb48: x, bb24: make([]complex64, len(x)/2), bb96: lowIQ(x, iq0LowHz)}
	for i := range s.bb24 {
		s.bb24[i] = (x[2*i] + x[2*i+1]) / 2
	}
	return s
}

func (s *rxBoundSignal) iq(bb []complex64, k int, signed bool) []byte {
	out := make([]byte, 0, 2*k*len(bb))
	for _, v := range bb {
		re, im := float64(real(v))*0.5*127.5, float64(imag(v))*0.5*127.5
		var a, b byte
		if signed {
			a, b = byte(int8(max(-127, min(127, re)))), byte(int8(max(-127, min(127, im))))
		} else {
			a, b = byte(max(0, min(255, re+127.5))), byte(max(0, min(255, im+127.5)))
		}
		for range k {
			out = append(out, a, b)
		}
	}
	return out
}

type rxBoundCase struct {
	tier                     Tier
	half, signed, baseband   bool
	mult, maxWrite, lookback int
	cuts                     []byte
	inner                    int
}

func (c rxBoundCase) config() (Config, bool) {
	cfg := Config{Tier: c.tier, Signed: c.signed, MaxWrite: c.maxWrite, LookbackSlots: c.lookback}
	hz := BasebandRate
	if c.half {
		if c.tier > Tier1 {
			return cfg, false
		}
		hz = BasebandRate / 2
		cfg.BasebandHz = hz
	}
	switch c.inner {
	case 1:
		if c.tier < Tier3 || c.half {
			return cfg, false
		}
		o := tierOptions(c.tier)
		o.Comb = true
		cfg.opt = &o
	case 2, 3:
		if c.half {
			return cfg, false
		}
		o := iq0Options(4, 2*symbolRate)
		cfg.Tier, cfg.opt = Tier0, &o
		if c.inner == 3 {
			o.IQLow = 1
			hz = iq0LowHz
			cfg.BasebandHz = hz
		}
	}
	cfg.Rate = float64(hz * c.mult)
	if cfg.Tier == Tier0 {
		cfg.LookbackSlots = 0
	}
	return cfg, true
}

func rxEngine(r *Receiver) string {
	switch {
	case r.cb != nil:
		return "comb"
	case r.iql != nil:
		return "iqlow"
	case r.iq0 != nil:
		return "iq0"
	case r.iqd != nil:
		return "iqdisc"
	case r.lean != nil:
		return "lean"
	default:
		return "engine"
	}
}

func runRxBounds(t *testing.T, s *rxBoundSignal, c rxBoundCase) (string, int) {
	t.Helper()
	cfg, ok := c.config()
	if !ok {
		return "", 0
	}
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	ocfg := cfg
	ocfg.OnBurst = func(*Burst) { seen++ }
	ro, err := New(ocfg)
	if err != nil {
		t.Fatal(err)
	}
	x := s.bb48
	switch {
	case c.half:
		x = s.bb24
	case c.inner == 3:
		x = s.bb96
	}
	if cfg.Tier >= Tier3 {
		x = x[:len(x)/3]
	}
	most := cfg.MaxWrite
	if most == 0 {
		most = 1 << 16
	}
	if c.baseband && most < 2 {
		return "", 0
	}
	var in []byte
	n := len(x)
	if !c.baseband {
		in = s.iq(x, c.mult, c.signed)
		n = len(in)
	}
	total := 0
	for off, i := 0, 0; off < n; i++ {
		k := 4096
		if len(c.cuts) > 0 {
			v := int(c.cuts[i%len(c.cuts)])
			k = 1 + v*v*v/16
		}
		k = min(k, n-off)
		if i%5 == 0 {
			var b []Burst
			var err error
			if long := most/2 + 1; c.baseband && off+long <= n {
				b, err = r.WriteBaseband(x[off : off+long])
			} else if long := most + 1; !c.baseband && off+long <= n {
				b, err = r.Write(in[off : off+long])
			} else {
				err = ErrWriteTooLong
			}
			if b != nil || !errors.Is(err, ErrWriteTooLong) {
				t.Fatalf("%+v: a call longer than MaxWrite gave %d bursts and %v", c, len(b), err)
			}
		}
		var got []Burst
		var bound int
		if c.baseband {
			k = min(k, max(most/2, 1))
			got, bound = must(r.WriteBaseband(x[off:off+k])), r.MaxBursts(k)
			must(ro.WriteBaseband(x[off : off+k]))
		} else {
			k = min(k, most)
			got, bound = must(r.Write(in[off:off+k])), r.MaxBursts((k+1)/2)
			must(ro.Write(in[off : off+k]))
		}
		if len(got) > bound {
			t.Fatalf("%+v: a call of %d gave %d bursts, MaxBursts %d", c, k, len(got), bound)
		}
		total += len(got)
		off += k
	}
	if got, bound := r.Flush(), r.MaxBursts(0); len(got) > bound {
		t.Fatalf("%+v: Flush gave %d bursts, MaxBursts(0) %d", c, len(got), bound)
	} else {
		total += len(got)
	}
	ro.Flush()
	if seen != total {
		t.Errorf("%+v: %s returned %d bursts, OnBurst took %d", c, rxEngine(r), total, seen)
	}
	return rxEngine(r), total
}

func TestReceiverMaxBursts(t *testing.T) {
	t.Parallel()
	s := newRxBoundSignal(t)
	engines := map[string]int{}
	tiers := []Tier{Tier0, Tier1, Tier2, Tier3, Tier4}
	for i, tier := range tiers {
		for j, c := range []rxBoundCase{
			{baseband: true, mult: 1},
			{baseband: true, mult: 5, maxWrite: 64, lookback: 1, cuts: []byte{0, 3, 40}},
			{baseband: true, mult: 5, cuts: []byte{255}},
			{mult: 1, lookback: 40, cuts: []byte{2, 70}},
			{mult: 2, maxWrite: 300, signed: true, cuts: []byte{1, 9, 30}},
			{half: true, baseband: true, mult: 1, lookback: 3},
			{half: true, mult: 3, maxWrite: 1 << 17},
		} {
			if tier >= Tier3 && j%2 == 1 {
				continue
			}
			c.tier = tier
			name, n := runRxBounds(t, s, c)
			if name == "" {
				continue
			}
			engines[name] += n
			if j == 0 && n == 0 {
				t.Errorf("tier %d gave no bursts", i)
			}
		}
	}
	for _, inner := range []int{1, 2, 3} {
		for _, c := range []rxBoundCase{
			{tier: Tier3, baseband: true, mult: 1, inner: inner},
			{tier: Tier3, baseband: true, mult: 1, lookback: 2, cuts: []byte{1, 9, 30}, inner: inner},
			{tier: Tier4, mult: 1, inner: inner, cuts: []byte{60}},
			{tier: Tier3, mult: 2, maxWrite: 999, inner: inner, cuts: []byte{3, 50}},
		} {
			if name, n := runRxBounds(t, s, c); name != "" {
				engines[name] += n
			}
		}
	}
	for _, e := range []string{"engine", "lean", "iqdisc", "comb", "iq0", "iqlow"} {
		if _, ok := engines[e]; !ok {
			t.Errorf("no case ran the %s receiver: %v", e, engines)
		}
	}
	t.Logf("bursts by receiver: %v", engines)
}

func FuzzReceiverMaxBursts(f *testing.F) {
	s := newRxBoundSignal(f)
	f.Add(uint8(0), uint8(0), uint8(1), uint16(0), uint8(0), []byte{3})
	f.Add(uint8(1), uint8(3), uint8(2), uint16(300), uint8(9), []byte{0, 255, 17})
	f.Add(uint8(2), uint8(4), uint8(5), uint16(1), uint8(40), []byte{1})
	f.Fuzz(func(t *testing.T, tier, flags, mult uint8, maxWrite uint16, lookback uint8, cuts []byte) {
		c := rxBoundCase{tier: Tier0 + Tier(tier%3), half: flags&1 != 0, baseband: flags&2 != 0, signed: flags&4 != 0,
			mult: 1 + int(mult%6), maxWrite: int(maxWrite), lookback: int(lookback % 48), cuts: cuts}
		if flags&8 != 0 {
			c.tier = Tier3 + Tier(tier%2)
		}
		c.inner = int(flags>>4) % 4
		_, _ = runRxBounds(t, s, c)
	})
}
