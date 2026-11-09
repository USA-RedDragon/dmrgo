package rf

import (
	"math"
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

func impairedFM(t testing.TB, freq, env []float64, dev, fd, cn float64, seed uint64) []complex64 {
	seed = noiseSeed(t, seed)
	pad := int(benchPad * basebandRate)
	x := make([]complex64, len(freq)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	const rays = 32
	var dop, phase [rays]float64
	for i := range rays {
		dop[i] = 2 * math.Pi * fd * math.Cos(2*math.Pi*(float64(i)+rng.Float64())/rays) / basebandRate
		phase[i] = 2 * math.Pi * rng.Float64()
	}
	var ph float64
	for i := range x {
		var s complex128
		if k := i - pad; k >= 0 && k < len(freq) {
			ph += 2 * math.Pi * (freq[k]*(1+dev) + 1400) / basebandRate
			s = complex(env[k]*math.Cos(ph), env[k]*math.Sin(ph))
			if fd > 0 {
				var g complex128
				for r := range rays {
					sn, cs := math.Sincos(phase[r] + dop[r]*float64(k))
					g += complex(cs, sn)
				}
				s *= g / complex(math.Sqrt(rays), 0)
			}
		}
		x[i] = complex(float32(real(s)+rng.NormFloat64()*sigma), float32(imag(s)+rng.NormFloat64()*sigma))
	}
	return x
}

const tierImpairedDraws = 509

var tierImpairedSD = map[string]float64{
	"dev+10/1/ambe":      8.53,
	"dev+10/1/ambe_c0c1": 0.23,
	"dev+10/1/csbk":      1.04,
	"dev+10/1/elc":       1.60,
	"dev+10/1/lc":        0.09,
	"dev+10/1/rate12":    0.38,
	"dev+10/1/rate34":    0.86,
	"dev+10/2/ambe":      7.60,
	"dev+10/2/ambe_c0c1": 0.18,
	"dev+10/2/csbk":      1.01,
	"dev+10/2/elc":       1.60,
	"dev+10/2/lc":        0.09,
	"dev+10/2/rate12":    0.33,
	"dev+10/2/rate34":    0.81,
	"dev-10/1/ambe":      14.69,
	"dev-10/1/ambe_c0c1": 0.42,
	"dev-10/1/csbk":      0.44,
	"dev-10/1/elc":       3.24,
	"dev-10/1/lc":        0.04,
	"dev-10/1/rate12":    0.23,
	"dev-10/1/rate34":    1.74,
	"dev-10/2/ambe":      13.86,
	"dev-10/2/ambe_c0c1": 0.41,
	"dev-10/2/csbk":      0.45,
	"dev-10/2/elc":       3.24,
	"dev-10/2/lc":        0.04,
	"dev-10/2/rate12":    0.25,
	"dev-10/2/rate34":    1.79,
	"fade50/1/ambe_c0c1": 3.33,
	"fade50/1/csbk":      2.69,
	"fade50/1/elc":       3.93,
	"fade50/1/lc":        0.75,
	"fade50/1/rate12":    1.41,
	"fade50/1/rate34":    2.81,
	"fade50/2/ambe_c0c1": 3.64,
	"fade50/2/csbk":      2.88,
	"fade50/2/elc":       4.46,
	"fade50/2/lc":        0.80,
	"fade50/2/rate12":    1.63,
	"fade50/2/rate34":    3.83,
}

// TestTierImpaired checks that Tiers 1 and 2 recover no fewer bursts than
// Tier 0 on impaired MS streams. A tier fails a run only when it falls more
// than max(2, 1%) below Tier 0 and also more than BarZ standard deviations
// below 0, the requirement, with the standard deviation of the difference
// over tierImpairedDraws run seeds. The cells where the mean difference is
// significantly below 0 are not in tierImpairedSD and keep the first bar.
func TestTierImpaired(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("receives 56 streams with 3 receivers")
	}
	type scenario struct {
		name    string
		dev, fd float64
		cns     []float64
	}
	scenarios := []scenario{
		{"dev-10", -0.1, 0, []float64{8, 12}},
		{"dev+10", 0.1, 0, []float64{8, 12}},
		{"fade50", 0, 50, []float64{18, 24, 30}},
	}
	// Under -race 2 seeds run the code paths; the bars hold for 8 seeds only.
	seeds := testmode.N(8, 8, 2)
	fm, env, truth := msStream(t, 3, 0.37)
	tiers := []Tier{Tier0, Tier1, Tier2}
	type key struct {
		scenario string
		tier     Tier
		payload  string
	}
	sum := map[key]*benchScore{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, sc := range scenarios {
		for _, cn := range sc.cns {
			for seed := range uint64(seeds) {
				x := sync.OnceValue(func() []complex64 { return impairedFM(t, fm, env, sc.dev, sc.fd, cn, 100+seed) })
				for _, tier := range tiers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						got := scoreMS(truth, receiveBaseband(x(), tier))
						mu.Lock()
						defer mu.Unlock()
						for _, p := range tierOrderPayloads() {
							k := key{sc.name, tier, p}
							if sum[k] == nil {
								sum[k] = &benchScore{}
							}
							sum[k].Truth += got[p].Truth
							sum[k].Exact += got[p].Exact
						}
					}()
				}
			}
		}
	}
	wg.Wait()
	for _, sc := range scenarios {
		for _, p := range tierOrderPayloads() {
			lo := sum[key{sc.name, Tier0, p}]
			if lo.Truth == 0 {
				t.Fatalf("%s: no %s in the stream", sc.name, p)
			}
			for _, tier := range tiers[1:] {
				hi := sum[key{sc.name, tier, p}]
				d := hi.Exact - lo.Exact
				sd, measured := tierImpairedSD[sc.name+"/"+strconv.Itoa(int(tier-Tier0))+"/"+p]
				if testmode.Race || d >= -max(2, lo.Truth/100) || measured && float64(d) >= -testseed.BarZ*max(sd, 1) {
					continue
				}
				t.Errorf("%s iq %s: Tier%d recovers %d of %d, Tier0 %d; at equal tiers the difference has a standard deviation of %.2f over %d run seeds, so below %.1f has probability below %g", sc.name, p, tier-Tier0, hi.Exact, hi.Truth, lo.Exact, sd, tierImpairedDraws, -testseed.BarZ*max(sd, 1), testseed.BarAlpha)
			}
		}
	}
	if testing.Verbose() {
		for _, sc := range scenarios {
			for _, tier := range tiers {
				line := sc.name + " iq" + strconv.Itoa(int(tier-Tier0))
				for _, p := range tierOrderPayloads() {
					s := sum[key{sc.name, tier, p}]
					line += " " + p + "=" + strconv.Itoa(s.Exact) + "/" + strconv.Itoa(s.Truth)
				}
				t.Log(line)
			}
		}
	}
}
