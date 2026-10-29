package unpacked

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
)

func TestReadersUseOnlyTheLowBit(t *testing.T) {
	t.Parallel()

	raw := []Bit{0xFF, 2, 3, 0x30, 7, 0xFE, 1, 0}
	low := []Bit{1, 0, 1, 0, 1, 0, 1, 0}
	if Uint(raw) != Uint(low) || Pack(raw)[0] != Pack(low)[0] {
		t.Error("a reader used more than the low bit")
	}
	var raw96, low96 [96]Bit
	copy(raw96[:], raw)
	copy(low96[:], low)
	if Pack96(raw96) != Pack96(low96) {
		t.Error("Pack96 used more than the low bit")
	}
	var rawAt, lowAt [3]byte
	PackAt(rawAt[:], 3, raw)
	PackAt(lowAt[:], 3, low)
	if rawAt != lowAt {
		t.Error("PackAt used more than the low bit")
	}
	m := slices.Clone(raw)
	Mask(m)
	if !slices.Equal(m, low) {
		t.Errorf("Mask = %v, want %v", m, low)
	}
}

func TestUint(t *testing.T) {
	t.Parallel()

	bits := []Bit{1, 0, 1, 1, 0, 0, 1, 0}
	if got := Uint(bits); got != 0b10110010 {
		t.Errorf("Uint = %#b, want 0b10110010", got)
	}
	if got := Uint(bits[2:6]); got != 0b1100 {
		t.Errorf("Uint(partial) = %#b, want 0b1100", got)
	}
	if got := Uint(nil); got != 0 {
		t.Errorf("Uint(nil) = %d", got)
	}
	all := make([]Bit, 64)
	for i := range all {
		all[i] = 1
	}
	if got := Uint(all); got != ^uint64(0) {
		t.Errorf("Uint(64 ones) = %#x", got)
	}
}

func TestPutUint(t *testing.T) {
	t.Parallel()

	dst := make([]Bit, 10)
	PutUint(dst[2:8], 0b101101)
	if want := []Bit{0, 0, 1, 0, 1, 1, 0, 1, 0, 0}; !slices.Equal(dst, want) {
		t.Errorf("PutUint = %v, want %v", dst, want)
	}
	PutUint(dst[:4], 0xFFF5)
	if want := []Bit{0, 1, 0, 1}; !slices.Equal(dst[:4], want) {
		t.Errorf("PutUint truncation = %v, want %v", dst[:4], want)
	}
	for _, v := range []uint64{0, 1, 0xDEADBEEF, ^uint64(0)} {
		b := make([]Bit, 64)
		PutUint(b, v)
		if Uint(b) != v {
			t.Errorf("Uint(PutUint(%#x)) = %#x", v, Uint(b))
		}
	}
}

func TestPackUnpack(t *testing.T) {
	t.Parallel()

	data := []byte{0xA5, 0x3C, 0x01}
	bits := Unpack(data)
	if len(bits) != 24 {
		t.Fatalf("Unpack length %d", len(bits))
	}
	if got := Pack(bits); !slices.Equal(got, data) {
		t.Errorf("Pack(Unpack) = %x, want %x", got, data)
	}
	if got := Pack([]Bit{1, 0, 1}); !slices.Equal(got, []byte{0xA0}) {
		t.Errorf("Pack of a partial byte = %x, want a0", got)
	}
}

func TestAppendPackAppendUnpack(t *testing.T) {
	t.Parallel()

	dst := make([]byte, 1, 4)
	dst[0] = 0x77
	spare := dst[:cap(dst)]
	spare[1], spare[2] = 0xFF, 0xFF
	got := AppendPack(dst, []Bit{1, 0, 1, 0, 0, 1, 0, 1, 1})
	if want := []byte{0x77, 0xA5, 0x80}; !slices.Equal(got, want) {
		t.Errorf("AppendPack = %x, want %x", got, want)
	}
	if &got[0] != &dst[0] {
		t.Error("AppendPack allocated with spare capacity")
	}
	bits := AppendUnpack([]Bit{1}, []byte{0x81})
	if want := []Bit{1, 1, 0, 0, 0, 0, 0, 0, 1}; !slices.Equal(bits, want) {
		t.Errorf("AppendUnpack = %v, want %v", bits, want)
	}
	if got := AppendPack(nil, nil); len(got) != 0 {
		t.Errorf("AppendPack(nil, nil) = %x", got)
	}
}

func TestPack96Unpack96(t *testing.T) {
	t.Parallel()

	var data [12]byte
	for i := range data {
		data[i] = byte(i*29 + 3)
	}
	bits := Unpack96(data)
	if !slices.Equal(bits[:], Unpack(data[:])) {
		t.Error("Unpack96 differs from Unpack")
	}
	if got := Pack96(bits); got != data {
		t.Errorf("Pack96(Unpack96) = %x, want %x", got, data)
	}
}

func TestPackAtUnpackAt(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	for range 5000 {
		src := make([]byte, 33)
		dst := make([]byte, 33)
		r.Read(src)
		r.Read(dst)
		n := r.Intn(200)
		ss, ds := r.Intn(264-n+1), r.Intn(264-n+1)
		want := slices.Clone(dst)
		bits.Copy(want, ds, src, ss, n)
		u := make([]Bit, n)
		UnpackAt(u, src, ss)
		for i, b := range u {
			if byte(b) != bits.Bit(src, ss+i) {
				t.Fatalf("UnpackAt(%d, %d) bit %d = %d", ss, n, i, b)
			}
		}
		PackAt(dst, ds, u)
		if !slices.Equal(dst, want) {
			t.Fatalf("PackAt(%d, %d) = %x, want %x", ds, n, dst, want)
		}
	}
}
