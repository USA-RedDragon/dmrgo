//go:build !tinygo

package fsktx

import (
	"fmt"
	"strings"
	"unsafe"
)

// Output kinds of the variants.
const (
	kindAudio = "audio"
	kindIQ    = "iq"
)

// sweepGain is the int16 units per symbol unit of the shapers of the
// sweep: 15000 units for the 1944 Hz of symbol +3, which leaves room for
// the overshoot of the pulse, up to 1.9 times.
const sweepGain = 5000

// spec is one modulator of the sweep.
type spec struct {
	Kind   string  `json:"kind"`
	Algo   string  `json:"algo"`
	Span   int     `json:"span"`
	Win    window  `json:"win"`
	Beta   float64 `json:"beta,omitempty"`
	Hz     int     `json:"hz"`
	G      uint    `json:"g,omitempty"`
	Half   bool    `json:"half,omitempty"`
	Shift  uint    `json:"shift,omitempty"`
	Up     int     `json:"up,omitempty"`
	UpK    int     `json:"upk,omitempty"`
	UpTaps int     `json:"uptaps,omitempty"`
	Order  int     `json:"order,omitempty"`
	Bits12 bool    `json:"bits12,omitempty"`
	Comp   bool    `json:"comp,omitempty"`
	// I/Q settings.
	Rate    int  `json:"rate,omitempty"`
	IQUp    int  `json:"iqup,omitempty"`
	NCO     int  `json:"nco,omitempty"`
	Bits    uint `json:"bits,omitempty"`
	Iters   int  `json:"iters,omitempty"`
	Out8    bool `json:"out8,omitempty"`
	RefIQ   bool `json:"refiq,omitempty"`
	OutRate int  `json:"outrate"`
	// Tier is the library tier of Algo "tier".
	Tier Tier `json:"tier,omitempty"`
}

// tierAudio runs an Audio of the library as a shaper.
type tierAudio struct{ a *Audio }

func (t tierAudio) appendSymbol(dst []int16, s int8) []int16 { return t.a.AppendSymbol(dst, s) }

// tierIQ runs an IQ of the library as a shaper of interleaved I/Q, in
// 8-bit values when s8 is set.
type tierIQ struct {
	q  *IQ
	s8 bool
	b  []byte
}

func (t *tierIQ) appendSymbol(dst []int16, s int8) []int16 {
	if !t.s8 {
		return t.q.AppendS16(dst, s)
	}
	t.b = t.q.AppendS8(t.b[:0], s)
	for _, v := range t.b {
		dst = append(dst, int16(int8(v)))
	}
	return dst
}

var winNames = map[window]string{winRect: "rect", winHamming: "hamming", winBlackman: "blackman", winKaiser: "kaiser"}

var upNames = map[int]string{stageHold: "hold", stageLinear: "linear", stageFIR: "fir", stageCIC: "cic"}

var ncoNames = map[int]string{ncoTable: "table", ncoTableInterp: "tablei", ncoCORDIC: "cordic", ncoF64: "f64", ncoRotF32: "rot32"}

// rfKey returns the name of s less what does not change its samples
// beyond rounding: the arithmetic of the shaper and the groups and halving
// of a lookup table.
func (s spec) rfKey() string {
	c := s
	if c.Algo != "lut8" && c.Algo != "tier" {
		c.Algo = "any"
	}
	c.G, c.Half = 0, false
	return c.name()
}

