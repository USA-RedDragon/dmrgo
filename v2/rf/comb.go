package rf

import (
	"math"
	"math/bits"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const combMatchSeconds = 0.003

const combT2Hold = 1.0

const (
	combCleanGain  = 1.0 / 16
	combCleanShare = 0.5
)

type combSide struct {
	flips    [layer2.VoiceSlotsPerBurst]uint8
	hasFlips bool
	coreC2   uint8
	coreN    [layer2.VoiceSlotsPerBurst]uint8
}

type combBurst struct {
	b Burst
	s combSide
}

type combRx struct {
	t3           *engine
	t2           *iqRx
	ext          *softExt
	on           func(*Burst)
	out          []Burst
	pend         [2][]combBurst
	now, perByte float64
	mode         int
	hold         float64
	last         float64
	err3, err2   float32
	clean        float32
	cb           Burst
}

func combT2Options(o *options) options {
	t := tierOptions(Tier2)
	t.ExtLookback = 8
	t.CohNoCross, t.CohNoNext, t.CohBudget = true, true, 350
	t.IQTuneMaxHz, t.IQTuneSyncOnly, t.IQAFCAvg, t.IQSpecHunt = iqHuntMaxHz, true, 5, true
	t.CohSymNoise = o.CohSymNoise
	switch {
	case o.CombBudget < 0:
		t.CohBudget = 0
	case o.CombBudget > 0:
		t.CohBudget = o.CombBudget
	}
	return t
}

func newComb(cfg Config, opt options) *combRx {
	c := &combRx{last: math.Inf(-1), on: cfg.OnBurst, mode: opt.CombMode, perByte: 0.5 / cfg.Rate}
	n := opt.passSlots() + 4*cfg.LookbackSlots + 512
	for i := range c.pend {
		c.pend[i] = make([]combBurst, 0, 2*n)
	}
	c.hold = float64(cfg.LookbackSlots+16) * slotSeconds
	if c.on == nil {
		c.out = make([]Burst, 0, 4*n+4*cfg.MaxWrite/(cfg.BasebandHz*3/100*2))
	}
	c3 := cfg
	c3.OnBurst = func(b *Burst) { c.add(0, b, &c.t3.side) }
	c.t3 = newEngine(c3, opt)
	o2 := combT2Options(&opt)
	c2 := cfg
	c2.OnBurst = func(b *Burst) {
		var s combSide
		if c.ext != nil {
			s = c.ext.side
		}
		c.add(1, b, &s)
	}
	c2.LookbackSlots = 8
	c.t2 = newIQRx(c2, o2)
	c.ext, _ = c.t2.d.ext.(*softExt)
	return c
}

func (c *combRx) add(i int, b *Burst, s *combSide) {
	if len(c.pend[i]) == cap(c.pend[i]) {
		c.release(c.pend[i][0].b.Seconds + slotSeconds)
	}
	if n := len(c.pend[i]); n < cap(c.pend[i]) {
		c.pend[i] = c.pend[i][:n+1]
		c.pend[i][n] = combBurst{b: *b, s: *s}
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

func bySeconds(a, b combBurst) int {
	switch {
	case a.b.Seconds < b.b.Seconds:
		return -1
	case a.b.Seconds > b.b.Seconds:
		return 1
	}
	return 0
}

func (c *combRx) release(h float64) {
	a, b := c.pend[0], c.pend[1]
	slices.SortFunc(a, bySeconds)
	slices.SortFunc(b, bySeconds)
	i, j := 0, 0
	for {
		ha := i < len(a) && a[i].b.Seconds < h
		hb := j < len(b) && b[j].b.Seconds < h
		switch {
		case ha && a[i].b.Seconds < c.last+combMatchSeconds:
			i++
		case hb && b[j].b.Seconds < c.last+combMatchSeconds:
			j++
		case (ha || hb) && i < len(a) && j < len(b) && math.Abs(a[i].b.Seconds-b[j].b.Seconds) < combMatchSeconds:
			c.emit(c.pick(&a[i], &b[j]))
			c.last = max(a[i].b.Seconds, b[j].b.Seconds)
			i, j = i+1, j+1
		case ha && (!hb || a[i].b.Seconds <= b[j].b.Seconds):
			c.emit(&a[i].b)
			c.last = a[i].b.Seconds
			i++
		case hb:
			c.emit(&b[j].b)
			c.last = b[j].b.Seconds
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
	if n := len(c.out); n < cap(c.out) {
		c.out = c.out[:n+1]
		c.out[n] = *b
	}
}

func isVoice(b *Burst) bool {
	if b.Voice != enums.VoiceBurstUnknown {
		return true
	}
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	return known && kind == layer2.KindVoice && b.Distance >= 0
}

func slotFlips(x *combBurst) [layer2.VoiceSlotsPerBurst]int {
	var f [layer2.VoiceSlotsPerBurst]int
	switch {
	case x.s.hasFlips:
		for i := range f {
			f[i] = int(x.s.flips[i])
		}
	case x.b.HasFixed:
		for i, n := range voiceFlips(&x.b.Data, &x.b.Fixed) {
			f[i] = int(n)
		}
	}
	return f
}

func voiceFlips(d, x *layer2.RawBurst) [layer2.VoiceSlotsPerBurst]uint8 {
	var f [layer2.VoiceSlotsPerBurst]uint8
	for i := range f {
		for id := softAMBECode + 2*softSmallID(i); id <= softAMBECode+2*softSmallID(i)+1; id++ {
			c := smallCode(id)
			for _, p := range c.pos[:c.n] {
				f[i] += (d[p/8] ^ x[p/8]) >> (7 - p%8) & 1
			}
		}
	}
	return f
}

func (c *combRx) pick(x, y *combBurst) *Burst {
	a, b := &x.b, &y.b
	switch c.mode {
	case 1:
		return b
	case 2:
		return a
	}
	if !isVoice(a) || !isVoice(b) {
		return a
	}
	fa, fb := slotFlips(x), slotFlips(y)
	c.cb = *a
	if !c.cb.HasFixed {
		c.cb.Fixed, c.cb.FixedCACH, c.cb.HasFixed = a.Data, a.CACH, true
	}
	out := c.cb.Fixed.VoiceSlots()
	bd, _ := b.Decoded()
	bs := bd.VoiceSlots()
	var raw [layer2.VoiceSlotsPerBurst][9]byte
	if c.mode == 12 {
		raw = b.Data.VoiceSlots()
	}
	clean := fb[0]+fb[1]+fb[2] == 0
	cleaner := c.track(&fa, &fb)
	for i := range fa {
		core := y.s.coreC2>>i&1 != 0
		take := false
		switch c.mode {
		case 0:
			take = fb[i] < fa[i]
		case 3:
			take = fb[i] == 0
		case 4:
			take = clean
		case 5:
			take = cleaner && fb[i] <= fa[i]
		case 7:
			take = cleaner
		case 8:
			take = core
		case 9:
			take = core && int(y.s.coreN[i]) <= fa[i]
		case 10:
			take = core || cleaner && fb[i] <= fa[i]
		case 11:
			take = core || fb[i] == 0
		case 12:
			if out[i] == bs[i] {
				break
			}
			var v float32
			if raw[i] == bs[i] && frameDist(&out[i], &bs[i]) == 1 {
				v = 1
			}
			take = core || v == 1 && c.clean > combCleanShare
			c.clean += combCleanGain * (v - c.clean)
		}
		if take {
			out[i] = bs[i]
		}
	}
	c.cb.Fixed.SetVoiceSlots(out)
	return &c.cb
}

func (c *combRx) track(fa, fb *[layer2.VoiceSlotsPerBurst]int) bool {
	for i := range fa {
		if min(fa[i], fb[i]) <= extErrMax {
			c.err3 += extErrGain * (float32(fa[i]) - c.err3)
			c.err2 += extErrGain * (float32(fb[i]) - c.err2)
		}
	}
	return c.err2 < extCoreShare*c.err3
}

func frameDist(x, y *[9]byte) int {
	n := 0
	for i := range x {
		n += bits.OnesCount8(x[i] ^ y[i])
	}
	return n
}
