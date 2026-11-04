//go:build !tinygo

package rf

import (
	"math"
	"math/cmplx"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

type toneRing struct {
	ring []complex64
	hz   float64
	rate float64
}

func (r *toneRing) cohRing(_ int64) ([]complex64, int64, int64) {
	return r.ring, int64(len(r.ring)), 0
}

func (r *toneRing) cohPhase(a, b, sign, hz, rate float64, _ bool) float64 {
	return -2 * math.Pi * sign * hz * (b - a) / rate
}

func (r *toneRing) cohKey() (int, float64, float64)                   { return 0, 1, 0 }
func (r *toneRing) cohTrack() (float64, bool)                         { return r.hz, true }
func (r *toneRing) cohWeight(_, _ float64) float32                    { return 1 }
func (r *toneRing) cohScale() float32                                 { return 1 }
func (r *toneRing) cohResidual() bool                                 { return false }
func (r *toneRing) cohHz() float64                                    { return r.hz }
func (r *toneRing) cohTaps(bool) []float32                            { return nil }
func (r *toneRing) cohAcqRing(size int64) ([]complex64, int64, int64) { return r.cohRing(size) }
func (r *toneRing) cohLockHz() (float64, float64)                     { return r.hz, 1 }

func TestCohFollowTonePhase(t *testing.T) {
	t.Parallel()
	opt := tierOptions(Tier3)
	s := newCohStageOf(&opt)
	s.size = 1 << 14
	const hz = -4000
	src := &toneRing{ring: make([]complex64, s.size), hz: hz, rate: s.rate}
	for i := range src.ring {
		src.ring[i] = complex64(cmplx.Exp(complex(0, 2*math.Pi*hz*float64(i)/s.rate)))
	}
	s.src = src
	s.totalHz, s.t0, s.dir = hz, 1000, 1
	for i := range s.follow {
		s.follow[i] = -0.54 * float64(len(s.follow)-1-i) / float64(len(s.follow)-1)
	}
	s.followOK = true
	x := make([]complex64, s.model.sps)
	ref := 0.0
	for k := range slotSymbols {
		s.fillSym(k, x)
		for j, v := range x {
			ph := cmplx.Phase(complex128(v))
			if k == 0 && j == 0 {
				ref = ph
			}
			if d := math.Remainder(ph-ref, 2*math.Pi); math.Abs(d) > 0.05 {
				t.Fatalf("symbol %d sample %d: a -4 kHz tone is %+.3f rad off the burst start after the strobe offsets", k, j, d)
			}
		}
	}
}

func TestCohFollowStrobe(t *testing.T) {
	t.Parallel()
	opt := tierOptions(Tier3)
	s := newCohStageOf(&opt)
	var strobes [extStrobes]int64
	s.strobes = &strobes
	c := &leanCore{}
	c.base, c.ns = 1000, 1000+slotSymbols
	bend := func(j int64) float64 { return 300 * math.Exp(-float64(j-c.base)/60) }
	for j := c.base - cohFollowPre; j < c.ns; j++ {
		strobes[j&(extStrobes-1)] = int64((float64(j)*samplesPerSy + bend(j)) * 256)
	}
	for _, v := range []enums.VoiceBurstType{enums.VoiceBurstB, enums.VoiceBurstUnknown} {
		s.followStrobe(c, &Burst{Voice: v})
		mid := 0.0
		if v == enums.VoiceBurstUnknown {
			mid = bend(c.base+syncFirstSym+syncSymbols/2) - bend(c.ns-1)
		}
		for k := -cohFollowPre; k < slotSymbols; k++ {
			want := (bend(c.base+int64(k)) - bend(c.ns-1) - mid) / s.div
			if got := s.followAt(k); math.Abs(got-want) > 1e-2 {
				t.Fatalf("voice %v symbol %d: follow %.3f ring samples, want %.3f", v, k, got, want)
			}
		}
	}
}
