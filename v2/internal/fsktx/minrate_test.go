//go:build !tinygo

package fsktx

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

var inputLowpassGrid = [...]int{0, lowpass8k, lowpass5k, lowpass3k}

var updateMinRates = flag.Bool("update-minrate", false, "rewrite minrate_gen.go from the measurement of the full rate grid (needs DMRGO_LONG=1)")

const minRateFile = "minrate_gen.go"

func longRun() bool { return os.Getenv("DMRGO_LONG") == "1" }

type limRadio int

const (
	radioPlain limRadio = iota
	radioIdeal
)

func (r limRadio) String() string { return [...]string{"plain", "ideal"}[r] }

type limPoint struct {
	u       txUnit
	bits    int
	radio   limRadio
	lowpass int
}

func (p limPoint) String() string {
	if p.u.audio() {
		return fmt.Sprintf("%s/%dbit/%s/lp%d", p.u, p.bits, p.radio, p.lowpass)
	}
	return p.u.String()
}

func measuredLevel(s Setup, hz int) int {
	level, depth := s.Tier0Level(), s.MeasuredBits()
	if s.Tier == Tier0 || depth == 0 || depth == 16 {
		return level
	}
	half := 1<<(depth-1) - 1
	peak := func(l int) float64 {
		a, err := NewAudio(AudioConfig{Hz: hz, Level: l, Tier: s.Tier})
		if err != nil {
			return math.Inf(1)
		}
		return math.Ceil(a.Peak())
	}
	top := level >> (s.Bits - depth)
	l := top
	if pk := peak(top); pk > float64(half) {
		l = min(top-1, int(float64(top)*float64(half)/pk))
		for l > 0 && peak(l) > float64(half) {
			l--
		}
		for l > 0 && l < top-1 && peak(l+1) <= float64(half) {
			l++
		}
	}
	return l << (s.Bits - depth)
}

func audioRa(hz int) float64 { return float64(max(192000, min(8*hz, 384000))) }

func holdFreq(d []float64, hz, ra float64) []float64 {
	n := int(float64(len(d)) * ra / hz)
	f := make([]float64, n)
	k := 0
	for i := range f {
		t0, t1 := float64(i)/ra, float64(i+1)/ra
		var acc float64
		for t0 < t1 && k < len(d) {
			b := float64(k+1) / hz
			e := min(t1, b)
			acc += d[k] * (e - t0)
			t0 = e
			if e == b {
				k++
			}
		}
		f[i] = acc * ra
	}
	return f
}

func idealFreq(d []float64, hz, ra float64) []float64 {
	const half, sub = 32, 256
	beta := 0.1102 * (100 - 8.7)
	tab := make([]float64, 2*half*sub+2)
	for i := range tab {
		t := float64(i)/sub - half
		v := 1.0
		if t != 0 {
			v = math.Sin(math.Pi*t) / (math.Pi * t)
		}
		tab[i] = v * taper(winKaiser, beta, t/(half+1))
	}
	n := int(float64(len(d)) * ra / hz)
	f := make([]float64, n)
	for i := range f {
		t := float64(i) * hz / ra
		k0 := int(math.Floor(t))
		var s float64
		for k := k0 - half + 1; k <= k0+half; k++ {
			if k < 0 || k >= len(d) {
				continue
			}
			x := (t - float64(k) + half) * sub
			j := int(x)
			if j < 0 || j+1 >= len(tab) {
				continue
			}
			w := tab[j] + (tab[j+1]-tab[j])*(x-float64(j))
			s += d[k] * w
		}
		f[i] = s
	}
	return f
}

func fmOf(f []float64, ra float64) []complex128 {
	x := make([]complex128, len(f))
	var ph float64
	for i, v := range f {
		ph = math.Mod(ph+2*math.Pi*v/ra, 2*math.Pi)
		x[i] = cmplx.Rect(1, ph)
	}
	return x
}

