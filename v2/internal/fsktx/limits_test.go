//go:build !tinygo

package fsktx

import (
	"math"
	"math/bits"
	"math/cmplx"
	"slices"
	"sort"
)

type dftPlan struct {
	n, m int
	w    []complex128
	bf   []complex128
	a    []complex128
}

func newDFT(n int) *dftPlan {
	p := &dftPlan{n: n}
	if n&(n-1) == 0 {
		return p
	}
	p.m = 1 << bits.Len(uint(2*n-2))
	p.w = make([]complex128, n)
	for k := range n {
		q := int64(k) * int64(k) % int64(2*n)
		p.w[k] = cmplx.Rect(1, -math.Pi*float64(q)/float64(n))
	}
	p.bf = make([]complex128, p.m)
	p.bf[0] = cmplx.Conj(p.w[0])
	for k := 1; k < n; k++ {
		p.bf[k] = cmplx.Conj(p.w[k])
		p.bf[p.m-k] = cmplx.Conj(p.w[k])
	}
	fft(p.bf)
	p.a = make([]complex128, p.m)
	return p
}

func (p *dftPlan) run(x []complex128) {
	if p.m == 0 {
		fft(x)
		return
	}
	clear(p.a)
	for k, v := range x {
		p.a[k] = v * p.w[k]
	}
	fft(p.a)
	for k := range p.a {
		p.a[k] = cmplx.Conj(p.a[k] * p.bf[k])
	}
	fft(p.a)
	s := 1 / float64(p.m)
	for k := range x {
		x[k] = cmplx.Conj(p.a[k]) * complex(s, 0) * p.w[k]
	}
}

func ifft(x []complex128) {
	for i := range x {
		x[i] = cmplx.Conj(x[i])
	}
	fft(x)
	s := 1 / float64(len(x))
	for i := range x {
		x[i] = cmplx.Conj(x[i]) * complex(s, 0)
	}
}

func blackmanHarris(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		t := 2 * math.Pi * float64(i) / float64(n)
		w[i] = 0.35875 - 0.48829*math.Cos(t) + 0.14128*math.Cos(2*t) - 0.01168*math.Cos(3*t)
	}
	return w
}

func fftFreqs(m int, rate float64) []float64 {
	f := make([]float64, m)
	for i := range f {
		f[i] = float64(i-m/2) * rate / float64(m)
	}
	return f
}

func shiftHalf(x []float64) {
	m := len(x)
	tmp := slices.Clone(x)
	for i := range x {
		x[(i+m/2)%m] = tmp[i]
	}
}

type limSpectrum struct {
	f, peak, bin []float64
	frames       int
}

// ETSI EN 300 113 clause 7.4; FCC 90.210(d)
func limSpectra(x []complex128, rate, pc, rbw float64, pad int) limSpectrum {
	n := int(math.Round(2.0044 * rate / rbw))
	w := blackmanHarris(n)
	m := n * pad
	hop := max(n/16, 1)
	plan := newDFT(m)
	buf := make([]complex128, m)
	s := limSpectrum{peak: make([]float64, m), bin: make([]float64, m)}
	var ws, ws2 float64
	for _, v := range w {
		ws += v
		ws2 += v * v
	}
	for start := 0; start+n <= len(x); start += hop {
		clear(buf)
		for i := range n {
			buf[i] = x[start+i] * complex(w[i], 0)
		}
		plan.run(buf)
		for i, v := range buf {
			p := real(v)*real(v) + imag(v)*imag(v)
			s.peak[i] = max(s.peak[i], p)
			s.bin[i] += p
		}
		s.frames++
	}
	for i := range s.peak {
		s.peak[i] /= ws * ws * pc
		s.bin[i] /= float64(s.frames) * float64(m) * ws2 * pc
	}
	shiftHalf(s.peak)
	shiftHalf(s.bin)
	s.f = fftFreqs(m, rate)
	return s
}

func padPow2(x []complex128) []complex128 {
	m := 1 << bits.Len(uint(len(x)-1))
	y := make([]complex128, m)
	copy(y, x)
	return y
}

func spectrumOf(x []complex128) []complex128 {
	y := padPow2(x)
	fft(y)
	return y
}

func filtered(xf []complex128, rate float64, n int, h func(f float64) float64) []complex128 {
	m := len(xf)
	y := make([]complex128, m)
	for i, v := range xf {
		k := i
		if k >= m/2 {
			k -= m
		}
		y[i] = v * complex(h(float64(k)*rate/float64(m)), 0)
	}
	ifft(y)
	return y[:n]
}

