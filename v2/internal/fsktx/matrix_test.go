//go:build !tinygo

package fsktx

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// txFull reports whether DMRGO_TXMATRIX is "full", which runs the full
// robustness matrix: every rate through every receiver and hours of
// symbols. The default is a subset that fits a CI run.
func txFull() bool { return os.Getenv("DMRGO_TXMATRIX") == "full" }

// txFormat is the output format of a modulator of the matrix.
type txFormat int

// The output formats.
const (
	fmtAudio txFormat = iota
	fmtS16
	fmtS8
)

// txUnit is one modulator setup of the robustness matrix: the modulator,
// its output rate and format, and its level and offset or its carrier
// offset and power ramp.
type txUnit struct {
	mcu           bool
	tier          Tier
	format        txFormat
	rate          int
	level, offset int
	offHz         int
	ramp          int
}

// kind names the modulator of u, without its setup.
func (u txUnit) kind() string {
	switch {
	case u.format == fmtAudio && u.mcu:
		return "MCUAudio"
	case u.format == fmtAudio:
		return fmt.Sprintf("Audio-T%d", u.tier-Tier0)
	case u.mcu:
		return "MCUIQ"
	case u.format == fmtS8:
		return fmt.Sprintf("IQ-T%d-S8", u.tier-Tier0)
	default:
		return fmt.Sprintf("IQ-T%d-S16", u.tier-Tier0)
	}
}

func (u txUnit) String() string {
	if u.format == fmtAudio {
		return fmt.Sprintf("%s/%d/L%d/O%d", u.kind(), u.rate, u.level, u.offset)
	}
	return fmt.Sprintf("%s/%d/F%d/R%d", u.kind(), u.rate, u.offHz, u.ramp)
}

// audio reports whether u writes audio.
func (u txUnit) audio() bool { return u.format == fmtAudio }

// txMod is a modulator of the matrix behind one interface: key keys an
// I/Q modulator up or down, and emit appends the output of one symbol,
// audio samples or interleaved I/Q, with 8-bit I/Q widened to int16.
type txMod interface {
	key(on bool)
	emit(dst []int16, s int8) []int16
}

type audioMod struct{ a *Audio }

func (audioMod) key(bool)                           {}
func (m audioMod) emit(dst []int16, s int8) []int16 { return m.a.AppendSymbol(dst, s) }

type mcuAudioMod struct {
	m   MCUAudio
	buf [maxSamples]int16
}

func (*mcuAudioMod) key(bool) {}
func (m *mcuAudioMod) emit(dst []int16, s int8) []int16 {
	return append(dst, m.buf[:m.m.Symbol(m.buf[:], s)]...)
}

type iqMod struct {
	q  *IQ
	s8 bool
	b  []byte
}

func (m *iqMod) key(on bool) { m.q.Key(on) }
func (m *iqMod) emit(dst []int16, s int8) []int16 {
	if !m.s8 {
		return m.q.AppendS16(dst, s)
	}
	m.b = m.q.AppendS8(m.b[:0], s)
	for _, v := range m.b {
		dst = append(dst, int16(int8(v)))
	}
	return dst
}

type mcuIQMod struct {
	m   MCUIQ
	buf [2 * maxSamples << 3]int16
}

func (m *mcuIQMod) key(on bool) { m.m.Key(on) }
func (m *mcuIQMod) emit(dst []int16, s int8) []int16 {
	return append(dst, m.buf[:m.m.Symbol(m.buf[:], s)]...)
}

// mod returns a new modulator of u.
func (u txUnit) mod() (txMod, error) {
	switch {
	case u.audio() && u.mcu:
		m := &mcuAudioMod{}
		t := Table24k()
		if u.rate == 48000 {
			t = Table48k()
		}
		return m, m.m.Init(t, u.level, u.offset)
	case u.audio():
		a, err := NewAudio(AudioConfig{Hz: u.rate, Level: u.level, Offset: u.offset, Tier: u.tier})
		return audioMod{a}, err
	case u.mcu:
		m := &mcuIQMod{}
		return m, m.m.Init(u.rate, u.offHz, u.ramp)
	default:
		q, err := NewIQ(IQConfig{Rate: u.rate, OffsetHz: float64(u.offHz), RampSeconds: float64(u.ramp) / float64(u.rate), Tier: u.tier})
		return &iqMod{q: q, s8: u.format == fmtS8}, err
	}
}

