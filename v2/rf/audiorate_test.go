package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

type sincKernel struct {
	from, to, half int
	fc, norm       float64
	g, phases      int64
	tab            []float64
	have           []bool
	buf            []float64
}

func newSincKernel(from, to, half int, fc float64) *sincKernel {
	g := int64(from)
	for b := int64(to); b != 0; {
		g, b = b, g%b
	}
	k := &sincKernel{from: from, to: to, half: half, fc: fc, norm: resample.Bessel0(10), g: g, phases: int64(to) / g,
		buf: make([]float64, 2*half)}
	if k.phases*int64(2*half) <= 1<<20 {
		k.tab, k.have = make([]float64, k.phases*int64(2*half)), make([]bool, k.phases)
	}
	return k
}

func (k *sincKernel) fill(taps []float64, frac float64) {
	for i := range taps {
		u := frac + float64(k.half-1-i)
		v := 2 * k.fc
		if u != 0 {
			v = math.Sin(2*math.Pi*k.fc*u) / (math.Pi * u)
		}
		r := u / float64(k.half)
		taps[i] = v * resample.Bessel0(10*math.Sqrt(max(0, 1-r*r))) / k.norm
	}
}

func (k *sincKernel) at(n int) (int, []float64) {
	num := int64(n) * int64(k.from)
	c, p := num/int64(k.to), num%int64(k.to)
	frac := float64(p) / float64(k.to)
	if k.tab == nil {
		k.fill(k.buf, frac)
		return int(c), k.buf
	}
	q := p / k.g
	taps := k.tab[q*int64(2*k.half) : (q+1)*int64(2*k.half)]
	if !k.have[q] {
		k.fill(taps, frac)
		k.have[q] = true
	}
	return int(c), taps
}

func sincResample(a []int16, from, to int) []int16 {
	const half = 64
	k := newSincKernel(from, to, half, 0.45*float64(min(from, to))/float64(from))
	out := make([]int16, int(int64(len(a))*int64(to)/int64(from)))
	for n := range out {
		c, taps := k.at(n)
		var s float64
		for i, v := range taps {
			if j := c - half + 1 + i; j >= 0 && j < len(a) {
				s += float64(a[j]) * v
			}
		}
		out[n] = int16(max(min(math.Round(s), 32767), -32768))
	}
	return out
}

func TestAudioRates(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	a := discAudio(noisyBaseband(slots, 1400, 30, 4), 2*discRate)
	rng := rand.New(rand.NewPCG(4, 4))
	rates := make([]int, 0, 16)
	rates = append(rates, 8000, 9601, 11025, 12001, 16000, 22050, 24001, 32000, 44100, 47999, 48001, 96000, 192000)
	for range 6 {
		rates = append(rates, audioMinRate+rng.IntN(2*discRate-audioMinRate))
	}
	for _, rate := range rates {
		in := sincResample(a, 2*discRate, rate)
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			cfg := AudioRxConfig{Rate: rate, Tier: tier}
			r, err := NewAudioReceiver(&cfg)
			if tier == Tier0 && rate > 2*discRate {
				if err == nil {
					t.Errorf("Tier0 took %d Hz", rate)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%d Hz, tier %d: %v", rate, tier-Tier0, err)
			}
			var got []Burst
			for k := 0; k < len(in); k += 1000 {
				got = r.AppendBursts(got, in[k:min(k+1000, len(in))])
			}
			got = r.AppendFlush(got)
			if exact, total := exactFrames(slots, got); exact < total || total == 0 {
				t.Errorf("%d Hz, tier %d: %d of %d voice frames exact", rate, tier-Tier0, exact, total)
			}
		}
	}
	var r AudioReceiver
	for _, rate := range []int{-1, 7999, 48001, 1 << 30} {
		if err := r.Init(AudioRxConfig{Rate: rate}); err == nil {
			t.Errorf("Init took %d Hz", rate)
		}
	}
	if _, err := NewAudioReceiver(&AudioRxConfig{Rate: audioMaxRate + 1}); err == nil {
		t.Error("NewAudioReceiver took a rate above 1 MHz")
	}
}

func TestAudio44kMatches48k(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	tiers := []Tier{Tier0, Tier1, Tier2}
	if !testing.Short() && !testmode.Race {
		tiers = append(tiers, Tier3)
	}
	const tol = 0.03
	seeds := testmode.N(3, 6, 1)
	for _, cn := range []float64{4, 6, 8, 10} {
		in := make([][2][]int16, seeds)
		for seed := range in {
			a := discAudio(noisyBaseband(slots, 1400, cn, 100+uint64(seed)), 2*discRate)
			in[seed] = [2][]int16{a, sincResample(a, 2*discRate, 44100)}
		}
		for _, tier := range tiers {
			var n48, n44, total int
			for seed := range in {
				for i, c := range []struct {
					hz int
					n  *int
				}{{2 * discRate, &n48}, {44100, &n44}} {
					sc := scoreBench(truth, receiveAudioAt(t, in[seed][i], c.hz, tier))
					for k, s := range sc {
						if k == "ambe" || k == "rate12" || k == "rate34" || k == "csbk" {
							*c.n += s.Exact
							if c.hz == 2*discRate {
								total += s.Truth
							}
						}
					}
				}
			}
			r48, r44 := float64(n48)/float64(total), float64(n44)/float64(total)
			t.Logf("tier %d at C/N %v dB: %.3f recovered at 48 kHz, %.3f at 44.1 kHz", tier-Tier0, cn, r48, r44)
			if total == 0 || math.Abs(r48-r44) > tol {
				t.Errorf("tier %d at C/N %v dB: %.3f recovered at 48 kHz, %.3f at 44.1 kHz, more than %v apart", tier-Tier0, cn, r48, r44, tol)
			}
		}
	}
}