func gaussH(f0, rbw float64) func(float64) float64 {
	return func(f float64) float64 { return math.Exp2(-((f - f0) / (rbw / 2)) * ((f - f0) / (rbw / 2)) / 2) }
}

func maskdAttn(fd, pw float64) float64 {
	fd = math.Abs(fd) / 1000
	switch {
	case fd <= 5.625:
		return 0
	case fd <= 12.5:
		return 7.27 * (fd - 2.88)
	}
	return min(50+10*math.Log10(pw), 70)
}

var annexCurves = map[string][4][]float64{
	"nominal": {{0, 3, 4.25, 5.5, 9.5}, {0, 2, 6, 26, 90}, {0, 3, 4.25, 5.5, 9.5}, {0, 2, 6, 26, 90}},
	"wide":    {{0, 4.34, 4.35, 5.5, 9.5}, {0, 2, 6, 26, 90}, {0, 5, 6.25, 7.5, 11.5}, {0, 2, 6, 26, 90}},
}

func interp(x float64, xs, ys []float64) float64 {
	if x >= xs[len(xs)-1] {
		return 90
	}
	for i := 1; i < len(xs); i++ {
		if x <= xs[i] {
			return ys[i-1] + (ys[i]-ys[i-1])*(x-xs[i-1])/(xs[i]-xs[i-1])
		}
	}
	return 90
}

// ETSI EN 300 113 V2.2.1 Annex C, figure C.1 and tables C.1 to C.3
func annexC(f, fc float64, kind string) float64 {
	c := annexCurves[kind]
	d := (f - fc) / 1000
	toward := fc != 0 && math.Signbit(d) != math.Signbit(fc)
	if toward {
		return min(interp(math.Abs(d), c[0], c[1]), 90)
	}
	return min(interp(math.Abs(d), c[2], c[3]), 90)
}

func dbOf(v float64) float64 { return 10 * math.Log10(max(v, 1e-30)) }

func onPower(x []complex128) float64 {
	var mx float64
	for _, v := range x {
		mx = max(mx, real(v)*real(v)+imag(v)*imag(v))
	}
	var s float64
	n := 0
	for _, v := range x {
		if p := real(v)*real(v) + imag(v)*imag(v); p > mx/2 {
			s += p
			n++
		}
	}
	return s / float64(n)
}

func movingMean(x []float64, k int) []float64 {
	if len(x) < k {
		return nil
	}
	out := make([]float64, len(x)-k+1)
	var s float64
	for i, v := range x {
		s += v
		if i >= k {
			s -= x[i-k]
		}
		if i >= k-1 {
			out[i-k+1] = s / float64(k)
		}
	}
	return out
}

// ETSI EN 300 113 clauses 7.4 and 7.5; FCC 90.210(d); FCC 97.307(e); ERC/REC 74-01 Annex 6
func limStatic(x []complex128, rate, pc float64) map[string]float64 {
	out := map[string]float64{}
	s := limSpectra(x, rate, pc, 100, 2)
	y := filtered(spectrumOf(x), rate, len(x), gaussH(0, 30000))
	k := max(int(rate/30000), 1)
	p := make([]float64, len(y))
	on := make([]float64, len(y))
	for i, v := range y {
		p[i] = real(v)*real(v) + imag(v)*imag(v)
		if p[i] > pc/4 {
			on[i] = 1
		}
	}
	env, onm := movingMean(p, k), movingMean(on, k)
	var ref float64
	for i, v := range env {
		if onm[i] > 0.999 {
			ref = max(ref, v)
		}
	}
	out["maskd_ref_db"] = dbOf(ref / pc)
	hi := min(56250, rate/2-200)
	for _, pw := range []float64{1, 10, 100} {
		mg := math.Inf(1)
		for i, f := range s.f {
			if math.Abs(f) >= 5625 && math.Abs(f) <= hi {
				mg = min(mg, dbOf(ref/pc)-dbOf(s.peak[i])-maskdAttn(f, pw))
			}
		}
		out[powKey("maskd_", pw, "w")] = mg
	}
	var tot float64
	for _, v := range s.bin {
		tot += v
	}
	for _, kind := range []string{"nominal", "wide"} {
		for _, c := range []struct {
			nm  string
			off float64
		}{{"acp", 12500}, {"alt", 25000}} {
			var v float64
			for _, o := range []float64{c.off, -c.off} {
				var a float64
				for i, f := range s.f {
					a += s.bin[i] * math.Pow(10, -annexC(f, o, kind)/10)
				}
				v = max(v, a)
			}
			out[c.nm+"_annexc_"+kind+"_dbc"] = dbOf(v / tot)
		}
	}
	band := func(lo, hi float64) float64 {
		var a float64
		for i, f := range s.f {
			if f >= lo && f <= hi {
				a += s.bin[i]
			}
		}
		return a
	}
	for _, c := range []struct {
		nm  string
		off float64
	}{{"acp", 12500}, {"alt", 25000}} {
		if c.off+4250 <= rate/2 {
			out[c.nm+"_bins_dbc"] = dbOf(max(band(c.off-4250, c.off+4250), band(-c.off-4250, -c.off+4250)) / tot)
		}
	}
	out["jp_acp_dbc"] = dbOf(max(band(10500, 14500), band(-14500, -10500)) / tot)
	for _, c := range []struct {
		nm string
		lo float64
	}{{"us97", 19000}, {"erc", 62500}} {
		if c.lo+1000 <= rate/2 {
			out["spur_"+c.nm+"_dbc"] = dbOf(max(band(c.lo, math.Inf(1)), band(math.Inf(-1), -c.lo)) / tot)
		}
	}
	var c float64
	lo, hiF := math.NaN(), math.NaN()
	for i, v := range s.bin {
		c += v / tot
		if math.IsNaN(lo) && c >= 0.005 {
			lo = s.f[i]
		}
		if math.IsNaN(hiF) && c >= 0.995 {
			hiF = s.f[i]
		}
	}
	out["obw_hz"] = hiF - lo
	return out
}

