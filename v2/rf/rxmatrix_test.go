//go:build !tinygo

package rf

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type mxXFail struct {
	checks string
	bug    string
}

const (
	mxStart = iota
	mxLead
	mxOffset
	mxSource
	mxContent
	mxClock
	mxLevel
	mxFormat
	mxCN
	mxDC
	mxDims
	mxBaseDims = mxDC
)

var mxValues = [mxDims][]string{
	{"s0", "gap", "mid", "agc"},
	{"none", "noise", "cw500ms", "cw2s"},
	{"+0", "+700", "-700", "+1500", "-1500", "+2500", "-2500", "+3000", "-3000", "+4000", "-4000"},
	{"bs", "ms", "ms2"},
	{"voice", "data12", "data34", "data1", "csbk", "rc", "pi"},
	{"0ppm", "+500ppm", "-500ppm"},
	{"nom", "low", "high"},
	{"a9600", "a12000", "a24000", "a48000", "cu8-48k", "cs16-48k", "cu8-2048k", "cs16-2048k", "cs16-9600"},
	{"inf", "40dB"},
	{"none", "dc"},
}

const (
	mxSecondsMean = 2e-6
	mxSeconds     = 5.2e-6
	mxSecondsCN   = 10.4e-6
	mxDevHigh     = 0.03
	mxDevLow      = 0.88
	mxDevIQ       = 0.95
	mxDevLowRate  = 0.90
	mxAudioDC     = 300
	mxAGCSteps    = 8
	mxTail        = 0.06
	mxAGCBlock    = float64(frontBlock) / 9600
	mxIQRate      = 2048000
	mxOuterHz     = 3 * 648.0
	mxCutSeconds  = slotSeconds / 2
	mxDCRe        = -0.7123
	mxDCIm        = -0.7019
)

type mxRow [mxDims]uint8

func (r mxRow) val(d int) string { return mxValues[d][r[d]] }

func (r mxRow) name() string {
	s := make([]string, 0, mxDims)
	for d := range mxDims {
		if d < mxBaseDims || r[d] != 0 {
			s = append(s, r.val(d))
		}
	}
	return strings.Join(s, ".")
}

func (r mxRow) seed() uint64 {
	r[mxDC] = 0
	return rxSeed(r.name())
}

func mxPairOK(d1 int, v1 uint8, d2 int, v2 uint8) bool {
	if d1 > d2 {
		d1, v1, d2, v2 = d2, v2, d1, v1
	}
	switch {
	case d1 == mxStart && d2 == mxLead:
		start, lead := mxValues[mxStart][v1], mxValues[mxLead][v2]
		return (lead != "noise" || start != "gap" && start != "mid") && (lead == "none" || start != "mid")
	case d1 == mxSource && d2 == mxContent:
		return mxValues[mxSource][v1] != "bs" || mxValues[mxContent][v2] != "rc"
	case d1 == mxFormat && d2 == mxDC:
		return v2 == 0 || !strings.HasPrefix(mxValues[mxFormat][v1], "a")
	}
	return true
}

type mxPair struct{ d1, v1, d2, v2 uint8 }

func mxPairs(dims int) []mxPair {
	var out []mxPair
	for d1 := range dims {
		for d2 := d1 + 1; d2 < dims; d2++ {
			for v1 := range mxValues[d1] {
				for v2 := range mxValues[d2] {
					if mxPairOK(d1, uint8(v1), d2, uint8(v2)) {
						out = append(out, mxPair{uint8(d1), uint8(v1), uint8(d2), uint8(v2)})
					}
				}
			}
		}
	}
	return out
}

func mxCovers(r mxRow, p mxPair) bool { return r[p.d1] == p.v1 && r[p.d2] == p.v2 }

func mxBaselines() []mxRow {
	out := make([]mxRow, 0, 2*len(mxValues[mxFormat]))
	for f := range mxValues[mxFormat] {
		for _, src := range []uint8{0, 1} {
			out = append(out, mxRow{mxStart: 3, mxLead: 1, mxSource: src, mxContent: 1, mxFormat: uint8(f)})
		}
	}
	return out
}