// name returns a short name of the modulator.
func (s spec) name() string {
	var b strings.Builder
	if s.Algo == "tier" {
		if s.Kind == kindAudio {
			fmt.Fprintf(&b, "audio-tier%d-%gk", s.Tier-Tier0, float64(s.Hz)/1000)
		} else {
			fmt.Fprintf(&b, "iq-tier%d-%gM", s.Tier-Tier0, float64(s.Rate)/1e6)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "%s-%s-s%d-%s", s.Kind, s.Algo, s.Span, winNames[s.Win])
	if s.Win == winKaiser {
		fmt.Fprintf(&b, "%g", s.Beta)
	}
	fmt.Fprintf(&b, "-%dk", s.Hz/1000)
	if s.Hz%1000 != 0 {
		fmt.Fprintf(&b, "%d", s.Hz%1000/100)
	}
	if s.Algo == "lut8" {
		b.WriteString("-g4z")
	}
	if s.Algo == "lut" {
		fmt.Fprintf(&b, "-g%d", s.G)
		if s.Half {
			b.WriteString("h")
		}
		if s.Shift > 0 {
			fmt.Fprintf(&b, "-q%d", s.Shift)
		}
	}
	if s.UpK > 1 {
		fmt.Fprintf(&b, "-%s%d", upNames[s.Up], s.UpK)
		if s.Up == stageFIR {
			fmt.Fprintf(&b, "t%d", s.UpTaps)
		}
		if s.Up == stageCIC {
			fmt.Fprintf(&b, "o%d", s.Order)
		}
	}
	if s.Bits12 {
		b.WriteString("-dac12")
	}
	if s.Comp {
		b.WriteString("-comp")
	}
	if s.Kind == kindIQ {
		if s.RefIQ {
			b.WriteString("-direct")
		} else {
			fmt.Fprintf(&b, "-%s", map[int]string{upHold: "hold", upLinear: "lin"}[s.IQUp])
		}
		fmt.Fprintf(&b, "-%s", ncoNames[s.NCO])
		switch s.NCO {
		case ncoTable, ncoTableInterp:
			fmt.Fprintf(&b, "%d", s.Bits)
		case ncoCORDIC:
			fmt.Fprintf(&b, "%d", s.Iters)
		}
		if s.Out8 {
			b.WriteString("-i8")
		} else {
			b.WriteString("-i16")
		}
		fmt.Fprintf(&b, "-%gM", float64(s.Rate)/1e6)
	}
	return b.String()
}

// pulse returns the pulse of s.
func (s spec) pulse() pulse {
	p := pulse{span: s.Span, win: s.Win, beta: s.Beta}
	if s.Comp {
		p.comp = 1.0 / 24
	}
	return p
}

// shaper returns a new shaper of s, at the rate Hz, and its output rate.
func (s spec) shaper() (shaper, int) {
	var k shaper
	switch s.Algo {
	case "tier":
		a, err := NewAudio(AudioConfig{Hz: s.Hz, Tier: s.Tier})
		if err != nil {
			panic(err)
		}
		return tierAudio{a}, s.Hz
	case "poly64":
		k = newPolyF64(s.pulse(), s.Hz, sweepGain)
	case "poly32":
		k = newPolyF32(s.pulse(), s.Hz, sweepGain)
	case "polyq15":
		k = newPolyQ15(s.pulse(), s.Hz, sweepGain, 14)
	case "lut8":
		k = newLUT8(s.pulse(), s.Hz/SymbolRate, sweepGain)
	case "lut":
		lk := newLUTQ15(s.pulse(), s.Hz, s.G, s.Half)
		if s.Shift > 0 {
			lk = quantizeLUT(lk, s.Shift)
		}
		k = lk
	default:
		panic(s.Algo)
	}
	hz := s.Hz
	if s.UpK > 1 {
		k = newUpStage(k, s.UpK, s.Up, s.UpTaps, s.Order)
		hz *= s.UpK
	}
	if s.Bits12 {
		k = dac12{k}
	}
	return k, hz
}

// dac12 rounds the samples of a shaper to 12 bits.
type dac12 struct{ in shaper }

func (d dac12) appendSymbol(dst []int16, s int8) []int16 {
	n := len(dst)
	dst = d.in.appendSymbol(dst, s)
	for i := n; i < len(dst); i++ {
		dst[i] = int16(min((int32(dst[i])+8)>>4, 2047) << 4)
	}
	return dst
}

// quantizeLUT rounds the table entries of k to multiples of 2^shift, as
// a table of narrower entries holds them.
func quantizeLUT(k *lutQ15, shift uint) *lutQ15 {
	for i, v := range k.tab {
		k.tab[i] = int16(((int32(v) + 1<<shift>>1) >> shift) << shift)
	}
	return k
}

// mcuIQ is the I/Q modulator of the microcontroller: a lut8 and an nco.
type mcuIQ struct {
	sh  *lut8
	o   *nco
	buf [16]int16
}

func (m *mcuIQ) appendSymbol(dst []int16, s int8) []int16 {
	n := m.sh.symbol(m.buf[:], s)
	k := len(dst)
	for range 2 * m.o.up * n {
		dst = append(dst, 0)
	}
	m.o.run(dst[k:], m.buf[:n])
	return dst
}

// iq returns a new I/Q modulator of s.
func (s spec) iq() shaper {
	amp := 0.9 * 32767
	if s.Out8 {
		amp = 0.9 * 127
	}
	if s.Algo == "tier" {
		q, err := NewIQ(IQConfig{Rate: s.Rate, Tier: s.Tier})
		if err != nil {
			panic(err)
		}
		q.Key(true)
		return &tierIQ{q: q, s8: s.Rate >= 1e6}
	}
	if s.Algo == "lut8" {
		return &mcuIQ{sh: newLUT8(s.pulse(), s.Hz/SymbolRate, sweepGain), o: newNCO(DeviationHz/sweepGain, s.Rate, s.Rate/s.Hz, amp, 0, max(s.Bits, ncoBits), s.NCO == ncoTableInterp)}
	}
	if s.RefIQ {
		sh := newPolyF64(s.pulse(), s.Rate, sweepGain)
		return newIQPipe(sh, float64(s.Rate), sweepGain, s.Rate, upHold, s.NCO, s.Bits, s.Iters, 0, amp)
	}
	sh, hz := s.shaper()
	return newIQPipe(sh, float64(hz), sweepGain, s.Rate, s.IQUp, s.NCO, s.Bits, s.Iters, 0, amp)
}

// tableBytes returns the read-only bytes of the tables of s, which a
// microcontroller keeps in flash, and stateBytes the bytes of its mutable
// state.
func (s spec) memory() (tables, state int) {
	if s.Algo == "tier" {
		return s.tierMemory()
	}
	l, _ := ratio(s.Hz)
	n := 2 * s.Span
	switch s.Algo {
	case "poly64":
		tables, state = 8*l*n, 8*2*n+48
	case "poly32":
		tables, state = 4*l*n, 4*2*n+40
	case "polyq15":
		tables, state = 2*l*n, 2*n+32
	case "lut8":
		tables, state = 2*l*2*(256+16), 8+16
	case "lut":
		groups := (n + int(s.G) - 1) / int(s.G)
		size := 1 << (2 * s.G)
		if s.Half {
			size /= 2
		}
		entry := 2
		if s.Shift >= 8 {
			entry = 1
		}
		tables = entry*l*groups*size + 2*l*n
		state = 16 + 2*n + 24
	}
	if s.UpK > 1 {
		switch s.Up {
		case stageFIR:
			tables += 2 * s.UpK * s.UpTaps
			state += 2 * 2 * s.UpTaps
		case stageCIC:
			state += 8 * 2 * s.Order
		default:
			state += 4
		}
	}
	if s.Kind == kindIQ {
		if s.RefIQ {
			l2, _ := ratio(s.Rate)
			tables = 8 * l2 * n
		}
		switch s.NCO {
		case ncoTable, ncoTableInterp:
			tables += 2 * (1<<s.Bits + 1)
		case ncoCORDIC:
			tables += 4 * s.Iters
		}
		state += 32
	}
	return tables, state
}

// tierMemory returns the tables and state bytes of a tier of the library.
func (s spec) tierMemory() (tables, state int) {
	switch {
	case s.Kind == kindAudio && s.Tier == Tier0:
		t := Table24k()
		if s.Hz == 48000 {
			t = Table48k()
		}
		return 2 * s.Hz / SymbolRate * 2 * t.Span(), int(unsafe.Sizeof(MCUAudio{}))
	case s.Kind == kindIQ && s.Tier == Tier0:
		return int(unsafe.Sizeof(tapsIQ) + unsafe.Sizeof(cosTable)), int(unsafe.Sizeof(MCUIQ{}))
	}
	span, w := 8, 4
	if s.Tier == Tier2 {
		span, w = 16, 8
	}
	hz := s.Hz
	if s.Kind == kindIQ {
		hz = iqShaperHz
	}
	l, _ := ratio(hz)
	tables = l * 2 * span * w
	state = 2*2*span*w + int(unsafe.Sizeof(poly{}))
	if s.Kind == kindAudio {
		return tables, state + int(unsafe.Sizeof(Audio{})) + 8*(l+1)
	}
	if s.Tier == Tier1 {
		tables += 4 * (1<<iqTableBits + 1)
	}
	return tables, state + int(unsafe.Sizeof(IQ{})) + 8*(l+1)
}

// sweepSpecs returns the modulators of the sweep.
func sweepSpecs() []spec {
	var out []spec
	add := func(s spec) {
		_, hz := s.shaper()
		s.OutRate = hz
		if s.Kind == kindIQ {
			s.OutRate = s.Rate
		}
		for _, o := range out {
			if o.name() == s.name() {
				return
			}
		}
		out = append(out, s)
	}
	audioSpecs(add)
	iqSpecs(add)
	return out
}

// audioSpecs adds the audio modulators of the sweep.
func audioSpecs(add func(spec)) {
	audio := func(algo string, span int, w window, beta float64, hz int) spec {
		return spec{Kind: kindAudio, Algo: algo, Span: span, Win: w, Beta: beta, Hz: hz}
	}
	// Pulse length and taper, float64 at 48 kHz.
	for _, span := range []int{2, 3, 4, 6, 8, 12, 16} {
		add(audio("poly64", span, winRect, 0, 48000))
	}
	for _, span := range []int{2, 3, 4, 6} {
		add(audio("poly64", span, winHamming, 0, 48000))
		add(audio("poly64", span, winBlackman, 0, 48000))
		add(audio("poly64", span, winKaiser, 4, 48000))
		add(audio("poly64", span, winKaiser, 8, 48000))
	}
	// Output rate.
	for _, hz := range []int{8000, 9600, 12000, 16000, 19200, 24000} {
		add(audio("poly32", 8, winRect, 0, hz))
	}
	// Arithmetic.
	for _, hz := range []int{24000, 48000} {
		add(audio("poly32", 8, winRect, 0, hz))
		add(audio("polyq15", 8, winRect, 0, hz))
		add(audio("polyq15", 4, winKaiser, 4, hz))
		add(audio("poly32", 4, winKaiser, 4, hz))
	}
	// Lookup tables.
	for _, hz := range []int{9600, 24000, 48000} {
		for _, span := range []int{3, 4, 6, 8} {
			for _, g := range []uint{1, 2, 4} {
				if g == 4 && span > 4 && hz > 24000 {
					continue
				}
				add(spec{Kind: kindAudio, Algo: "lut", Span: span, Win: winKaiser, Beta: 4, Hz: hz, G: g})
			}
		}
		add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: hz, G: 4, Half: true})
		add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: hz, G: 2, Half: true})
		add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winRect, Hz: hz, G: 4})
	}
	for _, sh := range []uint{4, 6, 8} {
		add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: 24000, G: 4, Shift: sh})
	}
	add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: 24000, G: 4, Bits12: true})
	add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: 48000, G: 4, Bits12: true})
	// Making up for the droop of the hold of the DAC.
	for _, hz := range []int{9600, 12000, 24000, 48000} {
		add(spec{Kind: kindAudio, Algo: "polyq15", Span: 4, Win: winKaiser, Beta: 4, Hz: hz, Comp: true})
		add(spec{Kind: kindAudio, Algo: "polyq15", Span: 4, Win: winKaiser, Beta: 4, Hz: hz})
		add(spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: hz, G: 4, Comp: true})
	}
	// Two stages: a shaper at a low rate and an interpolation.
	for _, st := range []struct{ hz, k int }{{9600, 5}, {24000, 2}, {12000, 2}, {12000, 4}} {
		for _, up := range []int{stageHold, stageLinear, stageFIR, stageCIC} {
			s := spec{Kind: kindAudio, Algo: "lut", Span: 4, Win: winKaiser, Beta: 4, Hz: st.hz, G: 4, Up: up, UpK: st.k, Order: 3}
			if st.hz == 12000 {
				s.Algo, s.G = "polyq15", 0
			}
			if up == stageFIR {
				s.UpTaps = 8
				add(s)
				s.UpTaps = 4
			}
			add(s)
		}
	}
}