// mustMod is mod that fails t on an error.
func (u txUnit) mustMod(t testing.TB) txMod {
	t.Helper()
	m, err := u.mod()
	if err != nil {
		t.Fatalf("%v: %v", u, err)
	}
	return m
}

// lag returns the delay of the output of u behind its symbols, in symbols:
// the half-length of its pulse.
func (u txUnit) lag() int {
	switch {
	case u.audio() && (u.mcu || u.tier == Tier0) && u.rate == 48000:
		return Table48k().Span()
	case u.audio() && (u.mcu || u.tier == Tier0):
		return Table24k().Span()
	case u.mcu || u.tier == Tier0:
		return tableIQ().Span()
	case u.tier == Tier2:
		return 16
	default:
		return 8
	}
}

// amplitude returns the full-power I/Q amplitude of u in its own units.
func (u txUnit) amplitude() float64 {
	switch {
	case u.format == fmtS8:
		return float64((iqAmplitude + 1<<7) >> 8)
	case u.mcu || u.tier == Tier0:
		return iqAmplitude
	default:
		return 0.9 * math.MaxInt16
	}
}

// phaseStep returns the most error of the phase of u between two samples,
// in radians: the 12 bits of the cosine table of Tier0, which it does not
// interpolate.
func (u txUnit) phaseStep() float64 {
	if u.mcu || u.tier == Tier0 {
		return 2 * math.Pi / (1 << cosTableBits)
	}
	return 0
}

// peakHz returns the most deviation of any run of symbols for u, from its
// taps.
func (u txUnit) peakHz() float64 {
	switch {
	case u.audio() && (u.mcu || u.tier == Tier0) && u.rate == 48000:
		return 3 * taps48kPeak * 3 * DeviationHz / tableUnits
	case u.audio() && (u.mcu || u.tier == Tier0):
		return 3 * taps24kPeak * 3 * DeviationHz / tableUnits
	case u.mcu || u.tier == Tier0:
		return 3 * tapsIQPeak * 3 * DeviationHz / tableUnits
	}
	hz := u.rate
	if !u.audio() {
		hz = iqShaperHz
	}
	span := 8
	if u.tier == Tier2 {
		span = 16
	}
	k, err := newPoly(pulse{span: span}, hz, u.tier != Tier2)
	if err != nil {
		panic(err)
	}
	return k.peak
}

// audioBounds returns the least and the most output of the audio unit u
// for any run of symbols.
func (u txUnit) audioBounds() (lo, hi float64) {
	if u.mcu || u.tier == Tier0 {
		t := Table24k()
		if u.rate == 48000 {
			t = Table48k()
		}
		scale := int32((u.level<<15 + tableUnits/2) / tableUnits)
		return float64((-3*t.peak*scale+1<<14)>>15 + int32(u.offset)), float64((3*t.peak*scale+1<<14)>>15 + int32(u.offset))
	}
	x := u.peakHz() * float64(u.level) / (3 * DeviationHz)
	return float64(u.offset) - x, float64(u.offset) + x
}

// keying is a change of the key of an I/Q modulator before the symbol at.
type keying struct {
	at int
	on bool
}

// run returns the output of a new modulator of u for the symbols, keyed
// by keys, which are in order.
func (u txUnit) run(t testing.TB, syms []int8, keys []keying) []int16 {
	t.Helper()
	m := u.mustMod(t)
	return runMod(m, syms, keys)
}

// runMod returns the output of m for the symbols, keyed by keys.
func runMod(m txMod, syms []int8, keys []keying) []int16 {
	var out []int16
	k := 0
	for i, s := range syms {
		for k < len(keys) && keys[k].at == i {
			m.key(keys[k].on)
			k++
		}
		out = m.emit(out, s)
	}
	return out
}

// keyedOn returns the keying of a carrier keyed up from the start.
func keyedOn() []keying { return []keying{{at: 0, on: true}} }

// iqComplex returns the interleaved I/Q raw as complex samples.
func iqComplex(raw []int16) []complex128 {
	return fromIQ(raw)
}

