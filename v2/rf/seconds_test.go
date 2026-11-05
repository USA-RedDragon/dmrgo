//go:build !tinygo

package rf

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
)

const (
	secondsAudioLag = 45.5
	secondsTol      = 0.01
)

func exactBaseband(slots []rftx.BSSlot) benchSignal {
	fm, env := rftx.ModulateBS(slots)
	x := rftx.Baseband(fm, env, 0)
	pad := int(benchPad * basebandRate)
	s := benchSignal{make([]float64, len(x)+2*pad), make([]float64, len(x)+2*pad)}
	for k, v := range x {
		s.re[pad+k], s.im[pad+k] = float64(real(v)), float64(imag(v))
	}
	return s
}

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
	x := exactBaseband(slots).noisy(t, 40, 3)
	a48 := discAudio(x, basebandRate)
	type tcase struct {
		name string
		run  func(t testing.TB) ([]float64, float64)
	}
	median := func(errs []float64) float64 {
		if len(errs) == 0 {
			return math.NaN()
		}
		return errs[len(errs)/2]
	}
	var cases []tcase
	for _, hz := range []int{8000, 9600, 11025, 12000, 16000, 24000, 44100, 48000} {
		var a []int16
		switch hz {
		case 11025:
			a = sincResample(discAudio(x, discRate), discRate, hz)
		case 44100:
			a = sincResample(a48, basebandRate, hz)
		default:
			a = discAudio(x, hz)
		}
		for tier := Tier0; tier <= Tier3; tier++ {
			cases = append(cases, tcase{"audio" + strconv.Itoa(int(tier-Tier0)) + "/" + strconv.Itoa(hz), func(t testing.TB) ([]float64, float64) {
				errs := secondsErrors(receiveAudioAt(t, a, hz, tier), secondsAudioLag/float64(basebandRate))
				return errs, median(errs)
			}})
		}
	}
	for _, hz := range []int{9600, 12000, 24000, 36000, 48000, 240000} {
		for tier := Tier0; tier <= Tier3; tier++ {
			if hz < discRate && tier != Tier0 || hz < basebandRate && tier > Tier1 {
				continue
			}
			cases = append(cases, tcase{"iq" + strconv.Itoa(int(tier-Tier0)) + "/" + strconv.Itoa(hz), func(t testing.TB) ([]float64, float64) {
				if hz == basebandRate {
					errs := secondsErrors(receiveIQAt(t, x, hz, tier), 0)
					return errs, median(errs)
				}
				phases := 1
				if hz == iq0LowHz {
					phases = basebandRate / hz
				}
				var all []float64
				var mid float64
				for k := range phases {
					y := sincResampleIQ(append(make([]complex64, k), x...), hz)
					errs := secondsErrors(receiveIQAt(t, y, hz, tier), -float64(k)/basebandRate)
					all = append(all, errs...)
					mid += median(errs) / float64(phases)
				}
				slices.Sort(all)
				return all, mid
			}})
		}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			errs, mid := c.run(t)
			if len(errs) < 20 {
				t.Fatalf("%d bursts", len(errs))
			}
			lo, hi := errs[len(errs)/10], errs[len(errs)*9/10]
			t.Logf("%-14s %d bursts, error %+.3f symbol, 10%%..90%% %+.3f..%+.3f", c.name, len(errs), mid, lo, hi)
			if math.Abs(mid) > secondsTol {
				t.Errorf("Seconds is %+.3f symbol from the first burst bit", mid)
			}
		})
	}
}

func fracResampleIQ(x []complex64, to int, d float64) []complex64 {
	const half = 32
	k := newSincKernel(basebandRate, to, half, 0.45*float64(to)/basebandRate)
	step := float64(basebandRate) / float64(to)
	out := make([]complex64, int(float64(len(x))/step))
	taps := make([]float64, 2*half)
	for n := range out {
		pos := float64(n)*step - d
		c := math.Floor(pos)
		k.fill(taps, pos-c)
		var re, im float64
		for i, v := range taps {
			if j := int(c) - half + 1 + i; j >= 0 && j < len(x) {
				re += float64(real(x[j])) * v
				im += float64(imag(x[j])) * v
			}
		}
		out[n] = complex(float32(re), float32(im))
	}
	return out
}

