//go:build !tinygo

package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

const txHostBuilt = true

type txHost struct {
	iq  *fsktx.IQ
	au  *fsktx.Audio
	s16 []int16
}

func (h *txHost) newIQ(t Tier, rate int, offsetHz float64) error {
	q, err := fsktx.NewIQ(fsktx.IQConfig{Rate: rate, OffsetHz: offsetHz, RampSeconds: txRampSec, Tier: fsktx.Tier(t)})
	if err != nil {
		return err
	}
	*h = txHost{iq: q}
	return nil
}

func (h *txHost) newAudio(t Tier, rate, level, offset int) error {
	a, err := fsktx.NewAudio(fsktx.AudioConfig{Hz: rate, Level: level, Offset: offset, Tier: fsktx.Tier(t)})
	if err != nil {
		return err
	}
	*h = txHost{au: a, s16: make([]int16, a.MaxSamples())}
	return nil
}

func (h *txHost) key(on bool) {
	if h.iq != nil {
		h.iq.Key(on)
	}
}

func (h *txHost) iqS16(s int8) []int16 { return h.iq.Symbol(s) }

func (h *txHost) audio(s int8) []int16 { return h.s16[:h.au.Symbol(h.s16, s)] }

type audioPeaks struct{ a *fsktx.Audio }

func newAudioPeaks(t Tier, rate int) (audioPeaks, error) {
	a, err := fsktx.NewAudio(fsktx.AudioConfig{Hz: rate, Level: 1, Tier: fsktx.Tier(t)})
	return audioPeaks{a}, err
}

func (p audioPeaks) peak(level int) (int, error) {
	pk, err := p.a.PeakAt(level)
	if err != nil {
		return 0, err
	}
	return int(math.Ceil(pk)), nil
}

func measuredLevel(t Tier, rate, bits int) int {
	s := fsktx.Setup{Tier: fsktx.Tier(t), Bits: bits}
	level, depth := s.Tier0Level(), s.MeasuredBits()
	if t == Tier0 || depth == 0 || depth == 16 {
		return level
	}
	half := 1<<(depth-1) - 1
	ps, _ := newAudioPeaks(t, rate)
	peak := func(l int) int {
		pk, err := ps.peak(l)
		if err != nil {
			return half + 1
		}
		return pk
	}
	top := level >> (bits - depth)
	l := top
	if pk := peak(top); pk > half {
		l = min(top-1, top*half/pk)
		for l > 0 && peak(l) > half {
			l--
		}
		for l > 0 && l < top-1 && peak(l+1) <= half {
			l++
		}
	}
	return l << (bits - depth)
}

func newChunk(n int) []byte { return make([]byte, n) }

func newChunk16(n int) []int16 { return make([]int16, n) }