func (p limPoint) audioSignal(raw []int16) ([]complex128, float64) {
	hz := float64(p.u.rate)
	ra := audioRa(p.u.rate)
	d := make([]float64, len(raw))
	g := 3 * DeviationHz / float64(p.u.level)
	for i, v := range raw {
		d[i] = float64(int(v)-p.u.offset) * g
	}
	var f []float64
	switch p.radio {
	case radioIdeal:
		f = idealFreq(d, hz, ra)
	default:
		f = holdFreq(d, hz, ra)
	}
	if p.lowpass > 0 {
		f = analogLowpass(f, ra, float64(p.lowpass))
	}
	return fmOf(f, ra), ra
}

func analogLowpass(f []float64, rate, fc float64) []float64 {
	x := make([]complex128, len(f))
	for i, v := range f {
		x[i] = complex(v, 0)
	}
	xf := spectrumOf(x)
	m := len(xf)
	for i := range xf {
		k := i
		if k >= m/2 {
			k -= m
		}
		w := float64(k) * rate / float64(m) / fc
		xf[i] /= complex(1-w*w, math.Sqrt2*w)
	}
	ifft(xf)
	out := make([]float64, len(f))
	for i := range out {
		out[i] = real(xf[i])
	}
	return out
}

func decimate(x []complex128, rate float64) ([]complex128, float64) {
	const pass = 60000.0
	for rate >= 4*pass+40000 {
		stop := rate/2 - pass
		trans := (stop - pass) / rate
		n := int(math.Ceil((100-8)/(2.285*2*math.Pi*trans))) | 1
		half := n / 2
		fc := (pass + stop) / 2 / rate
		beta := 0.1102 * (100 - 8.7)
		h := make([]float64, n)
		var sum float64
		for j := range h {
			t := float64(j - half)
			v := 2 * fc
			if t != 0 {
				v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
			}
			h[j] = v * taper(winKaiser, beta, t/float64(half+1))
			sum += h[j]
		}
		y := make([]complex128, len(x)/2)
		for i := range y {
			var re, im float64
			c := 2 * i
			for j, w := range h {
				if k := c + j - half; k >= 0 && k < len(x) {
					re += real(x[k]) * w
					im += imag(x[k]) * w
				}
			}
			y[i] = complex(re/sum, im/sum)
		}
		x, rate = y, rate/2
	}
	return x, rate
}

// ETSI TS 102 361-1 clause 10.2.2.3
func limDeviation(x []complex128, rate float64) float64 {
	lp := butter2(4000, rate)
	var hi, lo float64
	for i := 1; i < len(x); i++ {
		y := lp.step(cmplx.Phase(x[i]*cmplx.Conj(x[i-1])) * rate / (2 * math.Pi))
		if i > len(x)/4 && i < 3*len(x)/4 {
			hi, lo = max(hi, y), min(lo, y)
		}
	}
	return 100 * (max(hi, -lo)/2749 - 1)
}

type limStreams struct {
	cont []int8
	ms   *testStream
	keys []keying
}

var streamCache sync.Map

func limStreamsOf(t testing.TB, u txUnit, superframes int) limStreams {
	key := fmt.Sprint(u.lag(), superframes, u.audio())
	if v, ok := streamCache.Load(key); ok {
		if s, ok := v.(limStreams); ok {
			return s
		}
	}
	s := limStreams{cont: newTestStream(t, superframes, 40).syms}
	if !u.audio() {
		s.ms, s.keys = newMSStream(t, u, superframes)
	}
	streamCache.Store(key, s)
	return s
}