func TestSecondsLowRatePhase(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	x := exactBaseband(slots).noisy(t, math.Inf(1), 3)
	const phases = 20
	span := float64(basebandRate) / iq0LowHz
	for k := range phases {
		d := span * float64(k) / phases
		errs := secondsErrors(receiveIQAt(t, fracResampleIQ(x, iq0LowHz, d), iq0LowHz, Tier0), -d/basebandRate)
		if len(errs) < 20 {
			t.Fatalf("shift %.2f sample: %d bursts", d/span, len(errs))
		}
		var m float64
		for _, e := range errs {
			m += e / symbolRate
		}
		if m /= float64(len(errs)); math.Abs(m) > mxSecondsMean {
			t.Errorf("shift %.2f sample: Seconds is %+.2f us off on average", d/span, m*1e6)
		}
	}
}

func TestSecondsLockBurstExact(t *testing.T) {
	t.Parallel()
	secondsLockBurstExact(t, testConfig(t))
}

// TestSecondsLockBurstDeferredFit pins a payload whose lock burst a false
// lock before it made the receiver hunt: the sync fit deferred to the next
// symbol, and the lock took the strobe unfitted, 25 us early.
func TestSecondsLockBurstDeferredFit(t *testing.T) {
	t.Parallel()
	secondsLockBurstExact(t, testscen.Of(1234071648616322122).Apply(testConfigFixed()))
}

