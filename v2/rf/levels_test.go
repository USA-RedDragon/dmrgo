package rf

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// levelsReceiver is a receiver of the tests of the level control of the
// DiscReceiver: on the complex baseband, or on its discriminator audio at
// hz.
type levelsReceiver struct {
	hz   int
	tier Tier
}

func (lr levelsReceiver) name() string {
	if lr.hz == 0 {
		return fmt.Sprintf("iq%d", lr.tier-Tier0)
	}
	return fmt.Sprintf("audio%d-%d", lr.tier-Tier0, lr.hz)
}

// receive runs the baseband x through lr.
func (lr levelsReceiver) receive(t testing.TB, x []complex64) []Burst {
	t.Helper()
	if lr.hz == 0 {
		return receiveBaseband(x, lr.tier)
	}
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: lr.hz, Tier: lr.tier, OnBurst: func(b *Burst) { out = append(out, *b) }})
	if err != nil {
		t.Fatal(err)
	}
	a := discAudio(x, lr.hz)
	for k := 0; k < len(a); k += 1024 {
		r.Write(a[k:min(k+1024, len(a))])
	}
	r.Flush()
	return out
}

// levelsReceivers lists the receivers that run the DiscReceiver.
func levelsReceivers() []levelsReceiver {
	return []levelsReceiver{
		{9600, Tier0}, {12000, Tier0}, {24000, Tier0}, {9600, Tier1},
		{0, Tier0}, {0, Tier1},
	}
}

// signalBaseband returns the baseband of the deviation and envelope of
// parts, each from its start in seconds, at a carrier offset of 300 Hz and
// C/N cn, lasting until end seconds. A part without fm is an unmodulated
// carrier for len(env) samples.
func signalBaseband(parts []signalPart, end, cn float64, seed uint64) []complex64 {
	x := make([]complex64, int(end*basebandRate))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	var sigma float64
	if !math.IsInf(cn, 1) {
		sigma = math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	}
	f := make([]float64, len(x))
	a := make([]float64, len(x))
	for _, p := range parts {
		s := int(p.start * basebandRate)
		for k, e := range p.env {
			if i := s + k; i < len(x) {
				a[i] = e
				if p.fm != nil {
					f[i] = p.fm[k]
				}
			}
		}
	}
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (f[i] + 300) / basebandRate
		x[i] = complex(float32(a[i]*math.Cos(ph)+rng.NormFloat64()*sigma), float32(a[i]*math.Sin(ph)+rng.NormFloat64()*sigma))
	}
	return x
}

// signalPart is a part of the signal of signalBaseband.
type signalPart struct {
	start   float64
	fm, env []float64
}

// exactFrom returns the payloads of the bursts of got from the stream of
// truth that starts at start seconds, from its slot from on, that decode
// exactly, and their number.
func exactFrom(truth []benchSlot, got []Burst, start float64, from int) (exact, total int) {
	var g []Burst
	shift := start - benchPad + float64(from)*slotSeconds
	for _, b := range got {
		if b.Seconds-shift > benchPad-slotSeconds/2 {
			b.Seconds -= shift
			g = append(g, b)
		}
	}
	sc := scoreBench(truth[from:], g)
	for _, k := range []string{"ambe", "rate12", "rate34", "rate1", "csbk", "header", "lc"} {
		exact += sc[k].Exact
		total += sc[k].Truth
	}
	return exact, total
}

// TestDiscLevelsAfterUnlock receives a BS stream whose repeater keys up
// again 45 ms after it ends, with a new slot timing, so that the receiver
// unlocks while the second stream is on, after lostSlots, and locks to it
// at once. The start of the first stream moves the unlock across a block
// of the level control of the DiscReceiver. The bursts of the second stream
// after the lock must decode: the level control must measure only the
// input that came after the unlock.
func TestDiscLevelsAfterUnlock(t *testing.T) {
	t.Parallel()
	s1, _ := benchStream(t, 2)
	s2, truth := benchStream(t, 4)
	f1, e1 := rftx.ModulateBS(s1)
	f2, e2 := rftx.ModulateBS(s2)
	for _, lr := range levelsReceivers() {
		t.Run(lr.name(), func(t *testing.T) {
			t.Parallel()
			for k := range 40 {
				pad := 0.3 + float64(k)*0.0007
				start := pad + float64(len(f1))/basebandRate + 0.045
				x := signalBaseband([]signalPart{{pad, f1, e1}, {start, f2, e2}}, start+float64(len(f2))/basebandRate+0.3, 30, 1)
				// The receiver unlocks lostSlots after the first stream
				// ends, and locks again within a slot.
				if ex, tot := exactFrom(truth, lr.receive(t, x), start, lostSlots+3); ex < tot-2 {
					t.Errorf("first stream from %.4f s: %d of %d payloads of the second exact", pad, ex, tot)
				}
			}
		})
	}
}