func mxValueCost(d, v int) float64 {
	switch d {
	case mxLead:
		return []float64{0, 0.3, 0.5, 2}[v]
	case mxFormat:
		return mxFormatCost(mxValues[mxFormat][v])
	case mxSource:
		return []float64{1, 0, 1}[v]
	}
	return 0
}

func mxFormatCost(f string) float64 {
	switch {
	case strings.HasSuffix(f, "2048k"):
		return 6
	case strings.HasSuffix(f, "48k"):
		return 2
	}
	return 1
}

func mxRowCost(r mxRow) float64 {
	return (0.6 + mxValueCost(mxLead, int(r[mxLead]))) * mxFormatCost(r.val(mxFormat))
}

func mxCover() []mxRow {
	return mxGreedy(mxFlip(mxGreedy(nil, mxBaseDims)), mxDims)
}

func mxGreedy(rows []mxRow, dims int) []mxRow {
	pairs := mxPairs(dims)
	left, open := mxLeft(pairs, rows)
	rng := rand.New(rand.NewPCG(7, 11))
	for open > 0 {
		seed := pairs[slices.Index(left, true)]
		var best mxRow
		bestGain := -1.0
		for range 24 {
			var row [mxDims]int
			for d := range dims {
				row[d] = -1
			}
			row[seed.d1], row[seed.d2] = int(seed.v1), int(seed.v2)
			for _, d := range rng.Perm(dims) {
				if row[d] >= 0 {
					continue
				}
				pick, pickGain := -1, -1
				for _, v := range rng.Perm(len(mxValues[d])) {
					ok, gain := true, 0
					for e := range dims {
						if row[e] < 0 {
							continue
						}
						if !mxPairOK(d, uint8(v), e, uint8(row[e])) {
							ok = false
							break
						}
					}
					if !ok {
						continue
					}
					for i, p := range pairs {
						if !left[i] {
							continue
						}
						a, b := int(p.d1), int(p.d2)
						switch {
						case a == d && int(p.v1) == v && row[b] == int(p.v2), b == d && int(p.v2) == v && row[a] == int(p.v1):
							gain++
						}
					}
					if gain > pickGain || gain == pickGain && mxValueCost(d, v) < mxValueCost(d, pick) {
						pick, pickGain = v, gain
					}
				}
				row[d] = pick
			}
			var r mxRow
			for d := range r {
				r[d] = uint8(row[d])
			}
			n := 0
			for i, p := range pairs {
				if left[i] && mxCovers(r, p) {
					n++
				}
			}
			if gain := float64(n) / mxRowCost(r); gain > bestGain {
				best, bestGain = r, gain
			}
		}
		for i, p := range pairs {
			if left[i] && mxCovers(best, p) {
				left[i] = false
				open--
			}
		}
		rows = append(rows, best)
	}
	return rows
}

func mxLeft(pairs []mxPair, rows []mxRow) ([]bool, int) {
	left := make([]bool, len(pairs))
	open := 0
	for i, p := range pairs {
		left[i] = !slices.ContainsFunc(rows, func(r mxRow) bool { return mxCovers(r, p) })
		if left[i] {
			open++
		}
	}
	return left, open
}

func mxRowOK(r mxRow) bool {
	for d1 := range mxDims {
		for d2 := d1 + 1; d2 < mxDims; d2++ {
			if !mxPairOK(d1, r[d1], d2, r[d2]) {
				return false
			}
		}
	}
	return true
}

func mxFlip(rows []mxRow) []mxRow {
	pairs := mxPairs(mxDims)
	n := make([]int, len(pairs))
	for i, p := range pairs {
		for _, r := range rows {
			if mxCovers(r, p) {
				n[i]++
			}
		}
	}
	for {
		bi, bd, bv, bg := -1, 0, uint8(0), 0
		for i, r := range rows {
			for d := mxBaseDims; d < mxDims; d++ {
				for v := range mxValues[d] {
					f := r
					f[d] = uint8(v)
					if f == r || !mxRowOK(f) {
						continue
					}
					g, ok := 0, true
					for k, p := range pairs {
						was, now := mxCovers(r, p), mxCovers(f, p)
						switch {
						case now && !was && n[k] == 0:
							g++
						case was && !now && n[k] == 1:
							ok = false
						}
					}
					if ok && g > bg {
						bi, bd, bv, bg = i, d, uint8(v), g
					}
				}
			}
		}
		if bi < 0 {
			return rows
		}
		f := rows[bi]
		f[bd] = bv
		for k, p := range pairs {
			if mxCovers(rows[bi], p) {
				n[k]--
			}
			if mxCovers(f, p) {
				n[k]++
			}
		}
		rows[bi] = f
	}
}