func secondsLockBurstExact(t *testing.T, cfg rftx.SeqConfig) {
	t.Helper()
	cfg.Superframes = 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	ramp := (slotSeconds - burstSeconds) / 2
	for _, cn := range []float64{math.Inf(1), 30} {
		for tier := Tier0; tier <= Tier2; tier++ {
			var sum float64
			for k := range 16 {
				start := benchPad + float64(k)*0.00037
				x := signalBaseband([]signalPart{{start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, uint64(k+1))
				got := receiveIQAt(t, x, basebandRate, tier)
				if len(got) == 0 {
					t.Fatalf("%.0f dB Tier%d start %.5f s: no bursts", cn, tier-Tier0, start)
				}
				d := got[0].Seconds - math.Floor(start*basebandRate)/basebandRate - ramp
				sum += d - math.Round(d/(2*slotSeconds))*2*slotSeconds
			}
			if mean := sum / 16; math.Abs(mean) > 0.6e-6 {
				t.Errorf("%.0f dB Tier%d: the lock burst Seconds is %+.2f us off on average", cn, tier-Tier0, mean*1e6)
			}
		}
	}
}

// TestSecondsLockBurstOffset checks that the lock burst Seconds of a BS
// stream, whose sync starts and ends on different symbols, does not move
// with the carrier offset: the mean over the seeds at +-2000 Hz must stay
// within 2 us of the mean at 0 Hz.
func TestSecondsLockBurstOffset(t *testing.T) {
	t.Parallel()
	const seeds = 8
	slots, _ := benchStream(t, 2)
	for tier := Tier0; tier <= Tier2; tier++ {
		var at [3]float64
		for i, off := range []float64{0, 2000, -2000} {
			sig := cleanBaseband(slots, off)
			d := make([]float64, seeds)
			forSeeds(seeds, func(k int) {
				got := receiveBaseband(sig.noisy(t, 30, uint64(k+1)), tier)
				if len(got) == 0 {
					d[k] = math.NaN()
					return
				}
				d[k] = math.Remainder(got[0].Seconds, slotSeconds)
			})
			at[i], _ = meanSE(d)
			if i > 0 && !(math.Abs(at[i]-at[0]) <= 2e-6) {
				t.Errorf("Tier%d %+.0f Hz: the lock burst Seconds moves %+.2f us from 0 Hz", tier-Tier0, off, (at[i]-at[0])*1e6)
			}
		}
	}
}

var (
	switchFirst  = rftx.SeqConfig{ColorCode: 1, Source: 1000001, Dest: 9990, BS: 990911, Superframes: 1}
	switchSecond = rftx.SeqConfig{ColorCode: 1, Source: 1000002, Dest: 9990, BS: 990911, Message: "TALKER TWO DATA"}
)

func TestSecondsTalkerSwitch(t *testing.T) {
	t.Parallel()
	secondsTalkerSwitch(t, randomizedN(t, 1000001, switchFirst), randomizedN(t, 1000002, switchSecond))
}

// TestSecondsTalkerSwitchHuntFit pins payloads whose second talker locked
// while the receiver still held the first: that lock took the peak of the
// sync correlation, which moves with the symbols next to the sync, and
// not the MM zero of a first lock.
func TestSecondsTalkerSwitchHuntFit(t *testing.T) {
	t.Parallel()
	secondsTalkerSwitch(t, testscen.Of(2859943720115404153).Apply(switchFirst), testscen.Of(4478137214299954279).Apply(switchSecond))
}

func secondsTalkerSwitch(t *testing.T, c1, c2 rftx.SeqConfig) {
	t.Helper()
	v, err := rftx.Sequence("voice", c1)
	if err != nil {
		t.Fatal(err)
	}
	d, err := rftx.Sequence("data12", c2)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	vb := v.Bursts[:len(v.Bursts)-1]
	f1, e1 := rftx.Modulate(vb, rftx.Schedule{Slot: 0})
	f2, e2 := rftx.Modulate(d.Bursts, rftx.Schedule{Slot: 0})
	ramp := (slotSeconds - burstSeconds) / 2
	for _, c := range []struct{ gap, frac float64 }{{0.0192, 0.05}, {0.1192, 0.25}} {
		t2 := math.Round((lead+float64(2*len(vb))*slotSeconds+c.gap+c.frac/symbolRate)*basebandRate) / basebandRate
		x := signalBaseband([]signalPart{{lead, f1, e1}, {t2, f2, e2}}, t2+float64(len(f2))/basebandRate+0.1, math.Inf(1), 1)
		for tier := Tier0; tier <= Tier3; tier++ {
			got := receiveIQAt(t, x, basebandRate, tier)
			lo, hi, n := math.Inf(1), math.Inf(-1), 0
			for i := range d.Bursts {
				w := t2 + float64(2*i)*slotSeconds + ramp
				for _, b := range got {
					if e := b.Seconds - w; math.Abs(e) < 50e-6 {
						lo, hi, n = min(lo, e), max(hi, e), n+1
						break
					}
				}
			}
			if n < len(d.Bursts) || hi-lo > mxSecondsMean {
				t.Errorf("gap %.1f ms, %.2f symbol, Tier%d: %d of %d bursts of the second talker, Seconds spread %.1f us", c.gap*1e3, c.frac, tier-Tier0, n, len(d.Bursts), (hi-lo)*1e6)
			}
		}
	}
}

func TestSecondsClockError(t *testing.T) {
	t.Parallel()
	for _, content := range []uint8{0, 1} {
		for _, clock := range []uint8{1, 2} {
			for _, f := range []uint8{1, 5} {
				row := mxRow{mxOffset: 3, mxContent: content, mxClock: clock, mxFormat: f}
				s, err := mxBuild(t, row)
				if err != nil {
					t.Fatal(err)
				}
				in := mxInputOf(s, row)
				for tier := Tier0; tier <= Tier3; tier++ {
					got, _, err := mxReceive(in, mxRx{row.val(mxFormat), tier})
					if err != nil {
						t.Fatal(err)
					}
					var sum float64
					n := 0
					for _, b := range got {
						at := b.Seconds + in.delay
						j := mxNearest(s, at)
						if j < 3 {
							continue
						}
						d := at - s.truth[j].at
						if math.Abs(d) > mxSeconds {
							t.Errorf("%s Tier%d: burst %d is %+.1f us off", row.name(), tier-Tier0, j, d*1e6)
						}
						sum += d
						n++
					}
					if n < len(s.truth)-4 {
						t.Errorf("%s Tier%d: %d of %d bursts after the third", row.name(), tier-Tier0, n, len(s.truth)-3)
					} else if m := sum / float64(n); math.Abs(m) > mxSecondsMean {
						t.Errorf("%s Tier%d: Seconds is %+.1f us off on average", row.name(), tier-Tier0, m*1e6)
					}
				}
			}
		}
	}
}
