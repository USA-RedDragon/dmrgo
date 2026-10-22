package wire_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
)

func TestUintvar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   []byte
		want uint64
	}{
		{[]byte{0x3C}, 60},
		{[]byte{0x81, 0x7A}, 250},
		{[]byte{0x83, 0x74}, 500},
		{[]byte{0x00}, 0},
		{[]byte{0x81, 0x80, 0x00}, 1 << 14},
	}
	for _, tt := range tests {
		v, n, st := wire.Uintvar(tt.in, 0)
		if st != wire.OK || v != tt.want || n != len(tt.in) {
			t.Errorf("Uintvar(%x) = %d, %d, %d", tt.in, v, n, st)
		}
		if got := wire.AppendUintvar(nil, tt.want); tt.in[0] != 0x81 || tt.want != 1<<14 {
			if !bytes.Equal(got, tt.in) {
				t.Errorf("AppendUintvar(%d) = %x, want %x", tt.want, got, tt.in)
			}
		}
	}
	b := make([]byte, wire.UintvarLen(500))
	wire.PutUintvar(b, 500)
	if !bytes.Equal(b, []byte{0x83, 0x74}) || wire.UintvarLen(0) != 1 || wire.UintvarLen(127) != 1 || wire.UintvarLen(128) != 2 {
		t.Errorf("PutUintvar(500) = %x", b)
	}
	if _, _, st := wire.Uintvar([]byte{0x81}, 0); st != wire.Short {
		t.Errorf("Uintvar of a cut value: %d", st)
	}
	long := bytes.Repeat([]byte{0xFF}, 10)
	if _, _, st := wire.Uintvar(append(long, 0x7F), 0); st != wire.Overflow {
		t.Errorf("Uintvar of 11 octets: %d", st)
	}
}

func TestSintvar(t *testing.T) {
	t.Parallel()

	for _, v := range []int64{0, 1, -1, 63, 64, -64, 8191, -8192, 1 << 40, -(1 << 40)} {
		b := wire.AppendSintvar(nil, v, false)
		got, neg, n, st := wire.Sintvar(b, 0)
		if st != wire.OK || got != v || n != len(b) || neg != (v < 0) {
			t.Errorf("Sintvar(AppendSintvar(%d) = %x) = %d, %t, %d, %d", v, b, got, neg, n, st)
		}
	}
	if b := wire.AppendSintvar(nil, 63, false); !bytes.Equal(b, []byte{0x3F}) {
		t.Errorf("AppendSintvar(63) = %x", b)
	}
	if b := wire.AppendSintvar(nil, 64, false); !bytes.Equal(b, []byte{0x80, 0x40}) {
		t.Errorf("AppendSintvar(64) = %x", b)
	}
	if b := wire.AppendSintvar(nil, 0, true); !bytes.Equal(b, []byte{0x40}) {
		t.Errorf("AppendSintvar(-0) = %x", b)
	}
}