type mxRx struct {
	format string
	tier   Tier
}

func (x mxRx) name() string { return "t" + strconv.Itoa(int(x.tier-Tier0)) }

func mxReceivers(format string, t3 bool) []mxRx {
	if format == "cs16-9600" {
		return []mxRx{{format, Tier0}}
	}
	out := []mxRx{{format, Tier0}, {format, Tier1}, {format, Tier2}}
	if t3 {
		out = append(out, mxRx{format, Tier3})
	}
	return out
}

func mxCostly(row mxRow) bool {
	f := row.val(mxFormat)
	return !strings.HasPrefix(f, "a") && f != "cs16-9600"
}

type mxCase struct {
	row mxRow
	rxs []mxRx
}

func mxPlan() []mxCase {
	base, rows := mxBaselines(), mxCover()
	var need [mxDims][]bool
	for d := range need {
		need[d] = make([]bool, len(mxValues[d]))
		for v := range need[d] {
			need[d][v] = true
		}
	}
	for v := range mxValues[mxFormat] {
		if !mxCostly(mxRow{mxFormat: uint8(v)}) {
			need[mxFormat][v] = false
		}
	}
	t3 := make([]bool, len(rows))
	for _, dims := range []int{mxBaseDims, mxDims} {
		for {
			best, gain := -1, 0
			for i, r := range rows {
				if t3[i] || !mxCostly(r) {
					continue
				}
				g := 0
				for d := range dims {
					if need[d][r[d]] {
						g++
					}
				}
				if g > gain {
					best, gain = i, g
				}
			}
			if best < 0 {
				break
			}
			t3[best] = true
			for d := range mxDims {
				need[d][rows[best][d]] = false
			}
		}
	}
	out := make([]mxCase, 0, len(base)+len(rows))
	for _, r := range base {
		out = append(out, mxCase{r, mxReceivers(r.val(mxFormat), !mxCostly(r))})
	}
	for i, r := range rows {
		out = append(out, mxCase{r, mxReceivers(r.val(mxFormat), t3[i] || !mxCostly(r))})
	}
	return out
}

type mxTruth struct {
	at      float64
	data    layer2.RawBurst
	cach    [3]byte
	hasCACH bool
	bs      bool
	rc      bool
	who     int
	g       int
}

type mxPart struct {
	f, e []float64
	t0   float64
	amp  float64
	hz   float64
}

type mxSignal struct {
	parts []mxPart
	r     float64
	end   float64
	sigma float64
	seed  uint64
	truth []mxTruth
	what  string
}

type mxParams struct {
	slot      int
	lead2     int
	frac      float64
	diffDB    float64
	weakFirst bool
	dHz       float64
	agc       int
}

func mxParamsOf(row mxRow) mxParams {
	h := row.seed()
	return mxParams{
		slot:      int(h & 1),
		lead2:     int(h>>1) % 3,
		frac:      0.13 + 0.75*float64((h>>3)%16)/15,
		diffDB:    []float64{0, 7, 13, 20}[(h>>8)%4],
		weakFirst: (h>>10)&1 == 1,
		dHz:       []float64{150, -200, 250}[(h>>11)%3],
		agc:       int(h>>13) % mxAGCSteps,
	}
}

