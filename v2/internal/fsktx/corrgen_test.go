//go:build !tinygo

package fsktx

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
)

func corrPulse() PulseConfig {
	return PulseConfig{Span: 8, PerSymbol: 40, Gain: 1.0004, Shape: tierShape()}
}

const (
	corrFitSyms = 240000
	corrFitOpt  = 30
	corrFitSkip = 200
)

type corrFit struct {
	hz, syms, skip int
	t              []float64
	cnt, sd        []float64
}

func fitCorr(hz int, syms []int) []corrFit { return fitCorrN(hz, syms, corrFitSyms) }

func fitCorrN(hz int, syms []int, nsyms int) []corrFit {
	cfg := AudioConfig{Hz: hz, Tier: Tier1, Exact: true, Pulse: corrPulse()}
	a0, err := NewAudio(cfg)
	if err != nil {
		panic(err)
	}
	cfg.Opt = corrFitOpt
	a1, err := NewAudio(cfg)
	if err != nil {
		panic(err)
	}
	a1.opt = newAudioOptWin(corrFitOpt, hz, 1024, 0)
	spp := hz / SymbolRate
	rng := rand.New(rand.NewPCG(3, 4))
	sy := make([]int8, nsyms)
	y0 := make([]float64, 0, nsyms*spp)
	y1 := make([]float64, 0, nsyms*spp)
	buf := make([]float64, len(a0.buf))
	for i := range sy {
		s := int8(rng.IntN(4)*2 - 3)
		sy[i] = s
		n := a0.k.symbol(buf, s)
		y0 = append(y0, buf[:n]...)
		n = a1.k.symbol(buf, s)
		a1.opt.process(buf[:n])
		y1 = append(y1, buf[:n]...)
	}
	d := a1.opt.delay
	c := make([]float64, len(y0)-d)
	var tot float64
	for i := range c {
		c[i] = y1[i+d] - y0[i]
		if i >= corrFitSkip*spp {
			tot += c[i] * c[i]
		}
	}
	out := make([]corrFit, 0, len(syms))
	for _, C := range syms {
		best := corrFit{hz: hz, syms: C}
		bestEx := 0.0
		for K := 3; K <= 7; K++ {
			idxOf := func(j int) int {
				idx := 0
				for i := j - K - C + 1; i <= j-K; i++ {
					idx = idx<<2 | int(sy[i]+3)/2
				}
				return idx
			}
			sum := make([]float64, spp<<(2*C))
			sq := make([]float64, spp<<(2*C))
			cnt := make([]float64, 1<<(2*C))
			end := len(c)/spp - 1
			for j := corrFitSkip; j < end; j++ {
				idx := idxOf(j)
				cnt[idx]++
				for p := range spp {
					sum[idx*spp+p] += c[j*spp+p]
					sq[idx*spp+p] += c[j*spp+p] * c[j*spp+p]
				}
			}
			ec := make([]float64, len(sum))
			for i := range sum {
				ec[i] = cnt[i/spp]
				if n := ec[i]; n > 0 {
					sum[i] /= n
					sq[i] = math.Sqrt(max(0, sq[i]/n-sum[i]*sum[i]))
				}
			}
			var res float64
			for j := corrFitSkip; j < end; j++ {
				idx := idxOf(j)
				for p := range spp {
					e := c[j*spp+p] - sum[idx*spp+p]
					res += e * e
				}
			}
			if ex := 1 - res/tot; ex > bestEx {
				bestEx, best.skip, best.t, best.cnt, best.sd = ex, K, sum, ec, sq
			}
		}
		out = append(out, best)
	}
	return out
}

var (
	corrFitRates    = []int{24000, 48000}
	corrFitContexts = []int{5, 6}
)

func committedCorr(hz int, syms []int) []corrFit {
	out := make([]corrFit, 0, len(syms))
	for _, c := range syms {
		tab, skip := corrTable(hz, c)
		t := make([]float64, len(tab))
		for i, v := range tab {
			t[i] = float64(v) / corrUnits
		}
		out = append(out, corrFit{hz: hz, syms: c, skip: skip, t: t})
	}
	return out
}

func renderAudioCorr(b *bytes.Buffer, fit bool) {
	fits := make([][]corrFit, len(corrFitRates))
	var wg sync.WaitGroup
	for i, hz := range corrFitRates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if fit {
				fits[i] = fitCorr(hz, corrFitContexts)
			} else {
				fits[i] = committedCorr(hz, corrFitContexts)
			}
		}()
	}
	wg.Wait()
	for _, fs := range fits {
		for _, f := range fs {
			name := fmt.Sprintf("corr%dk%d", f.hz/1000, f.syms)
			fmt.Fprintf(b, "const %sSkip = %d\n\n", name, f.skip)
			fmt.Fprintf(b, "var %s = [%d]int16{", name, len(f.t))
			for i, v := range f.t {
				if i%20 == 0 {
					b.WriteString("\n")
				}
				fmt.Fprintf(b, "%d,", int16(max(-32767, min(32767, math.Round(v*corrUnits)))))
			}
			b.WriteString("\n}\n\n")
		}
	}
}

func TestCorrFitShort(t *testing.T) {
	t.Parallel()
	tabs := map[string][]int16{"corr24k5": corr24k5[:], "corr24k6": corr24k6[:], "corr48k5": corr48k5[:], "corr48k6": corr48k6[:]}
	skips := map[string]int{"corr24k5": corr24k5Skip, "corr24k6": corr24k6Skip, "corr48k5": corr48k5Skip, "corr48k6": corr48k6Skip}
	for _, hz := range []int{24000, 48000} {
		for _, f := range fitCorrN(hz, []int{5, 6}, 6000) {
			name := fmt.Sprintf("corr%dk%d", hz/1000, f.syms)
			tab := tabs[name]
			if len(f.t) != len(tab) {
				t.Fatalf("%s: a fit of %d values, the table has %d", name, len(f.t), len(tab))
			}
			var sxy, sxx, syy float64
			for i, v := range f.t {
				x, y := v*corrUnits, float64(tab[i])
				sxy, sxx, syy = sxy+x*y, sxx+x*x, syy+y*y
			}
			if spp := hz / SymbolRate; len(tab) != spp<<(2*f.syms) || skips[name] < 3 || skips[name] > 7 {
				t.Errorf("%s: %d values and skip %d, want %d values and skip 3 to 7", name, len(tab), skips[name], spp<<(2*f.syms))
			}
			if lo, hi := slices.Min(tab), slices.Max(tab); lo < -32767 || int(hi)-int(lo) < corrUnits {
				t.Errorf("%s: values %d to %d", name, lo, hi)
			}
			var vs, ns float64
			for i := range f.t {
				if f.cnt[i] >= 2 {
					vs += f.sd[i] * f.sd[i] * f.cnt[i]
					ns += f.cnt[i]
				}
			}
			sd := math.Sqrt(vs / ns)
			for i, v := range f.t {
				if f.cnt[i] < 4 {
					continue
				}
				if z := math.Abs(float64(tab[i])/corrUnits-v) / (sd / math.Sqrt(f.cnt[i])); z > 16 {
					t.Errorf("%s: value %d is %d, %.1f standard errors from %.0f, the mean of %v symbols of a fit on 6000", name, i, tab[i], z, v*corrUnits, f.cnt[i])
				}
			}
			if r := sxy / math.Sqrt(sxx*syy); f.skip != skips[name] || r < 0.7 {
				t.Errorf("%s: a fit on 6000 symbols has skip %d and correlation %.3f with the table, want skip %d and 0.7", name, f.skip, r, skips[name])
			}
		}
	}
}
