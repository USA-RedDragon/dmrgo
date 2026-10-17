// Package text formats values as the fmt verbs that the String and Error
// methods of the library use. It does not import fmt or strconv, so a
// TinyGo build of the library does not link them.
package text

import (
	"math"
	"math/bits"
)

// Integer is the set of integer types that Int formats.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

const hexDigits = "0123456789abcdef"

// Int returns v in decimal, as the %d verb.
func Int[T Integer](v T) string {
	var b [20]byte
	u := uint64(v)
	if v < 0 {
		return "-" + string(b[putUint(b[:], -u):])
	}
	return string(b[putUint(b[:], u):])
}

// putUint writes v in decimal at the end of b and returns the index of the
// first digit.
func putUint(b []byte, v uint64) int {
	i := len(b)
	for {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			return i
		}
	}
}

// Bool returns "true" or "false", as the %t verb.
func Bool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// Hex returns b as lowercase hex, two digits per byte, as the %x verb.
func Hex(b []byte) string {
	s := make([]byte, 2*len(b))
	for i, c := range b {
		s[2*i] = hexDigits[c>>4]
		s[2*i+1] = hexDigits[c&0x0F]
	}
	return string(s)
}

// Bytes returns b as decimal bytes in brackets, as the %v verb.
func Bytes(b []byte) string {
	s := make([]byte, 1, 2+4*len(b))
	s[0] = '['
	var d [20]byte
	for i, c := range b {
		if i > 0 {
			s = append(s, ' ')
		}
		s = append(s, d[putUint(d[:], uint64(c)):]...)
	}
	return string(append(s, ']'))
}

// Digits returns v in base 1<<shift, lowercase, with leading zeros to at
// least n digits. With shift 1 and 4 it is the %0nb and %0nx verbs.
func Digits(v uint64, n int, shift uint) string {
	for x := v >> (uint(n) * shift); x != 0 && n < 64; x >>= shift {
		n++
	}
	s := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		s[i] = hexDigits[v&(1<<shift-1)]
		v >>= shift
	}
	return string(s)
}

// Fixed returns v with six decimals, as the %f verb.
func Fixed(v float64) string {
	switch {
	case v != v:
		return "NaN"
	case v > math.MaxFloat64:
		return "+Inf"
	case v < -math.MaxFloat64:
		return "-Inf"
	}
	f := math.Float64bits(v)
	sign := ""
	if f>>63 != 0 {
		sign = "-"
	}
	exp := int(f >> 52 & 0x7FF)
	mant := f & (1<<52 - 1)
	if exp == 0 {
		exp = 1
	} else {
		mant |= 1 << 52
	}
	// v is mant * 2^e.
	e := exp - 1075
	if e >= 0 {
		return sign + shiftDecimal(mant, e) + ".000000"
	}
	n := uint(-e)
	var ip, fr uint64
	if n < 64 {
		ip, fr = mant>>n, mant&(1<<n-1)
	} else {
		fr = mant
	}
	// The fraction is fr / 2^n. Round fr * 10^6 / 2^n half to even.
	hi, lo := bits.Mul64(fr, 1e6)
	var q uint64
	if n < 128 {
		top, rest := shift128(hi, lo, n-1)
		q = top >> 1
		if top&1 != 0 && (rest || q&1 != 0) {
			q++
		}
	}
	if q == 1e6 {
		ip, q = ip+1, 0
	}
	var b [27]byte
	i := putUint(b[:20], ip)
	copy(b[20:], ".000000")
	putUint(b[21:], q)
	return sign + string(b[i:20]) + string(b[20:])
}

// shift128 returns the 128-bit value hi:lo shifted right by n, which must be
// less than 128 and leave a result that fits in 64 bits, and whether any of
// the bits shifted out is 1.
func shift128(hi, lo uint64, n uint) (uint64, bool) {
	switch {
	case n == 0:
		return lo, false
	case n < 64:
		return lo>>n | hi<<(64-n), lo&(1<<n-1) != 0
	default:
		return hi >> (n - 64), lo != 0 || hi&(1<<(n-64)-1) != 0
	}
}

// shiftDecimal returns mant * 2^e in decimal.
func shiftDecimal(mant uint64, e int) string {
	// Little-endian decimal digits.
	d := make([]byte, 0, 330)
	for ; mant > 0; mant /= 10 {
		d = append(d, byte(mant%10))
	}
	for ; e > 0; e-- {
		var carry byte
		for i := range d {
			x := d[i]*2 + carry
			d[i], carry = x%10, x/10
		}
		if carry > 0 {
			d = append(d, carry)
		}
	}
	s := make([]byte, len(d))
	for i, c := range d {
		s[len(d)-1-i] = '0' + c
	}
	return string(s)
}

// Float returns Fixed(v) without trailing zeros. It is the %v verb for a v
// with at most six decimals that is 0 or between 1e-4 and 1e6 in magnitude.
func Float(v float64) string {
	s := Fixed(v)
	if len(s) < 8 || s[len(s)-7] != '.' {
		return s
	}
	i := len(s)
	for s[i-1] == '0' {
		i--
	}
	if s[i-1] == '.' {
		i--
	}
	return s[:i]
}