// TestCarrierLeadIn receives a BS stream that follows an unmodulated
// carrier, without a dip of its power, noise-free and at 40 dB. The level
// control of the DiscReceiver scales the noise of the carrier up to
// discInputRMSHz, so that the front end clips the stream until the next
// block of the level control: the receiver must not lock to a SYNC that
// it clipped. The length of the carrier moves the start of the stream
// across a block, at 0.1, 0.5 and 2 s.
func TestCarrierLeadIn(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	fm, env := rftx.ModulateBS(slots)
	// The ramp of the stream holds the carrier.
	for i := 0; env[i] < 1; i++ {
		env[i] = 1
	}
	rxs := append(levelsReceivers(), levelsReceiver{0, Tier2})
	for _, cn := range []float64{math.Inf(1), 40} {
		for _, lr := range rxs {
			t.Run(fmt.Sprintf("%s/%.0f", lr.name(), cn), func(t *testing.T) {
				t.Parallel()
				for _, base := range []float64{0.1, 0.5, 2} {
					for k := range 30 {
						lead := base + float64(k)*0.001
						start := benchPad + lead
						carrier := make([]float64, int(lead*basebandRate))
						for i := range carrier {
							carrier[i] = 1
						}
						x := signalBaseband([]signalPart{{benchPad, nil, carrier}, {start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, 1)
						// The first burst may clip.
						if ex, tot := exactFrom(truth, lr.receive(t, x), start, 1); ex < tot-2 {
							t.Errorf("%.3f s of carrier: %d of %d payloads exact", lead, ex, tot)
						}
					}
				}
			})
		}
	}
}

// TestCoherentStart receives a BS stream after noise with Tier2 at 20 and
// 40 dB, from 60 starts 0.5 ms apart, and checks that the bursts after the
// first decode: the coherent stage must not take the data of the first
// burst for a SYNC, which the strong signal shows that it is not.
func TestCoherentStart(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	fm, env := rftx.ModulateBS(slots)
	for _, cn := range []float64{20, 40} {
		t.Run(fmt.Sprint(cn), func(t *testing.T) {
			t.Parallel()
			for k := range 60 {
				start := benchPad + float64(k)*0.0005
				x := signalBaseband([]signalPart{{start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, uint64(k)+1)
				if ex, tot := exactFrom(truth, levelsReceiver{0, Tier2}.receive(t, x), start, 1); ex < tot-2 {
					t.Errorf("start at %.4f s: %d of %d payloads exact", start, ex, tot)
				}
			}
		})
	}
}

// TestCarrierLeadInMS receives an MS voice call that follows an unmodulated
// carrier, without a dip of its power, as TestCarrierLeadIn does, and checks
// that the parts C0 and C1 of every voice frame decode. The slots before
// the lock that Tier2 decodes again hold the clipped carrier: a SYNC there
// must not anchor the stream, whose bursts would then fall between the
// slots of the call.
func TestCarrierLeadInMS(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	for i := 0; env[i] < 1; i++ {
		env[i] = 1
	}
	want := make([]uint64, 0, 18*cfg.Superframes)
	for k := range 18 * cfg.Superframes {
		want = append(want, rftx.AMBEValue(rftx.TestAMBEFrame(k)))
	}
	for _, cn := range []float64{math.Inf(1), 40} {
		for _, lr := range []levelsReceiver{{24000, Tier0}, {0, Tier2}} {
			t.Run(fmt.Sprintf("%s/%.0f", lr.name(), cn), func(t *testing.T) {
				t.Parallel()
				for _, base := range []float64{0.01, 0.1, 0.5, 2} {
					for k := range 30 {
						lead := base + float64(k)*0.001
						start := benchPad + lead
						carrier := make([]float64, int(lead*basebandRate))
						for i := range carrier {
							carrier[i] = 1
						}
						x := signalBaseband([]signalPart{{benchPad, nil, carrier}, {start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, uint64(k)+1)
						if n := c0c1InOrder(want, voiceFrames(lr.receive(t, x))); n < len(want) {
							t.Errorf("%.3f s of carrier: C0 and C1 of %d of %d voice frames exact", lead, n, len(want))
						}
					}
				}
			})
		}
	}
}