func mxTransmission(row mxRow, src uint32, slot int, bs bool) (f, e []float64, truth []mxTruth, first float64, err error) {
	cfg := rftx.SeqConfig{ColorCode: 1, Source: src, Dest: 9990, BS: 310123, Message: "DMRGO MATRIX", Superframes: 1, Slot: slot, BSSourced: bs}
	seq := row.val(mxContent)
	switch seq {
	case "voice":
		cfg.LateEntry = true
	case "csbk":
		seq = "radiocheck"
	case "rc":
		cfg.Wakeups = 1
	}
	built, err := rftx.Sequence(seq, cfg)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	ramp := (slotSeconds - burstSeconds) / 2
	if bs {
		f, e = rftx.ModulateBS(built.Slots)
		for k, s := range built.Slots {
			truth = append(truth, mxTruth{at: ramp + float64(k)*slotSeconds + float64(cachSymbols)/symbolRate, data: s.Burst, cach: s.CACH, hasCACH: true, bs: true, g: k})
		}
		return f, e, truth, 0, nil
	}
	f, e = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: slot})
	for i, b := range built.Bursts {
		x, err := layer2.NewBurstFromBytes(b)
		truth = append(truth, mxTruth{at: float64(2*i+slot)*slotSeconds + ramp, data: b, rc: err == nil && x.SyncPattern == enums.SyncMSSourcedRC, g: 2*i + slot})
	}
	return f, e, truth, float64(slot) * slotSeconds, nil
}

func mxBuild(row mxRow) (*mxSignal, error) {
	p := mxParamsOf(row)
	ppm := map[string]float64{"0ppm": 0, "+500ppm": 500, "-500ppm": -500}[row.val(mxClock)]
	off, _ := strconv.ParseFloat(row.val(mxOffset), 64)
	s := &mxSignal{r: 1 + ppm*1e-6, seed: row.seed()}
	if row.val(mxCN) == "40dB" {
		s.sigma = math.Sqrt(1/(math.Pow(10, 4)*12500)) / math.Sqrt2
	}
	src := row.val(mxSource)
	f, e, truth, first, err := mxTransmission(row, 3191868, p.slot, src == "bs")
	if err != nil {
		return nil, err
	}
	ramp := (slotSeconds - burstSeconds) / 2
	lead := map[string]float64{"none": 0, "noise": 0, "cw500ms": 0.5, "cw2s": 2}[row.val(mxLead)]
	content := 0.0
	if lead > 0 {
		n := int(lead * basebandRate)
		rn := int(ramp * basebandRate)
		cf, ce := make([]float64, n, n+len(f)), make([]float64, n, n+len(e))
		for i := range ce {
			ce[i] = 1
			if i < rn {
				ce[i] = 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(rn))
			}
		}
		e = slices.Clone(e)
		for i := 0; i < len(e) && e[i] < 1; i++ {
			e[i] = 1
		}
		f, e = append(cf, f...), append(ce, e...)
		content, first = float64(n)/basebandRate, 0
	}
	var t0 float64
	switch row.val(mxStart) {
	case "s0":
		t0 = -first
	case "gap":
		t0 = -first - ramp/2
	case "mid":
		t0 = -first - mxCutSeconds
		if src == "bs" {
			t0 -= ramp
		}
	case "agc":
		t0 = -first + 0.1 + float64(p.agc)*mxAGCBlock/mxAGCSteps
	}
	if row.val(mxLead) == "noise" {
		t0 += 0.3
	}
	t0 /= s.r
	a1, a2 := 1.0, 1.0
	if src == "ms2" {
		if p.weakFirst {
			a1 = math.Pow(10, -p.diffDB/20)
		} else {
			a2 = math.Pow(10, -p.diffDB/20)
		}
	}
	s.parts = append(s.parts, mxPart{f: f, e: e, t0: t0, amp: a1, hz: off})
	for _, tr := range truth {
		tr.at = t0 + (content+tr.at)/s.r
		s.truth = append(s.truth, tr)
	}
	if src == "ms2" {
		f2, e2, truth2, _, err := mxTransmission(row, 3191869, 1-p.slot, false)
		if err != nil {
			return nil, err
		}
		t2 := t0 + (content+float64(2*p.lead2)*slotSeconds)/s.r + p.frac/symbolRate
		s.parts = append(s.parts, mxPart{f: f2, e: e2, t0: t2, amp: a2, hz: off + p.dHz})
		for _, tr := range truth2 {
			tr.at = t2 + tr.at/s.r
			tr.who, tr.g = 1, tr.g+2*p.lead2
			s.truth = append(s.truth, tr)
		}
		s.what = fmt.Sprintf("second talker %d frames later +%.2f symbol, %+.0f Hz, %.0f dB %s", p.lead2, p.frac, p.dHz, p.diffDB, map[bool]string{true: "stronger", false: "weaker"}[p.weakFirst])
	}
	for _, q := range s.parts {
		s.end = max(s.end, q.t0+float64(len(q.f))/basebandRate/s.r)
	}
	s.end += mxTail
	burst := float64(burstSymbols) / symbolRate / s.r
	cach := float64(cachSymbols) / symbolRate / s.r
	s.truth = slices.DeleteFunc(s.truth, func(tr mxTruth) bool { return tr.at < 0 || tr.at+burst > s.end })
	for i := range s.truth {
		if s.truth[i].at-cach < 0 {
			s.truth[i].hasCACH = false
		}
	}
	slices.SortFunc(s.truth, func(a, b mxTruth) int { return cmp.Compare(a.at, b.at) })
	return s, nil
}

