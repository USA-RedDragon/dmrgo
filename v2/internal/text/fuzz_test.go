package text

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

// FuzzInt checks Int, Uint and Bool against the %d and %t verbs, and that
// Int and Uint parse back to the value.
func FuzzInt(f *testing.F) {
	for _, v := range []int64{0, 1, -1, 9, 10, -10, math.MaxInt64, math.MinInt64} {
		f.Add(v, v < 0)
	}
	f.Fuzz(func(t *testing.T, v int64, b bool) {
		s := Int(v)
		if want := fmt.Sprintf("%d", v); s != want {
			t.Fatalf("Int(%d) = %q", v, s)
		}
		if back, err := strconv.ParseInt(s, 10, 64); err != nil || back != v {
			t.Fatalf("Int(%d) = %q parses as %d, %v", v, s, back, err)
		}
		u := uint64(v)
		if got, want := Uint(u), fmt.Sprintf("%d", u); got != want {
			t.Fatalf("Uint(%d) = %q", u, got)
		}
		if back, err := strconv.ParseUint(Uint(u), 10, 64); err != nil || back != u {
			t.Fatalf("Uint(%d) parses as %d, %v", u, back, err)
		}
		if got, want := Bool(b), fmt.Sprintf("%t", b); got != want {
			t.Fatalf("Bool(%t) = %q", b, got)
		}
	})
}

// FuzzBytes checks Hex and Bytes against the %x and %v verbs, and that Hex
// decodes back to the bytes.
func FuzzBytes(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B})
	f.Add([]byte{0xF8, 0x01, 0xA9, 0x9F, 0x8C, 0xE0, 0x80})
	f.Add([]byte{0xFF, 0x10, 0x09, 0x63, 0x64})
	f.Fuzz(func(t *testing.T, b []byte) {
		h := Hex(b)
		if want := fmt.Sprintf("%x", b); h != want {
			t.Fatalf("Hex(%v) = %q, want %q", b, h, want)
		}
		for i := range b {
			if v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8); err != nil || byte(v) != b[i] {
				t.Fatalf("Hex(%v) = %q: byte %d reads as %d, %v", b, h, i, v, err)
			}
		}
		if got, want := Bytes(b), fmt.Sprintf("%v", b); got != want {
			t.Fatalf("Bytes(%v) = %q, want %q", b, got, want)
		}
	})
}

// FuzzDigits checks Digits for shift 1 to 4 and n 0 to 70: the result has
// at least n digits and at least one, has no extra leading zero, and parses
// back to v in base 1<<shift. For shift 1 and 4 it equals the %0nb and %0nx verbs.
func FuzzDigits(f *testing.F) {
	f.Add(uint64(0), uint8(0), uint8(1))
	f.Add(uint64(0xA5), uint8(8), uint8(1))
	f.Add(uint64(0x123456789AB), uint8(12), uint8(4))
	f.Add(uint64(math.MaxUint64), uint8(4), uint8(1))
	f.Add(uint64(math.MaxUint64), uint8(70), uint8(3))
	f.Add(uint64(0o1234567), uint8(3), uint8(3))
	f.Fuzz(func(t *testing.T, v uint64, n, shift uint8) {
		n %= 71
		shift = shift%4 + 1
		s := Digits(v, int(n), uint(shift))
		if len(s) < max(int(n), 1) || len(s) > max(int(n), 1) && s[0] == '0' {
			t.Fatalf("Digits(%d, %d, %d) = %q", v, n, shift, s)
		}
		if back, err := strconv.ParseUint(s, 1<<shift, 64); err != nil || back != v {
			t.Fatalf("Digits(%d, %d, %d) = %q parses as %d, %v", v, n, shift, s, back, err)
		}
		switch shift {
		case 1:
			if want := fmt.Sprintf("%0*b", n, v); s != want {
				t.Fatalf("Digits(%d, %d, 1) = %q, want %q", v, n, s, want)
			}
		case 4:
			if want := fmt.Sprintf("%0*x", n, v); s != want {
				t.Fatalf("Digits(%d, %d, 4) = %q, want %q", v, n, s, want)
			}
		}
	})
}

// FuzzFixed checks Fixed against the %f verb for any float64, and Float
// against the %v verb where Float documents that they agree: 0, or a
// magnitude from 1e-4 to 1e6 with at most six decimals. Float is always
// Fixed without trailing zeros.
func FuzzFixed(f *testing.F) {
	for _, v := range []float64{
		0, math.Copysign(0, -1), 1, -1, 0.5, 0.0078125, 2.5e-7, 5e-7, 0.9999995, 0.99999949,
		1e15, 1e21, 1e300, math.MaxFloat64, math.SmallestNonzeroFloat64, 180, -180, 90,
		999999.125, 1e-4, math.NaN(), math.Inf(1), math.Inf(-1),
	} {
		f.Add(math.Float64bits(v))
	}
	f.Fuzz(func(t *testing.T, x uint64) {
		v := math.Float64frombits(x)
		s := Fixed(v)
		if want := fmt.Sprintf("%f", v); s != want {
			t.Fatalf("Fixed(%b) = %q, want %q", v, s, want)
		}
		g := Float(v)
		if dot := strings.IndexByte(s, '.'); dot >= 0 {
			trimmed := strings.TrimRight(strings.TrimRight(s, "0"), ".")
			if g != trimmed {
				t.Fatalf("Float(%b) = %q, want %q", v, g, trimmed)
			}
		} else if g != s {
			t.Fatalf("Float(%b) = %q, want %q", v, g, s)
		}
		short := strconv.FormatFloat(v, 'f', -1, 64)
		dec := 0
		if dot := strings.IndexByte(short, '.'); dot >= 0 {
			dec = len(short) - dot - 1
		}
		if a := math.Abs(v); dec <= 6 && (a == 0 || a >= 1e-4 && a < 1e6) {
			if want := fmt.Sprintf("%v", v); g != want {
				t.Fatalf("Float(%b) = %q, want %q", v, g, want)
			}
		}
	})
}