func TestFloatvar(t *testing.T) {
	t.Parallel()

	v, n, st := wire.Ufloatvar([]byte{0x00, 0x63}, 0)
	if st != wire.OK || v != 99.0/128 || n != 2 {
		t.Errorf("Ufloatvar(00 63) = %v, %d, %d", v, n, st)
	}
	v, n, st = wire.Sfloatvar([]byte{0x40, 0x2B}, 0)
	if st != wire.OK || v != -43.0/128 || n != 2 {
		t.Errorf("Sfloatvar(40 2b) = %v, %d, %d", v, n, st)
	}
	v, n, st = wire.Sfloatvar([]byte{0x40, 0x00}, 0)
	if st != wire.OK || v != 0 || !math.Signbit(v) || n != 2 {
		t.Errorf("Sfloatvar(40 00) = %v, %d, %d", v, n, st)
	}
	if b := wire.AppendSfloatvar(nil, v, 1); !bytes.Equal(b, []byte{0x40, 0x00}) {
		t.Errorf("AppendSfloatvar(-0) = %x", b)
	}
	v, _, st = wire.Sfloatvar([]byte{0x82, 0x53, 0x80, 0x40}, 0)
	if st != wire.OK || v != 339+64.0/16384 {
		t.Errorf("Sfloatvar = %v, %d", v, st)
	}
	if _, _, st := wire.Ufloatvar([]byte{0x90, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00, 0x00}, 0); st != wire.Overflow {
		t.Errorf("Ufloatvar of 2^53: %d", st)
	}
	if b := wire.AppendUfloatvar(nil, 0.7734375, 1); !bytes.Equal(b, []byte{0x00, 0x63}) {
		t.Errorf("AppendUfloatvar = %x", b)
	}
	if b := wire.AppendSfloatvar(nil, -43.0/128, 1); !bytes.Equal(b, []byte{0x40, 0x2B}) {
		t.Errorf("AppendSfloatvar = %x", b)
	}
	if b := wire.AppendUfloatvar(nil, 2.5, 2); !bytes.Equal(b, []byte{0x02, 0xC0, 0x00}) {
		t.Errorf("AppendUfloatvar(2.5, 2) = %x", b)
	}
	if b := wire.AppendUfloatvar(nil, 0.9999999, 1); !bytes.Equal(b, []byte{0x00, 0x7F}) {
		t.Errorf("AppendUfloatvar(0.9999999) = %x", b)
	}
	for _, c := range []struct {
		v      float64
		signed bool
		want   bool
	}{{1, false, true}, {-1, false, false}, {-1, true, true}, {math.NaN(), true, false}, {math.Inf(1), true, false}, {1 << 53, true, false}, {1<<53 - 1, false, true}} {
		if got := wire.FloatvarFits(c.v, c.signed); got != c.want {
			t.Errorf("FloatvarFits(%v, %t) = %t", c.v, c.signed, got)
		}
	}
}

func TestUTF16(t *testing.T) {
	t.Parallel()

	in := []byte("Aé€\U0001F600")
	le := wire.AppendUTF16(nil, in, false)
	if !bytes.Equal(le, []byte{0x41, 0, 0xE9, 0, 0xAC, 0x20, 0x3D, 0xD8, 0x00, 0xDE}) {
		t.Errorf("AppendUTF16 LE = %x", le)
	}
	if got := wire.AppendUTF8(nil, le, false); !bytes.Equal(got, in) {
		t.Errorf("AppendUTF8 LE = %q", got)
	}
	be := wire.AppendUTF16(nil, in, true)
	if got := wire.AppendUTF8(nil, be, true); !bytes.Equal(got, in) {
		t.Errorf("AppendUTF8 BE = %q", got)
	}
	if got := wire.AppendUTF8(nil, []byte{0x41, 0, 0, 0, 0x42, 0}, false); string(got) != "A" {
		t.Errorf("text after NUL: %q", got)
	}
	if got := wire.AppendUTF8([]byte{0x7F}, []byte{0x41, 0, 0x7F, 0, 0x42, 0, 0x7F, 0, 0x7F, 0}, false); string(got) != "\x7fA\x7fB" {
		t.Errorf("trailing DEL: %q", got)
	}
	if got := wire.AppendUTF8([]byte{0x7F}, []byte{0x7F, 0}, false); string(got) != "\x7f" {
		t.Errorf("only DEL: %q", got)
	}
	if got := wire.AppendUTF8(nil, []byte{0x00, 0xD8, 0x41, 0}, false); string(got) != "�A" {
		t.Errorf("lone surrogate: %q", got)
	}
	if got := wire.AppendUTF16(nil, []byte{0xFF, 'A', 0xC3}, false); !bytes.Equal(got, []byte{0xFD, 0xFF, 0x41, 0, 0xFD, 0xFF}) {
		t.Errorf("invalid UTF-8: %x", got)
	}
}
