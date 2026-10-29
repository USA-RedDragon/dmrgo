//go:build !tinygo

package rf

import (
	"math"
	"slices"
	"strconv"
	"testing"
)

const (
	secondsAudioLag = 46
	secondsTol      = 0.01
)

func sincResampleIQ(x []complex64, to int) []complex64 {
	const half = 32
	k := newSincKernel(basebandRate, to, half, 0.45*float64(min(basebandRate, to))/basebandRate)
	out := make([]complex64, int(int64(len(x))*int64(to)/basebandRate))
	for n := range out {
		c, taps := k.at(n)
		var re, im float64
		for i, v := range taps {
			if j := c - half + 1 + i; j >= 0 && j < len(x) {
				re += float64(real(x[j])) * v
				im += float64(imag(x[j])) * v
			}
		}
		out[n] = complex(float32(re), float32(im))
	}
	return out
}

func receiveIQAt(t testing.TB, x []complex64, hz int, tier Tier) []Burst {
	t.Helper()
	r, err := NewReceiver(&RxConfig{Rate: hz, Tier: tier, Format: IQFormatCF32})
	if err != nil {
		t.Fatal(err)
	}
	var out []Burst
	b := cf32(x)
	for k := 0; k < len(b); k += 8192 {
		out = append(out, must(r.writeIQ(b[k:min(k+8192, len(b))]))...)
	}
	return append(out, r.flushAll()...)
}

func secondsErrors(got []Burst, lag float64) []float64 {
	first := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad - lag
	errs := make([]float64, 0, len(got))
	for _, b := range got {
		i := math.Round((b.Seconds - first) / slotSeconds)
		errs = append(errs, (b.Seconds-first-i*slotSeconds)*symbolRate)
	}
	slices.Sort(errs)
	return errs
}

func TestSecondsFirstBit(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	x := cleanBaseband(slots, 0).noisy(40, 3)
	a48 := discAudio(x, basebandRate)
	type tcase struct {
		name string
		run  func(t testing.TB) ([]Burst, float64)
	}
	var cases []tcase
	for _, hz := range []int{8000, 9600, 11025, 12000, 16000, 24000, 44100, 48000} {
		for tier := Tier0; tier <= Tier3; tier++ {
			var a []int16
			switch hz {
			case 11025:
				a = sincResample(discAudio(x, discRate), discRate, hz)
			case 44100:
				a = sincResample(a48, basebandRate, hz)
			default:
				a = discAudio(x, hz)
			}
			cases = append(cases, tcase{"audio" + strconv.Itoa(int(tier-Tier0)) + "/" + strconv.Itoa(hz), func(t testing.TB) ([]Burst, float64) {
				return receiveAudioAt(t, a, hz, tier), secondsAudioLag / float64(basebandRate)
			}})
		}
	}
	for _, hz := range []int{9600, 12000, 24000, 36000, 48000, 240000} {
		for tier := Tier0; tier <= Tier3; tier++ {
			if hz < discRate && tier != Tier0 || hz < basebandRate && tier > Tier1 {
				continue
			}
			cases = append(cases, tcase{"iq" + strconv.Itoa(int(tier-Tier0)) + "/" + strconv.Itoa(hz), func(t testing.TB) ([]Burst, float64) {
				y := x
				if hz != basebandRate {
					y = sincResampleIQ(x, hz)
				}
				return receiveIQAt(t, y, hz, tier), 0
			}})
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, lag := c.run(t)
			errs := secondsErrors(got, lag)
			if len(errs) < 20 {
				t.Fatalf("%d bursts", len(errs))
			}
			lo, mid, hi := errs[len(errs)/10], errs[len(errs)/2], errs[len(errs)*9/10]
			t.Logf("%-14s %d bursts, error %+.3f symbol, 10%%..90%% %+.3f..%+.3f", c.name, len(errs), mid, lo, hi)
			if math.Abs(mid) > secondsTol {
				t.Errorf("Seconds is %+.3f symbol from the first burst bit", mid)
			}
		})
	}
}
