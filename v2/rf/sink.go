package rf

import (
	"math"
	"unsafe"
)

const payloadEdgeBits = 0xbf1b4e81b4e81b4f

type burstSink struct {
	out []Burst
	on  func([]Burst)
}

func (s *burstSink) put(b *Burst) {
	if s == nil {
		return
	}
	if s.on != nil {
		if cap(s.out) == 0 {
			s.on(unsafe.Slice(b, 1))
			return
		}
		if len(s.out) == cap(s.out) {
			s.deliver()
		}
	}
	if n := len(s.out); n < cap(s.out) {
		s.out = s.out[:n+1]
		s.out[n] = *b
	}
}

// payloadInInput reports whether the first payload symbol of a burst that
// starts at seconds, on the time axis of Burst.Seconds, has its center inside
// the input. Each front gives Seconds net of its own lag, so the rule holds
// for every front. Before that point the input holds no sample of the
// symbol, and the receiver emits such a burst only when a CRC confirms it.
func payloadInInput(seconds float64) bool {
	b := math.Float64bits(seconds)
	return b>>63 == 0 || b <= payloadEdgeBits
}

func (s *burstSink) deliver() {
	if s.on != nil && len(s.out) > 0 {
		s.on(s.out)
		s.out = s.out[:0]
	}
}

func streamBursts(lookback int) int {
	if lookback <= 0 {
		return 0
	}
	return lookback + 4
}

func growBursts(dst []Burst, n int) []Burst {
	for cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], Burst{})[:len(dst)]
	}
	return dst
}