func (s *mxSignal) synth(rate float64, emit func(re, im float64)) {
	n := int(s.end * rate)
	noise := mxNoise()
	at := int(s.seed % mxNoiseLen)
	sigma := s.sigma * math.Sqrt(rate)
	slow := rate <= basebandRate
	rot := make([]complex128, len(s.parts))
	prev := make([]float64, len(s.parts))
	for j := range rot {
		rot[j], prev[j] = 1, math.NaN()
	}
	for i := range n {
		t := float64(i) / rate
		var re, im float64
		if sigma > 0 {
			re, im = float64(noise[at])*sigma, float64(noise[at+1])*sigma
			at = (at + 2) % mxNoiseLen
		}
		for j := range s.parts {
			q := &s.parts[j]
			u := (t - q.t0) * s.r * basebandRate
			if u < 0 {
				continue
			}
			k := int(u)
			if k+1 >= len(q.f) {
				continue
			}
			w := u - float64(k)
			f := q.f[k] + (q.f[k+1]-q.f[k])*w
			a := q.amp * (q.e[k] + (q.e[k+1]-q.e[k])*w)
			if math.IsNaN(prev[j]) {
				prev[j] = f
			}
			x := 2 * math.Pi * ((f+prev[j])/2 + q.hz) / rate
			prev[j] = f
			var step complex128
			if slow {
				sn, cs := math.Sincos(x)
				step = complex(cs, sn)
			} else {
				x2 := x * x
				step = complex(1-x2/2+x2*x2/24, x-x*x2/6+x*x2*x2/120)
			}
			z := rot[j] * step
			z *= complex((3-(real(z)*real(z)+imag(z)*imag(z)))/2, 0)
			rot[j] = z
			re += a * real(z)
			im += a * imag(z)
		}
		emit(re, im)
	}
}

const mxNoiseLen = 1 << 22

var mxNoise = sync.OnceValue(func() []float32 {
	rng := rand.New(rand.NewPCG(5, 6))
	x := make([]float32, mxNoiseLen+1)
	for i := range x {
		x[i] = float32(rng.NormFloat64())
	}
	return x
})

func (s *mxSignal) baseband() []complex64 {
	x := make([]complex64, 0, int(s.end*basebandRate))
	s.synth(basebandRate, func(re, im float64) { x = append(x, complex(float32(re), float32(im))) })
	return x
}

func mxIQLevel(format, level string) float64 {
	switch level {
	case "low":
		if strings.HasPrefix(format, "cu8") {
			return 5 / 127.5
		}
		return 0.0072
	case "high":
		return 0.99
	}
	return 0.72
}

func mxAudioGain(level string, offHz float64) float64 {
	switch level {
	case "low":
		return audioGain / 100
	case "high":
		return 30000 / (math.Abs(offHz) + 2900)
	}
	return audioGain
}

func appendIQ(b []byte, u8 bool, re, im float64) []byte {
	if u8 {
		return append(b, mxU8(re), mxU8(im))
	}
	return binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(b, mxS16(re)), mxS16(im))
}

func mxU8(v float64) byte {
	return byte(int(max(0, min(255, v*127.5+128))))
}

func mxS16(v float64) uint16 {
	return uint16(int(max(0, min(65535, v*32767+32768.5))) - 32768)
}

type mxInput struct {
	audio []int16
	hz    int
	iq    []byte
	rate  int
	iqFmt IQFormat
	delay float64
	gain  float64
}

