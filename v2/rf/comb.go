package rf

import (
	"math"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// combMatchSeconds is the most that the times of two bursts of the same
// slot differ between the two receivers of combRx.
const combMatchSeconds = 0.003

// combT2Hold is the time, in seconds, after which the Tier2 receiver of
// combRx emits no more bursts.
const combT2Hold = 1.0

// combRx runs the coherent receiver of a tier and the Tier2 receiver on the
// same input, and emits each slot once: the burst of the coherent receiver
// with the AMBE frames that options.CombMode selects taken from Tier2, or
// the burst of the receiver that alone has one (options.Comb).
type combRx struct {
	t3   *engine
	t2   *iqRx
	on   func(*Burst)
	out  []Burst
	pend [2][]Burst
	// now is the input time in seconds; perByte that of an input byte.
	now, perByte float64
	mode         int
	hold         float64
	// last is the time of the last burst that left.
	last float64
	// err3 and err2 are the mean numbers of bits of C0 and C1 of a voice
	// frame that the soft decode of each receiver changed.
	err3, err2 float32
	cb         Burst
}

// combT2Options returns the settings of the Tier2 receiver of combRx.
func combT2Options(o *options) options {
	t := tierOptions(Tier2)
	t.ExtLookback = 8
	t.CohNoCross, t.CohNoNext, t.CohBudget = true, true, 350
	t.IQTuneMaxHz, t.IQTuneSyncOnly, t.IQAFCAvg = 2500, true, 5
	switch {
	case o.CombBudget < 0:
		t.CohBudget = 0
	case o.CombBudget > 0:
		t.CohBudget = o.CombBudget
	}
	return t
}

// newComb returns the combRx of cfg with the coherent settings opt.
func newComb(cfg Config, opt options) *combRx {
	c := &combRx{last: math.Inf(-1), on: cfg.OnBurst, mode: opt.CombMode, perByte: 0.5 / cfg.Rate}
	n := opt.passSlots() + 4*cfg.LookbackSlots + 512
	for i := range c.pend {
		c.pend[i] = make([]Burst, 0, 2*n)
	}
	c.hold = float64(cfg.LookbackSlots+16) * slotSeconds
	if c.on == nil {
		c.out = make([]Burst, 0, 4*n)
	}
	o3 := opt
	c3 := cfg
	c3.OnBurst = func(b *Burst) { c.add(0, b) }
	c.t3 = newEngine(c3, o3)
	o2 := combT2Options(&opt)
	c2 := cfg
	c2.OnBurst = func(b *Burst) { c.add(1, b) }
	c2.LookbackSlots = 8
	c.t2 = newIQRx(c2, o2)
	return c
}

// add takes burst b of receiver s.
func (c *combRx) add(s int, b *Burst) {
	if len(c.pend[s]) == cap(c.pend[s]) {
		c.release(c.pend[s][0].Seconds + slotSeconds)
	}
	if len(c.pend[s]) < cap(c.pend[s]) {
		c.pend[s] = append(c.pend[s], *b)
	}
}

func (c *combRx) write(iq []byte) []Burst {
	c.out = c.out[:0]
	c.t3.write(iq)
	c.t2.write(iq)
	c.now += float64(len(iq)) * c.perByte
	c.release(c.horizon())
	return c.out
}

func (c *combRx) writeBaseband(x []complex64) []Burst {
	c.out = c.out[:0]
	c.t3.writeBaseband(x)
	c.t2.writeBaseband(x)
	c.now += float64(len(x)) / float64(c.t3.cfg.BasebandHz)
	c.release(c.horizon())
	return c.out
}

func (c *combRx) flush() []Burst {
	c.out = c.out[:0]
	c.t3.flush()
	c.t2.flush()
	c.release(math.Inf(1))
	return c.out
}

// horizon returns the time before which neither receiver emits a burst
// any more.
func (c *combRx) horizon() float64 {
	h := c.now - combT2Hold
	e := c.t3
	h3 := c.now - c.hold
	if e.locked || e.pend {
		j := min(e.first, e.passStart)
		h3 = min(h3, (e.slotTime(j)-float64(e.chz.delayCh))/float64(e.cfg.BasebandHz)-c.hold)
	}
	return min(h, h3)
}

func bySeconds(a, b Burst) int {
	switch {
	case a.Seconds < b.Seconds:
		return -1
	case a.Seconds > b.Seconds:
		return 1
	}
	return 0
}

// release emits, in time order, the bursts before h. Two bursts of the
// same slot leave together once either is before h, and a burst of a slot
// that has left already is dropped, so that each slot leaves once.
func (c *combRx) release(h float64) {
	a, b := c.pend[0], c.pend[1]
	slices.SortFunc(a, bySeconds)
	slices.SortFunc(b, bySeconds)
	i, j := 0, 0
	for {
		ha := i < len(a) && a[i].Seconds < h
		hb := j < len(b) && b[j].Seconds < h
		switch {
		case ha && a[i].Seconds < c.last+combMatchSeconds:
			i++
		case hb && b[j].Seconds < c.last+combMatchSeconds:
			j++
		case (ha || hb) && i < len(a) && j < len(b) && math.Abs(a[i].Seconds-b[j].Seconds) < combMatchSeconds:
			c.emit(c.pick(&a[i], &b[j]))
			c.last = max(a[i].Seconds, b[j].Seconds)
			i, j = i+1, j+1
		case ha && (!hb || a[i].Seconds <= b[j].Seconds):
			c.emit(&a[i])
			c.last = a[i].Seconds
			i++
		case hb:
			c.emit(&b[j])
			c.last = b[j].Seconds
			j++
		default:
			c.pend[0] = a[:copy(a, a[i:])]
			c.pend[1] = b[:copy(b, b[j:])]
			return
		}
	}
}

func (c *combRx) emit(b *Burst) {
	if c.on != nil {
		c.on(b)
		return
	}
	if len(c.out) < cap(c.out) {
		c.out = append(c.out, *b)
	}
}

// isVoice reports whether b carries AMBE frames.
func isVoice(b *Burst) bool {
	if b.Voice != enums.VoiceBurstUnknown {
		return true
	}
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	return known && kind == layer2.KindVoice && b.Distance >= 0
}

// slotFlips returns, for each voice slot, the bits that the soft decode of
// b changed.
func slotFlips(b *Burst) [layer2.VoiceSlotsPerBurst]int {
	var f [layer2.VoiceSlotsPerBurst]int
	switch {
	case b.hasFlips:
		for i := range f {
			f[i] = int(b.flips[i])
		}
	case b.HasFixed:
		for i, n := range voiceFlips(&b.Data, &b.Fixed) {
			f[i] = int(n)
		}
	}
	return f
}

// voiceFlips returns, for each voice slot, the bits in which d and x
// differ.
func voiceFlips(d, x *layer2.RawBurst) [layer2.VoiceSlotsPerBurst]uint8 {
	var f [layer2.VoiceSlotsPerBurst]uint8
	for i := range f {
		for id := softAMBECode + 2*softSmallID(i); id <= softAMBECode+2*softSmallID(i)+1; id++ {
			c := &softSmallCodes[id]
			for _, p := range c.pos[:c.n] {
				f[i] += (d[p/8] ^ x[p/8]) >> (7 - p%8) & 1
			}
		}
	}
	return f
}

// pick returns the burst of slot that a, of the coherent receiver, and b,
// of Tier2, both hold.
func (c *combRx) pick(a, b *Burst) *Burst {
	switch c.mode {
	case 1:
		return b
	case 2:
		return a
	}
	if !isVoice(a) || !isVoice(b) {
		return a
	}
	fa, fb := slotFlips(a), slotFlips(b)
	c.cb = *a
	if !c.cb.HasFixed {
		c.cb.Fixed, c.cb.FixedCACH, c.cb.HasFixed = a.Data, a.CACH, true
	}
	out := c.cb.Fixed.VoiceSlots()
	bd, _ := b.Decoded()
	bs := bd.VoiceSlots()
	clean := fb[0]+fb[1]+fb[2] == 0
	for i := range fa {
		if min(fa[i], fb[i]) <= extErrMax {
			c.err3 += extErrGain * (float32(fa[i]) - c.err3)
			c.err2 += extErrGain * (float32(fb[i]) - c.err2)
		}
	}
	cleaner := c.err2 < extCoreShare*c.err3
	for i := range fa {
		switch c.mode {
		case 5:
			if cleaner && fb[i] <= fa[i] {
				out[i] = bs[i]
			}
		case 6:
			if cleaner && fb[i] <= fa[i] {
				for _, p := range &softAMBEC2Pos[i] {
					q, m := p/8, byte(0x80)>>(p%8)
					c.cb.Fixed[q] = c.cb.Fixed[q]&^m | bd[q]&m
				}
				out[i] = c.cb.Fixed.VoiceSlots()[i]
			}
		case 7:
			if cleaner {
				out[i] = bs[i]
			}
		case 8:
			if b.coreC2>>i&1 != 0 {
				out[i] = bs[i]
			}
		case 10:
			if b.coreC2>>i&1 != 0 || cleaner && fb[i] <= fa[i] {
				out[i] = bs[i]
			}
		case 11:
			if b.coreC2>>i&1 != 0 || fb[i] == 0 {
				out[i] = bs[i]
			}
		case 9:
			if b.coreC2>>i&1 != 0 && int(b.coreN[i]) <= fa[i] {
				out[i] = bs[i]
			}
		case 0:
			if fb[i] < fa[i] {
				out[i] = bs[i]
			}
		case 3:
			if fb[i] == 0 {
				out[i] = bs[i]
			}
		case 4:
			if clean {
				out[i] = bs[i]
			}
		}
	}
	c.cb.Fixed.SetVoiceSlots(out)
	return &c.cb
}
