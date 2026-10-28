package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

func sincResample(a []int16, from, to int) []int16 {
	const half = 64
	fc := 0.45 * float64(min(from, to)) / float64(from)
	n := int(int64(len(a)) * int64(to) / int64(from))
	out := make([]int16, n)
	for k := range out {
		t := float64(k) * float64(from) / float64(to)
		c := int(math.Floor(t))
		var s float64
		for j := c - half + 1; j <= c+half; j++ {
			if j < 0 || j >= len(a) {
				continue
			}
			u := t - float64(j)
			v := 2 * fc
			if u != 0 {
				v = math.Sin(2*math.Pi*fc*u) / (math.Pi * u)
			}
			r := u / half
			v *= resample.Bessel0(10*math.Sqrt(max(0, 1-r*r))) / resample.Bessel0(10)
			s += float64(a[j]) * v
		}
		out[k] = int16(max(min(math.Round(s), 32767), -32768))
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
	for _, cn := range []float64{4, 6, 8, 10} {
		for _, tier := range tiers {
			var n48, n44, total int
			for seed := range uint64(testmode.N(3, 6, 1)) {
				a := discAudio(noisyBaseband(slots, 1400, cn, 100+seed), 2*discRate)
				for _, c := range []struct {
					hz int
					n  *int
				}{{2 * discRate, &n48}, {44100, &n44}} {
					in := a
					if c.hz != 2*discRate {
						in = sincResample(a, 2*discRate, c.hz)
					}
					sc := scoreBench(truth, receiveAudioAt(t, in, c.hz, tier))
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