// audioFM returns the FM signal at dacRate of the output raw of the audio
// unit u through the radio r, with the offset of u taken out, as the bias
// of the modulator input does, and the gain of r scaled for the level of
// u.
func (u txUnit) audioFM(raw []int16, r radio) []complex128 {
	a := make([]int16, len(raw))
	g := float64(tableUnits) / float64(u.level)
	for i, v := range raw {
		a[i] = int16(int(v) - u.offset)
	}
	if r.gain == 0 {
		r.gain = 1
	}
	r.gain *= g
	return dacFM(a, u.rate, r)
}

// signal48 returns the output raw of u as a unit-level baseband signal at
// rxRate: the FM signal of an audio unit through the radio r, or the I/Q
// of an I/Q unit moved to 0 Hz.
func (u txUnit) signal48(raw []int16, r radio) []complex128 {
	if u.audio() {
		return resample(u.audioFM(raw, r), dacRate, rxRate)
	}
	x := shift(iqComplex(raw), float64(u.rate), -float64(u.offHz))
	return resample(x, u.rate, rxRate)
}

// shift returns x at rate moved by hz.
func shift(x []complex128, rate, hz float64) []complex128 {
	if hz == 0 {
		return x
	}
	y := make([]complex128, len(x))
	for i, v := range x {
		s, c := math.Sincos(2 * math.Pi * hz * float64(i) / rate)
		y[i] = v * complex(c, s)
	}
	return y
}

// The symbol patterns of the matrix.
const (
	patPlus3 = iota
	patMinus3
	patAlt3
	patRuns1
	patAlt1
	patWorst
	patLongRuns
	patRandom
	patSync
	patETSI
	patCount
)

// patternName names the pattern p.
func patternName(p int) string {
	return [...]string{"plus3", "minus3", "alt3", "runs1", "alt1", "worst", "longruns", "random", "sync", "etsi"}[p]
}

// pattern returns n symbols of the pattern p for u, between runs of 0 that
// let the pulse settle.
func (u txUnit) pattern(p, n int) []int8 {
	lead := 2 * u.lag()
	out := make([]int8, lead, 2*lead+n)
	switch p {
	case patPlus3, patMinus3:
		v := int8(3)
		if p == patMinus3 {
			v = -3
		}
		for range n {
			out = append(out, v)
		}
	case patAlt3, patAlt1:
		v := int8(3)
		if p == patAlt1 {
			v = 1
		}
		for i := range n {
			out = append(out, v*int8(1-2*(i&1)))
		}
	case patRuns1:
		for i := range n {
			out = append(out, int8(1-2*(i/7&1)))
		}
	case patWorst:
		// Each copy of the worst symbols, of either sign, ends in a symbol
		// period whose samples take the phase of the worst sum.
		w, ph, l, m := u.worst()
		acc, sign, free := 0, int8(1), lead
		for k := 0; k < lead+n || sign < 0 || free == lead; k++ {
			if k >= len(out) {
				out = append(out, 0)
			}
			hit := false
			for a := acc; a < l; a += m {
				hit = hit || a == ph
			}
			if hit && k+1-len(w) >= free {
				for j, v := range w {
					out[k+1-len(w)+j] = sign * v
				}
				free, sign = k+1, -sign
			}
			for acc < l {
				acc += m
			}
			acc -= l
		}
	case patLongRuns:
		for i := range n {
			out = append(out, int8(3-6*(i/500&1)))
		}
	case patRandom:
		rng := rand.New(rand.NewPCG(uint64(n), 7))
		for range n {
			out = append(out, int8(2*rng.IntN(4)-3))
		}
	case patSync:
		pats := []enums.SyncPattern{enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice, enums.SyncMSSourcedData,
			enums.SyncMSSourcedRC, enums.SyncTDMA1Voice, enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data}
		for i := 0; len(out) < lead+n; i++ {
			v := uint64(pats[i%len(pats)])
			for j := 46; j >= 0; j -= 2 {
				out = appendBits(out, []unpacked.Bit{unpacked.Bit(v >> uint(j+1) & 1), unpacked.Bit(v >> uint(j) & 1)})
			}
		}
		out = out[:lead+n]
	case patETSI:
		for i := range n {
			out = append(out, int8(3-6*(i/2&1)))
		}
	}
	return append(out, make([]int8, lead)...)
}