func mxInputOf(s *mxSignal, row mxRow) mxInput {
	format, level := row.val(mxFormat), row.val(mxLevel)
	off, _ := strconv.ParseFloat(row.val(mxOffset), 64)
	if hz, ok := strings.CutPrefix(format, "a"); ok {
		in := mxInput{gain: mxAudioGain(level, off)}
		in.hz, _ = strconv.Atoi(hz)
		in.delay = secondsAudioLag / basebandRate
		in.audio = discAudio(s.baseband(), in.hz)
		for i, v := range in.audio {
			hzv := (float64(v) - audioOffset) / audioGain
			in.audio[i] = int16(max(math.MinInt16, min(math.MaxInt16, math.Round(hzv*in.gain+mxAudioDC))))
		}
		return in
	}
	a := mxIQLevel(format, level)
	in := mxInput{iqFmt: IQFormatCS16}
	u8 := strings.HasPrefix(format, "cu8")
	if u8 {
		in.iqFmt = IQFormatCU8
	}
	var dcRe, dcIm float64
	if row.val(mxDC) != "none" {
		lo := mxIQLevel(format, "low")
		dcRe, dcIm = mxDCRe*lo, mxDCIm*lo
	}
	put := func(re, im float64) { in.iq = appendIQ(in.iq, u8, a*re+dcRe, a*im+dcIm) }
	switch {
	case strings.HasSuffix(format, "-9600"):
		in.rate = iq0LowHz
		for _, v := range lowIQ(s.baseband(), iq0LowHz) {
			put(float64(real(v)), float64(imag(v)))
		}
	case strings.HasSuffix(format, "-48k"):
		in.rate = basebandRate
		s.synth(basebandRate, put)
	default:
		in.rate = mxIQRate
		in.iq = make([]byte, 0, int(s.end*mxIQRate)*in.iqFmt.bytes())
		s.synth(mxIQRate, put)
	}
	return in
}

func mxReceive(in mxInput, x mxRx) ([]Burst, float64, error) {
	if in.audio != nil {
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: in.hz, Tier: x.tier})
		if err != nil {
			return nil, 0, err
		}
		var got []Burst
		for k := 0; k < len(in.audio); k += 1024 {
			got = r.AppendBursts(got, in.audio[k:min(k+1024, len(in.audio))])
		}
		got = r.AppendFlush(got)
		return got, float64(r.Level()), nil
	}
	r, err := NewReceiver(&RxConfig{Rate: in.rate, Tier: x.tier, Format: in.iqFmt})
	if err != nil {
		return nil, 0, err
	}
	var got []Burst
	for k := 0; k < len(in.iq); k += 8192 {
		got = r.AppendBursts(got, in.iq[k:min(k+8192, len(in.iq))])
	}
	got = r.AppendFlush(got)
	return got, r.DeviationHz(), nil
}

type mxResult struct {
	fail   map[string]bool
	detail []string
}

func (m *mxResult) add(check, f string, args ...any) {
	if m.fail == nil {
		m.fail = map[string]bool{}
	}
	m.fail[check] = true
	m.detail = append(m.detail, check+": "+fmt.Sprintf(f, args...))
}

