//go:build !tinygo

package fsktx

import (
	"encoding/json"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"os"
	"testing"
)

func filter48k(x []float64, h func(f float64) complex128) []float64 {
	n := 1
	for n < len(x) {
		n <<= 1
	}
	c := make([]complex128, n)
	for i, v := range x {
		c[i] = complex(v, 0)
	}
	fft(c)
	for i := range c {
		k := i
		if k >= n/2 {
			k -= n
		}
		c[i] *= h(float64(k) * 48000 / float64(n))
	}
	ifft(c)
	out := make([]float64, len(x))
	for i := range out {
		out[i] = real(c[i])
	}
	return out
}

// TestCorrPEFit fits the correction table of MCUAudioPE: the
// table, odd in the symbols, whose output through the pre-emphasis of the
// radio comes nearest in least squares, weighted up to 13 times over 3 kHz,
// to the output of Table48k with its correction table through a flat radio.
func TestCorrPEFit(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRGO_CORRPE_FIT") != "1" {
		t.Skip("set DMRGO_CORRPE_FIT=1 to refit " + mcuCorrPEFile)
	}
	const nsym, skip0, spp, ctx, iters, hfWeight = 160000, 200, 10, 5, 60, 12
	const lag = (peSpan - 6) * spp
	rng := rand.New(rand.NewPCG(3, 4))
	sy := make([]int8, nsym)
	var mf MCUAudio
	var mc MCUAudioPE
	if err := mf.Init(Table48k(), 15000, 0); err != nil {
		t.Fatal(err)
	}
	if err := mc.Init(15000, 0); err != nil {
		t.Fatal(err)
	}
	xf := make([]float64, 0, nsym*spp)
	yc := make([]float64, 0, nsym*spp)
	for i := range sy {
		s := int8(rng.IntN(4)*2 - 3)
		sy[i] = s
		n := mf.k.symbol(s)
		var add *[10]int16
		if j, ok := mf.corrIdx(s); ok {
			add = &mcuCorr[j]
		}
		for p := range n {
			v := float64(mf.k.buf[p])
			if add != nil {
				v += float64(add[p])
			}
			xf = append(xf, v)
		}
		mc.pos--
		if mc.pos < 0 {
			mc.pos = maxTaps - 1
		}
		d := mc.ring[mc.pos]
		mc.ring[mc.pos], mc.ring[mc.pos+maxTaps] = s, s
		mc.e, _ = shape24kPre(&mc.buf, (*[maxTaps]int8)(mc.ring[mc.pos:mc.pos+maxTaps]), d, mc.e)
		mc.interp(0, false)
		for p := range spp {
			yc = append(yc, float64(mc.buf[p]))
		}
	}
	pe := func(f float64) complex128 { return (1 + complex(0, f/300)) / (1 + complex(0, f/3000)) }
	wt := func(f float64) float64 { return 1 + hfWeight*f*f/(f*f+3000*3000) }
	pey := filter48k(yc, pe)
	target := make([]float64, len(yc))
	for m := skip0 * spp; m < len(yc)-skip0*spp; m++ {
		target[m] = xf[m-lag] - pey[m]
	}
	fit := func(lagSyms int) ([]float64, float64) {
		rows := 1 << (2 * ctx)
		idx := make([]int, nsym)
		for j := skip0; j < nsym-skip0; j++ {
			v := 0
			for i := j - lagSyms - ctx + 1; i <= j-lagSyms; i++ {
				v = v<<2 | int(sy[i]+3)/2
			}
			idx[j] = v
		}
		scatter := func(c []float64) []float64 {
			s := make([]float64, len(yc))
			for j := skip0; j < nsym-skip0; j++ {
				copy(s[j*spp:j*spp+spp], c[idx[j]*spp:idx[j]*spp+spp])
			}
			return s
		}
		gather := func(s []float64) []float64 {
			c := make([]float64, rows*spp)
			for j := skip0; j < nsym-skip0; j++ {
				for p := range spp {
					c[idx[j]*spp+p] += s[j*spp+p]
				}
			}
			for i := range rows {
				for p := range spp {
					a, b := c[i*spp+p], c[(i^(rows-1))*spp+p]
					c[i*spp+p], c[(i^(rows-1))*spp+p] = (a-b)/2, (b-a)/2
				}
			}
			return c
		}
		normal := func(f float64) complex128 {
			g := cmplx.Abs(pe(f))
			return complex(g*g*wt(f), 0)
		}
		adjoint := func(f float64) complex128 { return cmplx.Conj(pe(f)) * complex(wt(f), 0) }
		dot := func(a, b []float64) float64 {
			var s float64
			for i := range a {
				s += a[i] * b[i]
			}
			return s
		}
		b := gather(filter48k(target, adjoint))
		x := make([]float64, len(b))
		r := append([]float64(nil), b...)
		p := append([]float64(nil), r...)
		rr := dot(r, r)
		for range iters {
			ap := gather(filter48k(scatter(p), normal))
			a := rr / dot(p, ap)
			for i := range x {
				x[i] += a * p[i]
				r[i] -= a * ap[i]
			}
			nr := dot(r, r)
			for i := range p {
				p[i] = r[i] + nr/rr*p[i]
			}
			rr = nr
		}
		e := filter48k(scatter(x), pe)
		var tot, res float64
		for m := skip0 * spp; m < len(yc)-skip0*spp; m++ {
			tot += target[m] * target[m]
			d := target[m] - e[m]
			res += d * d
		}
		return x, 1 - res/tot
	}
	bestK, bestEx := 0, math.Inf(-1)
	var x []float64
	for k := 4; k <= 8; k++ {
		if xk, ex := fit(k); ex > bestEx {
			bestK, bestEx, x = k, ex, xk
		}
	}
	t.Logf("lag %d symbols: the table explains %.4f of the output error", bestK, bestEx)
	hz := make([]float64, len(x))
	for i, v := range x {
		hz[i] = math.Round(v*1944/(tableUnits*(1<<tapShift))*100) / 100
	}
	raw, err := json.Marshal(struct {
		C, K, Base int
		Hz         []float64
	}{ctx, bestK + 2, 4, hz})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcuCorrPEFile, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMCUAudioPELag(t *testing.T) {
	t.Parallel()
	syms := goldenSymbols(3000)
	var mf MCUAudio
	var mp MCUAudioPE
	if err := mf.Init(Table48k(), 15000, 0); err != nil {
		t.Fatal(err)
	}
	if err := mp.Init(15000, 0); err != nil {
		t.Fatal(err)
	}
	var bf, bp [maxSamples]int16
	var xf, xp []float64
	for _, s := range syms {
		for _, v := range bf[:mf.Symbol(bf[:], s)] {
			xf = append(xf, float64(v))
		}
		for _, v := range bp[:mp.Symbol(bp[:], s)] {
			xp = append(xp, float64(v))
		}
	}
	pe := func(f float64) complex128 { return (1 + complex(0, f/300)) / (1 + complex(0, f/3000)) }
	y := filter48k(xp, pe)
	best, bestC := 0, math.Inf(-1)
	for l := 0; l <= 60; l++ {
		var c float64
		for i := 1000; i < len(xf)-1000; i++ {
			c += xf[i] * y[i+l]
		}
		if c > bestC {
			best, bestC = l, c
		}
	}
	if want := (mp.Span() - Table48k().Span()) * mp.Samples(); best != want {
		t.Errorf("the output through the pre-emphasis lags that of Table48k by %d samples, want %d from Span %d", best, want, mp.Span())
	}
}