func powKey(pre string, pw float64, post string) string {
	return pre + itoa(int(pw)) + post
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

var farLimits = []struct {
	nm string
	lo float64
}{{"fcc", 56250}, {"erc", 62500}, {"us97", 19000}}

// FCC 90.210(d)(3); ERC/REC 74-01 Annex 6; ETSI EN 300 113 clause 7.5; FCC 97.307(e)
func limFar(x []complex128, rate float64) map[string]float64 {
	n := int(rate / 1000)
	w := blackmanHarris(n)
	hop := n / 2
	plan := newDFT(n)
	buf := make([]complex128, n)
	P := make([]float64, n)
	for start := 0; start+n <= len(x); start += hop {
		for i := range n {
			buf[i] = x[start+i] * complex(w[i], 0)
		}
		plan.run(buf)
		for i, v := range buf {
			P[i] += real(v)*real(v) + imag(v)*imag(v)
		}
	}
	f := make([]float64, n)
	for i := range f {
		k := i
		if k >= (n+1)/2 {
			k -= n
		}
		f[i] = float64(k) * rate / float64(n)
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return f[idx[a]] < f[idx[b]] })
	fs := make([]float64, n)
	cum := make([]float64, n+1)
	for j, i := range idx {
		fs[j] = f[i]
		cum[j+1] = cum[j] + P[i]
	}
	tot := cum[n]
	sum := func(lo, hi float64) float64 {
		a := sort.SearchFloat64s(fs, lo)
		b := sort.SearchFloat64s(fs, hi)
		return cum[b] - cum[a]
	}
	out := map[string]float64{}
	for _, l := range farLimits {
		if l.lo+1000 > rate/2 {
			continue
		}
		var best float64
		for e := l.lo; e < rate/2; e += 1000 {
			best = max(best, sum(e, e+100000), sum(-e-100000+1e-9, -e+1e-9))
		}
		out["far_"+l.nm+"_dbc"] = dbOf(best / tot)
	}
	return out
}

func limEdges(p []float64, lo, hi float64) (att, rel [][2]int) {
	for i := 1; i < len(p); i++ {
		if p[i] >= lo && p[i-1] < lo {
			for j := i; j < len(p); j++ {
				if p[j] < lo {
					break
				}
				if p[j] >= hi {
					att = append(att, [2]int{i, j})
					break
				}
			}
		}
		if p[i] < hi && p[i-1] >= hi {
			for j := i; j < len(p); j++ {
				if p[j] >= hi {
					break
				}
				if p[j] < lo {
					rel = append(rel, [2]int{i, j})
					break
				}
			}
		}
	}
	return att, rel
}