func (m *mxResult) symptoms() string {
	keys := make([]string, 0, len(m.fail))
	for k := range m.fail {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return strings.Join(keys, ",")
}

type mxScorer struct {
	s       *mxSignal
	first   map[int]int
	offs    map[[2]int]int64
	streams map[int]map[int]bool
	found   map[string][]string
}

func (c *mxScorer) tag(j int) string {
	t := "#" + strconv.Itoa(j)
	if c.first[c.s.truth[j].who] == j {
		t += " first"
	}
	if c.s.truth[j].who > 0 {
		t += " talker 2"
	}
	return t
}

func (c *mxScorer) note(check, f string, args ...any) {
	c.found[check] = append(c.found[check], fmt.Sprintf(f, args...))
}

func (c *mxScorer) burst(b *Burst, j int) {
	tr := c.s.truth[j]
	if b.Data != tr.data {
		c.note("bits", "%s %d bits", c.tag(j), hamming(b.Data[:], tr.data[:]))
	}
	if b.BS != tr.bs {
		c.note("bs", "#%d BS %v", j, b.BS)
	}
	if tr.hasCACH && (!b.HasCACH || b.CACH != tr.cach) {
		c.note("cach", "%s has %v %d bits", c.tag(j), b.HasCACH, hamming(b.CACH[:], tr.cach[:]))
	}
	if tr.rc && b.Sync != enums.SyncMSSourcedRC {
		c.note("rc", "#%d sync %v", j, b.Sync)
	}
	if c.streams[tr.who] == nil {
		c.streams[tr.who] = map[int]bool{}
	}
	c.streams[tr.who][b.Stream] = true
	k := [2]int{tr.who, b.Stream}
	if o, ok := c.offs[k]; !ok {
		c.offs[k] = b.Slot - int64(tr.g)
	} else if o != b.Slot-int64(tr.g) {
		c.note("slot", "#%d slot %d want %d", j, b.Slot, int64(tr.g)+o)
	}
}

func mxNearest(s *mxSignal, at float64) int {
	j := -1
	for k, tr := range s.truth {
		if d := math.Abs(tr.at - at); d < 0.001 && (j < 0 || d < math.Abs(s.truth[j].at-at)) {
			j = k
		}
	}
	return j
}

func mxScore(s *mxSignal, in mxInput, tier Tier, got []Burst, dev float64, cn bool, res *mxResult) {
	matched := make([]bool, len(s.truth))
	slack := burstSeconds * math.Abs(s.r-1)
	tol := mxSeconds + slack
	if cn {
		tol = mxSecondsCN + slack
	}
	c := &mxScorer{s: s, first: map[int]int{}, offs: map[[2]int]int64{}, streams: map[int]map[int]bool{}, found: map[string][]string{}}
	for j := len(s.truth) - 1; j >= 0; j-- {
		c.first[s.truth[j].who] = j
	}
	var worst, sum float64
	n := 0
	for i := range got {
		b := &got[i]
		at := b.Seconds + in.delay
		j := mxNearest(s, at)
		if j < 0 || matched[j] {
			c.note("extra", "%.4f s", b.Seconds)
			continue
		}
		matched[j] = true
		d := at - s.truth[j].at
		sum, n = sum+d, n+1
		if math.Abs(d) > tol && math.Abs(d) > math.Abs(worst) {
			worst = d
		}
		c.burst(b, j)
	}
	for w, ss := range c.streams {
		if len(ss) > 1 {
			c.note("slot", "talker %d in %d streams", w, len(ss))
		}
	}
	for j, ok := range matched {
		if !ok {
			c.note("missing", "%s at %.4f s", c.tag(j), s.truth[j].at)
		}
	}
	for _, check := range []string{"missing", "extra", "bits", "cach", "bs", "slot", "rc"} {
		if v := c.found[check]; len(v) > 0 {
			res.add(check, "%d of %d: %s", len(v), len(s.truth), strings.Join(v, "; "))
		}
	}
	if mean := sum / float64(max(n, 1)); worst != 0 || math.Abs(mean) > mxSecondsMean+slack {
		res.add("seconds", "mean %+.1f us, worst %+.1f us", mean*1e6, worst*1e6)
	}
	if n > 0 {
		mxDeviation(in, tier, dev, res)
	}
}

func mxDeviation(in mxInput, tier Tier, dev float64, res *mxResult) {
	want, lo := mxOuterHz, mxDevIQ
	switch {
	case in.audio != nil:
		want, lo = want*in.gain, mxDevLow
	case tier == Tier3:
		lo = 1 - mxDevHigh
	case in.rate == iq0LowHz:
		lo = mxDevLowRate
	}
	if dev < want*lo-0.5 || dev > want*(1+mxDevHigh)+0.5 {
		res.add("deviation", "%.1f, want %.0f to %.0f", dev, want*lo, want*(1+mxDevHigh))
	}
}

func hamming(a, b []byte) int {
	n := 0
	for i := range a {
		x := a[i] ^ b[i]
		for ; x != 0; x &= x - 1 {
			n++
		}
	}
	return n
}

type mxLimit struct {
	match  func(row mxRow, x mxRx) bool
	checks string
	reason string
	doc    string
}

func mxApplyLimits(row mxRow, x mxRx, res *mxResult, used []bool) {
	for i, l := range rxMatrixLimits {
		if !l.match(row, x) {
			continue
		}
		for c := range res.fail {
			if l.checks == "all" || slices.Contains(strings.Split(l.checks, ","), c) {
				delete(res.fail, c)
				used[i] = true
			}
		}
	}
}

func mxRun(row mxRow, rxs []mxRx) (map[mxRx]*mxResult, error) {
	s, err := mxBuild(row)
	if err != nil {
		return nil, err
	}
	in := mxInputOf(s, row)
	out := map[mxRx]*mxResult{}
	for _, x := range rxs {
		got, dev, err := mxReceive(in, x)
		if err != nil {
			return nil, err
		}
		r := &mxResult{}
		mxScore(s, in, x.tier, got, dev, row.val(mxCN) != "inf", r)
		if s.what != "" && len(r.detail) > 0 {
			r.detail = append(r.detail, s.what)
		}
		out[x] = r
	}
	return out, nil
}

// TestRxMatrix runs every receiver on a pairwise cover of the scenario
// dimensions in mxValues and scores each run against the transmitted truth.
//
// The dc dimension adds a constant I/Q offset to the I/Q formats, as an
// RTL-SDR does. On the XPR 8400 capture in cmd/dmrrf/testdata the offset is
// about 0.18 LSB at every tuner gain and, on that weak carrier, as strong as
// the carrier; the 0 Hz bin of a 62.5 Hz FFT is then about 19 dB above its
// neighbors. The matrix keeps the offset fixed in ADC units, at the carrier
// amplitude of the low level and at the angle of the capture, so the low
// level sees the same offset-to-carrier ratio and the louder levels see it
// 25 dB or more down.
func TestRxMatrix(t *testing.T) {
	t.Parallel()
	if testmode.Race {
		t.Skip("each receiver runs on one goroutine; the runs without -race check the matrix")
	}
	plan := mxPlan()
	names := map[string]bool{}
	rows := make([]mxRow, 0, len(plan))
	for _, c := range plan {
		if names[c.row.name()] {
			t.Fatalf("row %s twice", c.row.name())
		}
		names[c.row.name()] = true
		rows = append(rows, c.row)
	}
	for _, p := range mxPairs(mxDims) {
		if !slices.ContainsFunc(rows, func(r mxRow) bool { return mxCovers(r, p) }) {
			t.Fatalf("pair %s=%s %s=%s not covered", mxDimName(int(p.d1)), mxValues[p.d1][p.v1], mxDimName(int(p.d2)), mxValues[p.d2][p.v2])
		}
	}
	var mu sync.Mutex
	seen := map[string]bool{}
	used := make([]bool, len(rxMatrixLimits))
	done := 0
	for k, x := range rxMatrixXFail {
		if _, ok := rxMatrixBugs[x.bug]; x.bug != "" && !ok {
			t.Errorf("expected failure %s names the unknown bug %q", k, x.bug)
		}
	}
	t.Cleanup(func() {
		if done < len(plan) {
			return
		}
		for k := range rxMatrixXFail {
			if !seen[k] {
				t.Errorf("expected failure %s names no case of the matrix", k)
			}
		}
		for i, l := range rxMatrixLimits {
			if !used[i] {
				t.Errorf("known limit %q (%s, %s) matches no failing case", l.reason, l.checks, l.doc)
			}
		}
	})
	for _, c := range plan {
		row := c.row
		t.Run(row.name(), func(t *testing.T) {
			t.Parallel()
			res, err := mxRun(row, c.rxs)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			done++
			for _, x := range c.rxs {
				r := res[x]
				mxApplyLimits(row, x, r, used)
				key := row.name() + "/" + x.name()
				seen[key] = true
				got, want := r.symptoms(), rxMatrixXFail[key].checks
				if testing.Verbose() {
					t.Logf("MXRUN\t%s", key)
					if got != "" {
						t.Logf("MX\t%s\t%s\t%s", key, got, strings.Join(r.detail, " | "))
					}
				}
				switch {
				case got == want:
				case want == "":
					t.Errorf("%s: %s", key, strings.Join(r.detail, " | "))
				default:
					t.Errorf("%s: expected to fail with %q, fails with %q: %s", key, want, got, strings.Join(r.detail, " | "))
				}
			}
		})
	}
}

func mxDimName(d int) string {
	return []string{"start", "lead", "offset", "source", "content", "clock", "level", "format", "cn", "dc"}[d]
}