func (p limPoint) measure(t testing.TB, superframes int) map[string]float64 {
	t.Helper()
	st := limStreamsOf(t, p.u, superframes)
	var m limMeasures
	etsi := p.u.pattern(patETSI, 4000)
	if p.u.audio() {
		x, ra := p.audioSignal(p.u.run(t, st.cont, nil))
		n := len(x)
		c := x[n/8 : n*7/8]
		pc := onPower(c)
		m.cont = limStatic(c, ra, pc)
		m.wide = limFar(c, ra)
		e, era := p.audioSignal(p.u.run(t, etsi, nil))
		m.devPct, m.hasDev = limDeviation(e, era), true
		return limMargins(m)
	}
	rate := float64(p.u.rate)
	full := shift(iqComplex(p.u.run(t, st.cont, keyedOn())), rate, -float64(p.u.offHz))
	n := len(full)
	c := full[n/8 : n*7/8]
	m.wide = limFar(c[:min(len(c), int(0.15*rate))], rate)
	pc := onPower(c)
	m.wide["far_pc"] = pc
	d, ra := decimate(c, rate)
	m.cont = limStatic(d, ra, onPower(d))
	e, era := decimate(shift(iqComplex(p.u.run(t, etsi, keyedOn())), rate, -float64(p.u.offHz)), rate)
	m.devPct, m.hasDev = limDeviation(e, era), true
	if p.u.offHz == 0 {
		zs := make([]int8, len(st.ms.syms))
		for _, z := range []struct {
			dst  *map[string]float64
			syms []int8
		}{{&m.ms, st.ms.syms}, {&m.msz, zs}} {
			x, xr := decimate(iqComplex(p.u.run(t, z.syms, st.keys)), rate)
			*z.dst = limTransient(x, xr, onPower(x), z.dst == &m.ms)
		}
	}
	return limMargins(m)
}

const (
	limWorld   = LimitsWorld
	limUS      = LimitsUS
	limCEPT    = LimitsCEPT
	limChina   = LimitsChina
	limJapan   = LimitsJapan
	limRegions = LimitsJapan + 1
)

const (
	limLandMobile = LimitsLandMobile
	limAmateur    = LimitsAmateur
	limServices   = LimitsAmateur + 1
)

var limRegionNames = [limRegions]string{"World", "US", "CEPT", "China", "Japan"}
var limServiceNames = [limServices]string{"LandMobile", "Amateur"}

func enKeys(pw float64) []string {
	k := []string{"ts_dev_pct"}
	for _, nm := range []string{"acp", "alt"} {
		k = append(k, "en_"+nm+"_annexc_nominal_db", "en_"+nm+"_annexc_wide_db", "en_"+nm+"_bins_db")
	}
	k = append(k, "en_tacp_annexc_unmod_db", "en_tacp_sa_unmod_db", "en_tp_unmod_ms", "en_td_unmod_ms", "en_slope_unmod_db", "en_df_unmod_khz")
	return append(k, powKey("en_spur_", pw, "w_db"))
}

func regimeKeys(region, service int, pw float64) []string {
	switch region {
	case limWorld:
		var all []string
		for r := limUS; r < limRegions; r++ {
			all = append(all, regimeKeys(r, service, pw)...)
		}
		slices.Sort(all)
		return slices.Compact(all)
	case limUS:
		if service == limAmateur {
			return []string{"ts_dev_pct", powKey("am_us97e_", pw, "w_db")}
		}
		return []string{"ts_dev_pct", powKey("fcc_maskd_", pw, "w_db"), powKey("fcc_maskd_far_", pw, "w_db")}
	case limCEPT:
		if service == limAmateur {
			return []string{"ts_dev_pct", powKey("am_erc_", pw, "w_db")}
		}
		return enKeys(pw)
	case limChina:
		return []string{"ts_dev_pct", "cn_alt_db", "obw_cn_kr_khz", "en_acp_annexc_nominal_db", "en_acp_annexc_wide_db", "en_acp_bins_db",
			"en_tacp_annexc_unmod_db", "en_tacp_sa_unmod_db", "en_talt_annexc_unmod_db", "en_talt_sa_unmod_db"}
	}
	return []string{"ts_dev_pct", "jp_acp_db", "obw_jp_khz"}
}

func regimeFails(g map[string]float64, region, service int, pw float64) []string {
	var bad []string
	for _, k := range regimeKeys(region, service, pw) {
		v, ok := g[k]
		if ok && v < limNeed(k) {
			bad = append(bad, fmt.Sprintf("%s %+.2f", k, v))
		}
	}
	return bad
}