// ETSI EN 300 113 clause 7.9
func limTransient(x []complex128, rate, pc float64, modulated bool) map[string]float64 {
	out := map[string]float64{}
	xf := spectrumOf(x)
	for _, c := range []struct {
		nm  string
		off float64
	}{{"tacp", 12500}, {"talt", 25000}} {
		if c.off+9500 > rate/2 {
			continue
		}
		for _, kind := range []string{"nominal", "wide"} {
			var v float64
			for _, o := range []float64{c.off, -c.off} {
				y := filtered(xf, rate, len(x), func(f float64) float64 { return math.Pow(10, -annexC(f, o, kind)/20) })
				for _, s := range y {
					v = max(v, real(s)*real(s)+imag(s)*imag(s))
				}
			}
			out[c.nm+"_annexc_"+kind+"_dbc"] = dbOf(v / pc)
		}
	}
	a := math.Exp(-2 * math.Pi * 10000 / rate)
	for _, c := range []struct {
		nm  string
		off float64
	}{{"tacp", 8250}, {"talt", 25000}} {
		if c.off+2000 > rate/2 {
			continue
		}
		var v float64
		for _, o := range []float64{c.off, -c.off} {
			y := filtered(xf, rate, len(x), gaussH(o, 1000))
			var vy float64
			for _, s := range y {
				vy = (1-a)*(real(s)*real(s)+imag(s)*imag(s)) + a*vy
				v = max(v, vy)
			}
		}
		out[c.nm+"_sa_dbc"] = dbOf(v / pc)
	}
	limTiming(out, filtered(xf, rate, len(x), gaussH(0, 30000)), rate, pc, modulated)
	return out
}

// ETSI EN 300 113 clause 7.9.4.1
func limTiming(out map[string]float64, xs []complex128, rate, pc float64, modulated bool) {
	p := make([]float64, len(xs))
	for i, v := range xs {
		p[i] = (real(v)*real(v) + imag(v)*imag(v)) / pc
	}
	att, rel := limEdges(p, 1e-3, math.Pow(10, -0.6))
	if len(att) == 0 || len(rel) == 0 {
		return
	}
	tp, td := math.Inf(1), math.Inf(1)
	lp := make([]float64, len(p))
	for i, v := range p {
		lp[i] = 10 * math.Log10(max(v, 1e-30))
	}
	var rev float64
	for _, e := range att {
		tp = min(tp, float64(e[1]-e[0])/rate*1e3)
		d := math.Inf(1)
		for k := e[1]; k >= e[0]; k-- {
			d = min(d, lp[k])
			rev = max(rev, lp[k]-d)
		}
	}
	for _, e := range rel {
		td = min(td, float64(e[1]-e[0])/rate*1e3)
		d := math.Inf(-1)
		for k := e[1]; k >= e[0]; k-- {
			d = max(d, lp[k])
			rev = max(rev, d-lp[k])
		}
	}
	out["tp_ms"], out["td_ms"], out["slope_reversal_db"] = tp, td, rev
	ph := make([]float64, len(xs)-1)
	for i := range ph {
		ph[i] = cmplx.Phase(xs[i+1]*cmplx.Conj(xs[i])) * rate / (2 * math.Pi)
	}
	k := max(int(rate/48000), 1)
	if k > 1 {
		sm := make([]float64, len(ph))
		for i := range sm {
			var s float64
			for m := range k {
				if j := i + (k-1)/2 - m; j >= 0 && j < len(ph) {
					s += ph[j]
				}
			}
			sm[i] = s / float64(k)
		}
		ph = sm
	}
	var steady []float64
	for i := range ph {
		if p[i+1] >= 0.5 {
			steady = append(steady, ph[i])
		}
	}
	slices.Sort(steady)
	fc := steady[len(steady)/2]
	if len(steady)%2 == 0 {
		fc = (steady[len(steady)/2-1] + steady[len(steady)/2]) / 2
	}
	var df float64
	for i := range ph {
		if p[i+1] >= 1e-3 && p[i] >= 1e-3 {
			df = max(df, math.Abs(ph[i]-fc))
		}
	}
	out["df_max_hz"] = df
	out["dfc_hz"] = 6250
	if modulated {
		out["dfc_hz"] = 12500
	}
}

type limMeasures struct {
	cont, wide, ms, msz map[string]float64
	devPct              float64
	hasDev              bool
}

func usReq(pw float64) float64 {
	if pw > 25 {
		return 60
	}
	return min(max(40, dbOf(pw/25e-6)), dbOf(pw/10e-6), 60)
}

