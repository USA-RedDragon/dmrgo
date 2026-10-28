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
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func txFull() bool { return os.Getenv("DMRGO_TXMATRIX") == "full" }

func raceUnits(units []txUnit) []txUnit {
	if !testmode.Race {
		return units
	}
	out := make([]txUnit, 0, len(units)/4+1)
	for i := 0; i < len(units); i += 4 {
		out = append(out, units[i])
	}
	return out
}

type txFormat int

const (
	fmtAudio txFormat = iota
	fmtS16
	fmtS8
)

type txUnit struct {
	mcu           bool
	pe            bool
	tier          Tier
	format        txFormat
	rate          int
	level, offset int
	offHz         int
	ramp          int
}

func (u txUnit) kind() string {
	switch {
	case u.format == fmtAudio && u.mcu && u.pe:
		return "MCUAudio-PE"
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

func (u txUnit) audio() bool { return u.format == fmtAudio }

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

type mcuAudioPEMod struct {
	m   MCUAudioPE
	buf [peSamples]int16
}

func (m *mcuAudioPEMod) key(bool) {}
func (m *mcuAudioPEMod) emit(dst []int16, s int8) []int16 {
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

func (u txUnit) mod() (txMod, error) {
	switch {
	case u.audio() && u.mcu && u.pe:
		m := &mcuAudioPEMod{}
		return m, m.m.Init(u.level, u.offset)
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

func (u txUnit) mustMod(t testing.TB) txMod {
	t.Helper()
	m, err := u.mod()
	if err != nil {
		t.Fatalf("%v: %v", u, err)
	}
	return m
}

func (u txUnit) lag() int {
	switch {
	case u.audio() && u.pe:
		return peSpan
	case u.audio() && (u.mcu || u.tier == Tier0) && u.rate == 48000:
		return Table48k().Span()
	case u.audio() && (u.mcu || u.tier == Tier0):
		return Table24k().Span()
	case u.mcu || u.tier == Tier0:
		return iqSpan
	}
	switch m := u.designMod().(type) {
	case audioMod:
		return m.a.lag()
	case *iqMod:
		return m.q.k.n/2 + m.q.dn
	}
	return 8
}

func (u txUnit) designMod() txMod {
	v := u
	if v.audio() {
		v.level, v.offset = tableUnits, 0
	} else {
		v.offHz, v.ramp = 0, max(1, v.rate/1000)
	}
	m, err := v.mod()
	if err != nil {
		return nil
	}
	return m
}

func (u txUnit) linear() bool {
	switch m := u.designMod().(type) {
	case audioMod:
		return m.a.corr == nil && m.a.shape == 0 && m.a.bpairs == 0
	case *iqMod:
		return m.q.pred == nil
	}
	return true
}

func (u txUnit) amplitude() float64 {
	switch {
	case u.format == fmtS8 && (u.mcu || u.tier == Tier0):
		return float64((iqAmplitude + 1<<7) >> 8)
	case u.format == fmtS8:
		return math.Floor((math.Floor(0.9*math.MaxInt16+0.5) + 1<<7) / 256)
	case u.mcu || u.tier == Tier0:
		return iqAmplitude
	default:
		return 0.9 * math.MaxInt16
	}
}

func (u txUnit) phaseStep() float64 {
	if u.mcu || u.tier == Tier0 {
		return 2 * math.Pi / (1 << cosTableBits)
	}
	return 0
}

func (u txUnit) peakHz() float64 {
	switch {
	case u.audio() && (u.mcu || u.tier == Tier0) && u.rate == 48000:
		return 3 * taps48kPeak * 3 * DeviationHz / (tableUnits << tapShift)
	case u.audio() && (u.mcu || u.tier == Tier0):
		return 3 * taps24kPeak * 3 * DeviationHz / (tableUnits << tapShift)
	case u.mcu || u.tier == Tier0:
		return 3 * tapsIQPeak * 3 * DeviationHz / tableUnits
	}
	switch m := u.designMod().(type) {
	case audioMod:
		return m.a.peak
	case *iqMod:
		return m.q.k.peak + predBound(m.q.pred)
	}
	return 0
}

func predBound(p *predictor) float64 {
	if p == nil {
		return 0
	}
	var a [predK]float64
	for t := range a {
		if p.kc != nil {
			for _, v := range p.kc[t] {
				a[t] += math.Abs(v)
			}
		} else {
			for _, v := range p.kern[t] {
				a[t] += math.Abs(v)
			}
		}
	}
	var b float64
	for ph := range predSPS {
		var s float64
		for t := ph; t < predK; t += predSPS {
			s += a[t]
		}
		b = max(b, s)
	}
	return 2 * b * p.gain * iqShaperHz / (2 * math.Pi)
}

func (u txUnit) audioBounds() (lo, hi float64) {
	if u.mcu || u.tier == Tier0 {
		t := Table24k()
		if u.rate == 48000 {
			t = Table48k()
		}
		lo, hi := t.bounds(int32((u.level<<15 + tableUnits/2) / tableUnits))
		return float64(lo + u.offset), float64(hi + u.offset)
	}
	if m, ok := u.mustDesign().(audioMod); ok {
		return m.a.bounds()
	}
	x := u.peakHz() * float64(u.level) / (3 * DeviationHz)
	return float64(u.offset) - x, float64(u.offset) + x
}

func (u txUnit) mustDesign() txMod {
	m, err := u.mod()
	if err != nil {
		return nil
	}
	return m
}

type keying struct {
	at int
	on bool
}

func (u txUnit) run(t testing.TB, syms []int8, keys []keying) []int16 {
	t.Helper()
	m := u.mustMod(t)
	return runMod(m, syms, keys)
}

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

func keyedOn() []keying { return []keying{{at: 0, on: true}} }

func iqComplex(raw []int16) []complex128 {
	return fromIQ(raw)
}

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

func (u txUnit) signal48(raw []int16, r radio) []complex128 {
	if u.audio() {
		return resample(u.audioFM(raw, r), dacRate, rxRate)
	}
	x := shift(iqComplex(raw), float64(u.rate), -float64(u.offHz))
	return resample(x, u.rate, rxRate)
}

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

func patternName(p int) string {
	return [...]string{"plus3", "minus3", "alt3", "runs1", "alt1", "worst", "longruns", "random", "sync", "etsi"}[p]
}

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
		var k *poly
		switch d := u.designMod().(type) {
		case audioMod:
			k = d.a.k
		case *iqMod:
			k = d.q.k
		}
		if k == nil {
			return nil, 0, 1, 1
		}
		for p := range k.l {
			row := make([]float64, k.n)
			for i := range row {
				if k.taps64 != nil {
					row[i] = k.taps64[p*k.n+i]
				} else {
					row[i] = float64(k.taps32[p*k.n+i])
				}
			}
			taps = append(taps, row)
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

func cleanSymbol(s int8) int8 {
	switch s {
	case -3, -1, 1, 3:
		return s
	}
	return 0
}

func checkExact(t *testing.T, name string, sc rawScore, miss int) {
	t.Helper()
	if sc.bursts == 0 || sc.exact < sc.bursts-miss {
		t.Errorf("%s: %d of %d bursts exact, %d of %d bits wrong", name, sc.exact, sc.bursts, sc.errs, sc.bits)
	}
}

func (a *Audio) lag() int {
	if a.k == nil {
		return a.mcu.k.t.span
	}
	if a.cbp == nil {
		return a.k.n / 2
	}
	return a.k.n/2 + int(math.Round(float64(len(a.cbp)/2)*a.step/float64(a.spp)))
}