type limClass struct {
	tier   Tier
	iq     bool
	class  int
	format txFormat
	bits   int
	mcu    bool
}

func (c limClass) String() string {
	if c.iq {
		return fmt.Sprintf("IQ-T%d-%s", c.tier-Tier0, [...]string{"", "S16", "S8"}[c.format])
	}
	return fmt.Sprintf("Audio-T%d-%dbit", c.tier-Tier0, c.bits)
}

var audioBitClasses = []int{8, 10, 12, 16}

func limClasses() []limClass {
	out := make([]limClass, 0, 3*(len(audioBitClasses)+2))
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		for i, b := range audioBitClasses {
			out = append(out, limClass{tier: tier, class: i, bits: b, mcu: tier == Tier0})
		}
		for i, f := range []txFormat{fmtS16, fmtS8} {
			out = append(out, limClass{tier: tier, iq: true, class: i, format: f, mcu: tier == Tier0 && f == fmtS16})
		}
	}
	return out
}

func audioGrid(tier Tier) []int {
	if tier == Tier0 {
		return []int{24000, 48000}
	}
	var r []int
	for hz := 8000; hz <= 48000; hz += 500 {
		r = append(r, hz)
	}
	return append(r, 96000)
}

func iqGrid(tier Tier) []int {
	if tier == Tier0 {
		return []int{96000}
	}
	set := map[int]bool{}
	for k := 0; k <= 33; k++ {
		set[int(math.Round(62500*math.Pow(2, float64(k)/4)))] = true
	}
	for _, r := range []int{62500, 96000, 96001, 192000, 250000, 250001, 384000, 768000, 1000000, 1024000, 1400000, 1800000, 1920000,
		2000000, 2048000, 2400000, 2400001, 2560000, 3200000, 4000000, 5000000, 6000000, 8000000, 10000000, 12500000, 15000000, 16000000, 20000000} {
		set[r] = true
	}
	var out []int
	for r := range set {
		if r <= 20000000 {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out
}

func iqOffsets(tier Tier, rate int) []int {
	q := rate/4 - 1
	if tier != Tier0 {
		return []int{0, q}
	}
	out := []int{0}
	for _, o := range []int{12500, 25000} {
		if o < q {
			out = append(out, o)
		}
	}
	return append(out, q)
}

func (c limClass) points(rate, lowpass int) []limPoint {
	u := txUnit{mcu: c.mcu, tier: c.tier, rate: rate}
	if !c.iq {
		u.format, u.level = fmtAudio, measuredLevel(Setup{Tier: c.tier, Bits: c.bits}, rate)
		return []limPoint{{u: u, bits: c.bits, radio: radioPlain, lowpass: lowpass}, {u: u, bits: c.bits, radio: radioIdeal, lowpass: lowpass}}
	}
	u.format, u.ramp = c.format, int(math.Round(1e-3*float64(rate)))
	var out []limPoint
	for _, off := range iqOffsets(c.tier, rate) {
		v := u
		v.offHz = off
		out = append(out, limPoint{u: v})
	}
	return out
}

func worstOf(dst, g map[string]float64) {
	for k, v := range g {
		if old, ok := dst[k]; !ok || v < old {
			dst[k] = v
		}
	}
}

func measureClassRate(t testing.TB, c limClass, rate, superframes, lowpass int) (map[string]float64, error) {
	g := map[string]float64{}
	for _, p := range c.points(rate, lowpass) {
		if _, err := p.u.mod(); err != nil {
			return nil, err
		}
		worstOf(g, p.measure(t, superframes))
	}
	return g, nil
}

func (c limClass) lowpasses() []int {
	if c.iq {
		return inputLowpassGrid[:1]
	}
	return inputLowpassGrid[:]
}

type limCell = [limRegions][limServices][len(limPowerClasses)]int32

type minRateTable struct {
	audio [3][4][len(inputLowpassGrid)]limCell
	iq    [3][2]limCell
}

var limPowerClasses = [...]float64{1, 10, 100, 1000}

func (tb *minRateTable) cell(c limClass, lp int) *limCell {
	if c.iq {
		return &tb.iq[c.tier-Tier0][c.class]
	}
	return &tb.audio[c.tier-Tier0][c.class][lp]
}

func minRateOf(rates []int, pass []bool, floor int) int32 {
	least := int32(0)
	for i := len(rates) - 1; i >= 0; i-- {
		if !pass[i] || rates[i] < floor {
			break
		}
		least = int32(rates[i])
	}
	return least
}

func gridFloor(iq bool) int {
	if iq {
		return 62500
	}
	return 0
}

func renderMinRates(tb *minRateTable) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by TestMinRatesGenerated in minrate_test.go. DO NOT EDIT.\n\npackage fsktx\n\n")
	cell := func(x *limCell, unit int32) error {
		for r := range limRegions {
			b.WriteString("{")
			for s := range limServices {
				b.WriteString("{")
				for p := range limPowerClasses {
					if x[r][s][p]%unit != 0 {
						return fmt.Errorf("rate %d is not a multiple of %d", x[r][s][p], unit)
					}
					fmt.Fprintf(&b, "%d,", x[r][s][p]/unit)
				}
				b.WriteString("},")
			}
			b.WriteString("},\n")
		}
		return nil
	}
	dims := fmt.Sprintf("[%d][%d][%d]", limRegions, limServices, len(limPowerClasses))
	fmt.Fprintf(&b, "var minRateAudio = [3][4][%d]%suint16{\n", len(inputLowpassGrid), dims)
	for t := range 3 {
		b.WriteString("{\n")
		for c := range 4 {
			b.WriteString("{\n")
			for lp := range inputLowpassGrid {
				b.WriteString("{\n")
				if err := cell(&tb.audio[t][c][lp], minRateUnit); err != nil {
					return nil, err
				}
				b.WriteString("},\n")
			}
			b.WriteString("},\n")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "var minRateIQ = [3][2]%sint32{\n", dims)
	for t := range 3 {
		b.WriteString("{\n")
		for c := range 2 {
			b.WriteString("{\n")
			if err := cell(&tb.iq[t][c], 1); err != nil {
				return nil, err
			}
			b.WriteString("},\n")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

func TestMinRatesGenerated(t *testing.T) {
	t.Parallel()
	if !longRun() {
		t.Skip("the full rate grid runs with DMRGO_LONG=1; TestMinRatesSample checks a sample of it")
	}
	var tb minRateTable
	var mu sync.Mutex
	var log []string
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	for _, c := range limClasses() {
		rates := iqGrid(c.tier)
		if !c.iq {
			rates = audioGrid(c.tier)
		}
		for li, lp := range c.lowpasses() {
			res := make([]map[string]float64, len(rates))
			for i, r := range rates {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					g, err := measureClassRate(t, c, r, 1, lp)
					if err != nil {
						return
					}
					res[i] = g
				}()
			}
			wg.Wait()
			cell := tb.cell(c, li)
			for reg := range limRegions {
				for s := range limServices {
					for p, pw := range limPowerClasses {
						pass := make([]bool, len(rates))
						for i, g := range res {
							pass[i] = g != nil && len(regimeFails(g, reg, s, pw)) == 0
						}
						cell[reg][s][p] = minRateOf(rates, pass, gridFloor(c.iq))
					}
				}
			}
			mu.Lock()
			for i, g := range res {
				if g == nil {
					continue
				}
				log = append(log, fmt.Sprintf("%s lp%d %d %s", c, lp, rates[i], summary(g)))
			}
			mu.Unlock()
		}
	}
	if p := os.Getenv("DMRGO_MINRATE_LOG"); p != "" {
		if err := os.WriteFile(p, []byte(strings.Join(log, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src, err := renderMinRates(&tb)
	if err != nil {
		t.Fatal(err)
	}
	if *updateMinRates {
		if err := os.WriteFile(minRateFile, src, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(minRateFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Errorf("%s differs from the measurement; run DMRGO_LONG=1 go test ./internal/fsktx -run TestMinRatesGenerated -update-minrate", minRateFile)
	}
}

func testParallel() int {
	if v := flag.Lookup("test.parallel"); v != nil {
		var n int
		if _, err := fmt.Sscan(v.Value.String(), &n); err == nil {
			return n
		}
	}
	return 2
}

func summary(g map[string]float64) string {
	keys := make([]string, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%.3f", k, g[k])
	}
	return b.String()
}

func (c limClass) setup(region, service, power, lowpass int) Setup {
	bits := c.bits
	if c.iq {
		bits = 16
		if c.format == fmtS8 {
			bits = 8
		}
	}
	return Setup{Tier: c.tier, IQ: c.iq, Bits: bits, Region: region, Service: service, Power: power, InputLowpassHz: lowpass}
}

func checkAgainstTable(t *testing.T, c limClass, rate, lowpass int, g map[string]float64) {
	t.Helper()
	for r := range limRegions {
		for s := range limServices {
			for p, pw := range limPowerClasses {
				su := c.setup(r, s, p, lowpass)
				if bad := regimeFails(g, r, s, pw); su.Allows(rate) && len(bad) > 0 {
					t.Errorf("%s lp%d at %d Hz, %s %s %g W: the table allows it from %d Hz, but %v", c, lowpass, rate, limRegionNames[r], limServiceNames[s], pw, su.MinRate(), bad)
				}
			}
		}
	}
}

func classOf(tier Tier, iq bool, f txFormat, bits int) limClass {
	for _, c := range limClasses() {
		if c.tier == tier && c.iq == iq && (iq && c.format == f || !iq && c.bits == bits) {
			return c
		}
	}
	return limClass{}
}

func TestMinRatesSample(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c       limClass
		rate    int
		lowpass int
	}{
		{classOf(Tier0, true, fmtS16, 0), 96000, 0},
		{classOf(Tier0, false, fmtAudio, 16), 48000, 0},
		{classOf(Tier1, false, fmtAudio, 16), 48000, 0},
		{classOf(Tier1, false, fmtAudio, 12), 24000, 5000},
		{classOf(Tier1, true, fmtS16, 0), 96000, 0},
	}
	if testmode.Race {
		cases = cases[:1]
	}
	for _, k := range cases {
		t.Run(fmt.Sprint(k.c, "/", k.rate, "/lp", k.lowpass), func(t *testing.T) {
			t.Parallel()
			g, err := measureClassRate(t, k.c, k.rate, 1, k.lowpass)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(summary(g))
			checkAgainstTable(t, k.c, k.rate, k.lowpass, g)
		})
	}
}

func TestMinRatesOffGrid(t *testing.T) {
	t.Parallel()
	type rc struct {
		c    limClass
		rate int
	}
	cases := []rc{
		{classOf(Tier1, false, fmtAudio, 16), 44100},
		{classOf(Tier1, true, fmtS16, 0), 100003},
	}
	if testmode.Race {
		cases = cases[:1]
	}
	if longRun() {
		rng := rand.New(rand.NewPCG(5, 6))
		for _, c := range limClasses() {
			if c.tier == Tier0 {
				continue
			}
			for range 4 {
				if c.iq {
					cases = append(cases, rc{c, int(62500 * math.Pow(320, rng.Float64()))})
					continue
				}
				for {
					hz := 8000 + rng.IntN(40000)
					if hz%500 != 0 && hz/gcd(hz, 4800) <= polyMaxPhases {
						cases = append(cases, rc{c, hz})
						break
					}
				}
			}
		}
	}
	for _, k := range cases {
		t.Run(fmt.Sprint(k.c, "/", k.rate), func(t *testing.T) {
			t.Parallel()
			g, err := measureClassRate(t, k.c, k.rate, 1, 0)
			if err != nil {
				t.Skipf("structural: %v", err)
			}
			checkAgainstTable(t, k.c, k.rate, 0, g)
		})
	}
}

func TestSetupAllows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		s    Setup
		hz   int
		want bool
	}{
		{Setup{Tier: Tier0, IQ: true, Bits: 16, Region: LimitsUS}, 192000, false},
		{Setup{Tier: Tier0, IQ: true, Bits: 16, Region: LimitsUS}, 48000, false},
		{Setup{Tier: Tier0, IQ: true, Bits: 16, Region: LimitsUS}, 96000, true},
		{Setup{Tier: Tier1, IQ: true, Bits: 16, Region: LimitsUS}, 192000, true},
		{Setup{Tier: Tier0, Bits: 16, Region: LimitsUS, Service: LimitsAmateur}, 32000, false},
		{Setup{Tier: Tier0, Bits: 16, Region: LimitsUS, Service: LimitsAmateur}, 48000, true},
		{Setup{IQ: true, Bits: 16}, 2400000, true},
		{Setup{IQ: true, Bits: 16}, 20000001, false},
		{Setup{IQ: true, Bits: 16}, 40000, false},
		{Setup{IQ: true, Bits: 12}, 2400000, false},
		{Setup{Bits: 16}, 48000, true},
		{Setup{Bits: 16}, 22051, false},
		{Setup{Bits: 16}, 96001, false},
		{Setup{Bits: 16}, -48000, false},
		{Setup{Bits: 7}, 48000, false},
		{Setup{Tier: Tier2 + 1, Bits: 16}, 48000, false},
		{Setup{Bits: 16, Region: limRegions}, 48000, false},
		{Setup{Bits: 16, Power: -1}, 48000, false},
	} {
		if got := c.s.Allows(c.hz); got != c.want {
			t.Errorf("%+v at %d Hz: Allows %v, want %v (MinRate %d)", c.s, c.hz, got, c.want, c.s.MinRate())
		}
	}
}

func TestTier0Budget(t *testing.T) {
	t.Parallel()
	for _, iq := range []bool{false, true} {
		for r := range limRegions {
			for s := range limServices {
				for p := range limPowerClasses {
					su := Setup{Tier: Tier0, IQ: iq, Bits: 16, Region: r, Service: s, Power: p}
					for _, hz := range []int{12000, 24000, 48000, 96000, 192000, 384000} {
						if ok := iq && hz == 96000 || !iq && (hz == 24000 || hz == 48000); su.Allows(hz) && !ok {
							t.Errorf("%+v allows %d Hz, outside the Tier0 budget", su, hz)
						}
					}
				}
			}
		}
	}
}

func TestInputLowpassRounds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ hz, grid int }{
		{0, 0}, {-1, 0}, {8001, 0}, {20000, 0}, {8000, 8000}, {6000, 8000}, {5001, 8000},
		{5000, 5000}, {3001, 5000}, {3000, 3000}, {1000, 3000}, {1, 3000},
	} {
		if g := inputLowpassGrid[lowpassClass(c.hz)]; g != c.grid {
			t.Errorf("a stated %d Hz low pass counts as %d Hz, want %d", c.hz, g, c.grid)
		}
		for _, b := range audioBitClasses {
			for r := range limRegions {
				for p := range limPowerClasses {
					a := Setup{Bits: b, Region: r, Power: p, InputLowpassHz: c.hz}
					g := a
					g.InputLowpassHz = c.grid
					if a.MinRate() != g.MinRate() {
						t.Errorf("%+v: MinRate %d, but %d at the grid cutoff %d Hz", a, a.MinRate(), g.MinRate(), c.grid)
					}
				}
			}
		}
	}
}

func TestZeroSetupsAllowed(t *testing.T) {
	t.Parallel()
	if s := (Setup{IQ: true, Bits: 16}); !s.Allows(2400000) {
		t.Errorf("Tier1 16-bit I/Q at 2.4 MHz, World land mobile 1 W: refused, least rate %d", s.MinRate())
	}
	if s := (Setup{Bits: 16}); !s.Allows(48000) {
		t.Errorf("Tier1 16-bit audio at 48 kHz, World land mobile 1 W: refused, least rate %d", s.MinRate())
	}
}
