package rf

import "unsafe"

type burstSink struct {
	slcHook
	out []Burst
	on  func([]Burst)
}

func (s *burstSink) put(b *Burst) {
	if s == nil {
		return
	}
	s.seeShortLC(b)
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