// ETSI EN 300 113 clauses 7.4, 7.5 and 7.9; FCC 90.210(d); FCC 97.307(e); ERC/REC 74-01 Annex 6; ETSI TS 102 361-1 clause 10.2.2.3
func limMargins(m limMeasures) map[string]float64 {
	g := map[string]float64{}
	put := func(k string, v float64, ok bool) {
		if ok {
			g[k] = v
		}
	}
	s := m.cont
	get := func(mp map[string]float64, k string) (float64, bool) {
		v, ok := mp[k]
		return v, ok
	}
	for _, pw := range limPowers {
		v, ok := get(s, powKey("maskd_", min(pw, 100), "w"))
		if pw > 100 {
			v, ok = get(s, "maskd_100w")
		}
		put(powKey("fcc_maskd_", pw, "w_db"), v, ok)
	}
	for _, nm := range []string{"acp", "alt"} {
		for _, k := range []string{"nominal", "wide"} {
			v, ok := get(s, nm+"_annexc_"+k+"_dbc")
			put("en_"+nm+"_annexc_"+k+"_db", -60-v, ok)
		}
		v, ok := get(s, nm+"_bins_dbc")
		put("en_"+nm+"_bins_db", -60-v, ok)
	}
	if v, ok := get(s, "alt_bins_dbc"); ok {
		g["cn_alt_db"] = -70 - v
	}
	if v, ok := get(s, "jp_acp_dbc"); ok {
		g["jp_acp_db"] = -55 - v
	}
	for _, pw := range limPowers {
		if v, ok := get(s, "spur_us97_dbc"); ok {
			g[powKey("am_us97e_", pw, "w_db")] = -v - usReq(pw)
		}
		if v, ok := get(s, "spur_erc_dbc"); ok {
			g[powKey("am_erc_", pw, "w_db")] = -v - min(43+dbOf(pw), 70)
		}
	}
	if v, ok := get(s, "obw_hz"); ok {
		g["obw_cn_kr_khz"] = 8.5 - v/1000
		g["obw_jp_khz"] = 11.5 - v/1000
	}
	for _, t := range []struct {
		tag string
		mp  map[string]float64
	}{{"mod", m.ms}, {"unmod", m.msz}} {
		if t.mp == nil {
			continue
		}
		for _, l := range []struct {
			nm  string
			lim float64
		}{{"tacp", -50}, {"talt", -60}} {
			a, okA := get(t.mp, l.nm+"_annexc_nominal_dbc")
			b, okB := get(t.mp, l.nm+"_annexc_wide_dbc")
			put("en_"+l.nm+"_annexc_"+t.tag+"_db", l.lim-max(a, b), okA && okB)
			v, ok := get(t.mp, l.nm+"_sa_dbc")
			put("en_"+l.nm+"_sa_"+t.tag+"_db", l.lim-v, ok)
		}
		if tp, ok := get(t.mp, "tp_ms"); ok {
			g["en_tp_"+t.tag+"_ms"] = tp - 0.1
			g["en_td_"+t.tag+"_ms"] = t.mp["td_ms"] - 0.1
			g["en_slope_"+t.tag+"_db"] = -t.mp["slope_reversal_db"]
			g["en_df_"+t.tag+"_khz"] = (t.mp["dfc_hz"] - t.mp["df_max_hz"]) / 1000
		}
	}
	w := m.wide
	for _, pw := range limPowers {
		if v, ok := get(w, "far_fcc_dbc"); ok {
			g[powKey("fcc_maskd_far_", pw, "w_db")] = s["maskd_ref_db"] - v - min(50+dbOf(pw), 70)
		}
		if v, ok := get(w, "far_erc_dbc"); ok {
			low(g, powKey("am_erc_", pw, "w_db"), -v-min(43+dbOf(pw), 70))
			g[powKey("en_spur_", pw, "w_db")] = -v - (dbOf(pw/1e-3) + 36)
		}
		if v, ok := get(w, "far_us97_dbc"); ok {
			low(g, powKey("am_us97e_", pw, "w_db"), -v-usReq(pw))
		}
	}
	if m.hasDev {
		g["ts_dev_pct"] = 10 - math.Abs(m.devPct)
	}
	return g
}

func low(g map[string]float64, k string, v float64) {
	if old, ok := g[k]; ok {
		v = min(old, v)
	}
	g[k] = v
}

var limPowers = []float64{1, 10, 100, 1000}

func limNeed(key string) float64 {
	switch {
	case hasSuffix(key, "_db") && hasPrefix(key, "en_slope_"):
		return -0.01
	case hasSuffix(key, "_db"):
		return 3
	}
	return 0
}

func hasSuffix(s, x string) bool { return len(s) >= len(x) && s[len(s)-len(x):] == x }
func hasPrefix(s, x string) bool { return len(s) >= len(x) && s[:len(x)] == x }