// worst returns the symbols, oldest first, that drive the output of u to
// its most deviation: 3 times the sign of each tap of its phase ph of the
// largest sum of tap magnitudes. l/m is the rate of its shaper in samples
// per symbol.
func (u txUnit) worst() (w []int8, ph, l, m int) {
	m = 1
	var taps [][]float64
	switch {
	case u.audio() && (u.mcu || u.tier == Tier0) && u.rate == 48000:
		for _, r := range taps48k {
			taps = append(taps, intRow(r[:]))
		}
		l = len(taps48k)
	case u.audio() && (u.mcu || u.tier == Tier0):
		for _, r := range taps24k {
			taps = append(taps, intRow(r[:]))
		}
		l = len(taps24k)
	case u.mcu || u.tier == Tier0:
		for _, r := range tapsIQ {
			taps = append(taps, intRow(r[:]))
		}
		l = len(tapsIQ)
	default:
		hz := u.rate
		if !u.audio() {
			hz = iqShaperHz
		}
		span := 8
		if u.tier == Tier2 {
			span = 16
		}
		k, err := newPoly(pulse{span: span}, hz, false)
		if err != nil {
			panic(err)
		}
		for p := range k.l {
			taps = append(taps, k.taps64[p*k.n:p*k.n+k.n])
		}
		l, m = k.l, k.m
	}
	best, bestSum := 0, 0.0
	for p, r := range taps {
		var s float64
		for _, v := range r {
			s += math.Abs(v)
		}
		if s > bestSum {
			best, bestSum = p, s
		}
	}
	r := taps[best]
	w = make([]int8, len(r))
	for i, v := range r {
		w[len(r)-1-i] = int8(3 - 6*boolInt(v < 0))
	}
	return w, best, l, m
}

func intRow(r []int16) []float64 {
	out := make([]float64, len(r))
	for i, v := range r {
		out[i] = float64(v)
	}
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// newMSStream builds an MS outbound transmission of a group voice call of
// n superframes on one slot: each burst keyed up and down within its slot,
// as an MS transmits, with the slots between them silent, and the keying
// that u needs for it, which leads the bursts by its lag.
func newMSStream(t testing.TB, u txUnit, n int) (*testStream, []keying) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, Group: true, Wakeups: 1, Superframes: n}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 1200
	ts := &testStream{lead: lead}
	ts.syms = make([]int8, lead, lead+2*len(b.Bursts)*144+60)
	keys := make([]keying, 0, 2*len(b.Bursts))
	pos := enums.VoiceBurstUnknown
	for _, burst := range b.Bursts {
		x, err := layer2.NewBurstFromBytes(burst)
		kind := kindIdle
		switch {
		case err == nil && x.Kind == layer2.KindVoice && x.SyncPattern == enums.SyncMSSourcedVoice:
			pos, kind = enums.VoiceBurstA, kindVoice
		case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || x.Kind != layer2.KindData):
			pos++
			kind = kindVoice
		case err == nil && x.Kind == layer2.KindData:
			pos, kind = enums.VoiceBurstUnknown, kindData
		}
		start := len(ts.syms) + 12
		keys = append(keys, keying{at: start - 6 + u.lag(), on: true}, keying{at: start + 132 + u.lag(), on: false})
		ts.syms = append(ts.syms, make([]int8, 12)...)
		var bits [264]unpacked.Bit
		unpacked.UnpackAt(bits[:], burst[:], 0)
		ts.syms = appendBits(ts.syms, bits[:])
		ts.syms = append(ts.syms, make([]int8, 144)...)
		ts.slots = append(ts.slots, rftx.BSSlot{Burst: burst}, rftx.BSSlot{})
		ts.kind = append(ts.kind, kind, kindIdle)
	}
	ts.syms = append(ts.syms, make([]int8, 60)...)
	return ts, keys
}

// cleanSymbol returns s as a modulator takes it: a symbol other than -3,
// -1, +1 and +3 counts as 0.
func cleanSymbol(s int8) int8 {
	switch s {
	case -3, -1, 1, 3:
		return s
	}
	return 0
}

// checkExact fails unless sc holds every burst bit for bit, less miss
// bursts that a receiver needs to lock.
func checkExact(t *testing.T, name string, sc rawScore, miss int) {
	t.Helper()
	if sc.bursts == 0 || sc.exact < sc.bursts-miss {
		t.Errorf("%s: %d of %d bursts exact, %d of %d bits wrong", name, sc.exact, sc.bursts, sc.errs, sc.bits)
	}
}