// iqSpecs adds the I/Q modulators of the sweep, and the tiers of the
// library of both kinds.
func iqSpecs(add func(spec)) {
	audio := func(algo string, span int, w window, beta float64, hz int) spec {
		return spec{Kind: kindAudio, Algo: algo, Span: span, Win: w, Beta: beta, Hz: hz}
	}
	// I/Q: the reference path, then shapers, interpolations and NCOs.
	iq := func(algo string, span int, hz, rate, up, nco int, bits uint, iters int, out8 bool) spec {
		s := spec{Kind: kindIQ, Algo: algo, Span: span, Win: winKaiser, Beta: 4, Hz: hz, Rate: rate, IQUp: up, NCO: nco, Bits: bits, Iters: iters, Out8: out8}
		if algo == "lut" {
			s.G = 4
		}
		if algo == "poly32" && span == 8 {
			s.Win, s.Beta = winRect, 0
		}
		return s
	}
	const hrf = 2400000
	add(spec{Kind: kindIQ, Algo: "poly64", Span: 16, Win: winRect, Hz: hrf, Rate: hrf, NCO: ncoF64, Out8: true, RefIQ: true})
	add(spec{Kind: kindIQ, Algo: "poly64", Span: 8, Win: winRect, Hz: hrf, Rate: hrf, NCO: ncoTableInterp, Bits: 10, Out8: true, RefIQ: true})
	for _, nco := range []struct {
		k     int
		bits  uint
		iters int
	}{{ncoF64, 0, 0}, {ncoRotF32, 0, 0}, {ncoTable, 8, 0}, {ncoTable, 10, 0}, {ncoTable, 12, 0}, {ncoTableInterp, 8, 0}, {ncoTableInterp, 10, 0}, {ncoCORDIC, 0, 8}, {ncoCORDIC, 0, 12}, {ncoCORDIC, 0, 16}} {
		add(iq("poly32", 8, 48000, hrf, upLinear, nco.k, nco.bits, nco.iters, true))
	}
	for _, up := range []int{upHold, upLinear} {
		add(iq("poly32", 8, 24000, hrf, up, ncoTable, 10, 0, true))
		add(iq("poly32", 8, 48000, hrf, up, ncoTable, 10, 0, true))
		add(iq("lut", 4, 48000, hrf, up, ncoTable, 10, 0, true))
		add(iq("lut", 4, 24000, hrf, up, ncoTable, 10, 0, true))
	}
	add(iq("poly32", 8, 48000, 2000000, upLinear, ncoTable, 10, 0, true))
	add(spec{Kind: kindIQ, Algo: "poly64", Span: 16, Win: winRect, Hz: 48000, Rate: hrf, IQUp: upLinear, NCO: ncoF64, Out8: true})
	add(spec{Kind: kindIQ, Algo: "poly64", Span: 16, Win: winRect, Hz: 48000, Rate: hrf, IQUp: upLinear, NCO: ncoTableInterp, Bits: 10, Out8: true})
	add(spec{Kind: kindIQ, Algo: "poly64", Span: 16, Win: winRect, Hz: 96000, Rate: hrf, IQUp: upLinear, NCO: ncoF64, Out8: true})
	add(spec{Kind: kindIQ, Algo: "poly32", Span: 8, Win: winRect, Hz: 48000, Rate: 2000000, IQUp: upLinear, NCO: ncoTableInterp, Bits: 10, Out8: true})
	add(iq("lut", 4, 48000, 2000000, upLinear, ncoTable, 10, 0, true))
	for _, rate := range []int{48000, 96000} {
		for _, nco := range []struct {
			k     int
			bits  uint
			iters int
		}{{ncoTable, 6, 0}, {ncoTable, 8, 0}, {ncoTable, 10, 0}, {ncoTableInterp, 8, 0}, {ncoCORDIC, 0, 12}} {
			add(iq("lut", 4, 48000, rate, upHold, nco.k, nco.bits, nco.iters, false))
		}
		add(iq("lut", 4, 48000, rate, upHold, ncoTable, 8, 0, true))
		add(iq("lut", 3, 48000, rate, upHold, ncoTable, 8, 0, false))
		add(iq("lut", 6, 48000, rate, upHold, ncoTable, 8, 0, false))
		add(iq("poly32", 8, 48000, rate, upHold, ncoF64, 0, 0, false))
	}
	for _, span := range []int{3, 4, 6, 8} {
		for _, beta := range []float64{1, 2, 3} {
			add(audio("poly64", span, winKaiser, beta, 48000))
		}
	}
	for _, hz := range []int{9600, 19200, 24000, 48000} {
		for _, beta := range []float64{1, 2, 3} {
			add(spec{Kind: kindAudio, Algo: "lut8", Span: 4, Win: winKaiser, Beta: beta, Hz: hz})
		}
		add(spec{Kind: kindAudio, Algo: "lut8", Span: 4, Win: winKaiser, Beta: 4, Hz: hz})
		add(spec{Kind: kindAudio, Algo: "lut8", Span: 4, Win: winRect, Hz: hz})
		add(spec{Kind: kindAudio, Algo: "lut8", Span: 4, Win: winHamming, Hz: hz})
	}
	for _, c := range []struct{ hz, rate int }{{48000, 48000}, {24000, 48000}, {48000, 96000}} {
		for _, nco := range []int{ncoTable, ncoTableInterp} {
			for _, w := range []window{winKaiser, winRect} {
				s := spec{Kind: kindIQ, Algo: "lut8", Span: 4, Win: w, Beta: 4, Hz: c.hz, Rate: c.rate, IQUp: upLinear, NCO: nco, Bits: ncoBits}
				if w == winRect {
					s.Beta = 0
				}
				add(s)
				if w == winKaiser {
					s.Beta = 2
					add(s)
				}
			}
		}
	}
	add(iq("lut", 4, 24000, 48000, upLinear, ncoTable, 8, 0, false))
	for _, c := range []struct{ hz, rate int }{{48000, 48000}, {24000, 48000}} {
		for _, bits := range []uint{10, 11, 12} {
			add(spec{Kind: kindIQ, Algo: "lut8", Span: 4, Win: winKaiser, Beta: 2, Hz: c.hz, Rate: c.rate, IQUp: upLinear, NCO: ncoTable, Bits: bits})
		}
	}
	for _, t := range []struct {
		tier Tier
		hz   int
	}{{Tier0, 24000}, {Tier0, 48000}, {Tier1, 24000}, {Tier1, 48000}, {Tier2, 48000}} {
		add(spec{Kind: kindAudio, Algo: "tier", Tier: t.tier, Hz: t.hz})
	}
	for _, t := range []struct {
		tier Tier
		rate int
	}{{Tier0, 48000}, {Tier0, 96000}, {Tier1, 96000}, {Tier1, 2000000}, {Tier1, 2400000}, {Tier2, 2400000}} {
		add(spec{Kind: kindIQ, Algo: "tier", Tier: t.tier, Rate: t.rate, Hz: 48000})
	}
	add(iq("lut", 4, 24000, 48000, upHold, ncoTable, 8, 0, false))
}
