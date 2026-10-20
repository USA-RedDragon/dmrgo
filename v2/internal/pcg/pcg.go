// Package pcg is the seeded random source of the service machines: a PCG32
// generator (XSH RR output, 64-bit state) in 16 bytes, without system calls
// (M. E. O'Neill, "PCG: A Family of Simple Fast Space-Efficient
// Statistically Good Algorithms for Random Number Generation", 2014). The
// same seed gives the same values, so tests of a machine are deterministic.
package pcg

import (
	"math"
	"math/bits"
	"time"
)

const multiplier = 6364136223846793005

// Source is a PCG32 generator. The zero value is the generator of seed 0
// before its first step, which is valid but differs from New(0).
type Source struct {
	state uint64
	inc   uint64
}

// New returns the Source of seed. Every seed, 0 included, is valid.
func New(seed uint64) Source {
	s := Source{inc: seed<<1 | 1}
	s.Uint32()
	s.state += seed
	s.Uint32()
	return s
}

// Uint32 returns the next value.
func (s *Source) Uint32() uint32 {
	old := s.state
	s.state = old*multiplier + s.inc
	x := uint32((old>>18 ^ old) >> 27 & math.MaxUint32)
	return bits.RotateLeft32(x, -int(old>>59))
}

// Duration returns a time from 0 to d, both included. It returns 0 for a d
// of 0 or less.
func (s *Source) Duration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	hi, _ := bits.Mul64(uint64(s.Uint32())<<32|uint64(s.Uint32()), uint64(d)+1)
	if hi > math.MaxInt64 {
		return d
	}
	return time.Duration(hi)
}
