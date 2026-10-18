package rf

import (
	"math"
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

// impairedFM returns the deviation freq scaled by 1+dev and the envelope
// env as baseband at offsetHz, faded by Rayleigh fading of Doppler fd Hz
// when fd is above 0, with noise at a C/N of cn dB in 12.5 kHz of the mean
// power, and benchPad of noise before and after, as noisyFM does.
func impairedFM(freq, env []float64, offsetHz, dev, fd, cn float64, seed uint64) []complex64 {
	pad := int(benchPad * basebandRate)
	x := make([]complex64, len(freq)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	// A sum of rays from angles spread around the receiver, each of the
	// same power, at a random phase.
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
			ph += 2 * math.Pi * (freq[k]*(1+dev) + offsetHz) / basebandRate
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

// TestTierImpaired checks that the coherent tiers on I/Q recover at least
// what Tier0 does, for each payload, summed over seeds, on direct mode
// traffic whose transmissions start between symbol boundaries, from a
// transmitter whose deviation is 10% off either way, and through Rayleigh
// fading of 50 Hz Doppler, at C/N points from the 90% point of Tier0 for
// exact voice frames up. The coherent stage then has to fit the
// modulation index of the stream, and to leave the uncoded bits to the
// core where the FEC shows that it makes fewer errors. Tiers decide some
// bursts differently by chance, so a tier may fall short by 1% of the
// truth, or 2 items, as in TestTierOrder.
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
	const seeds = 8
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
				x := impairedFM(fm, env, 1400, sc.dev, sc.fd, cn, 100+seed)
				for _, tier := range tiers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						got := scoreMS(truth, receiveBaseband(x, tier))
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
				if hi.Exact >= lo.Exact-max(2, lo.Truth/100) {
					continue
				}
				t.Errorf("%s iq %s: Tier%d recovers %d of %d, Tier0 %d", sc.name, p, tier-Tier0, hi.Exact, hi.Truth, lo.Exact)
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
