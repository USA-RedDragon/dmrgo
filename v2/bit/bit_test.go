package bit

import (
	"slices"
	"testing"
)

func TestReadersUseOnlyTheLowBit(t *testing.T) {
	t.Parallel()

	raw := []Bit{0xFF, 2, 3, 0x30, 7, 0xFE, 1, 0}
	low := []Bit{1, 0, 1, 0, 1, 0, 1, 0}
	if Uint(raw) != Uint(low) || Pack(raw)[0] != Pack(low)[0] {
		t.Error("a reader used more than the low bit")
	}
	var raw264, low264 [264]Bit
	copy(raw264[:], raw)
	copy(low264[:], low)
	if Pack264(raw264) != Pack264(low264) {
		t.Error("Pack264 used more than the low bit")
	}
	var raw96, low96 [96]Bit
	copy(raw96[:], raw)
	copy(low96[:], low)
	if Pack96(raw96) != Pack96(low96) {
		t.Error("Pack96 used more than the low bit")
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

func TestPack264Unpack264(t *testing.T) {
	t.Parallel()

	var data [33]byte
	for i := range data {
		data[i] = byte(i * 7)
	}
	if got := Pack264(Unpack264(data)); got != data {
		t.Errorf("Pack264(Unpack264) = %x, want %x", got, data)
	}
}

func TestPack32Unpack32(t *testing.T) {
	t.Parallel()

	data := [4]byte{0xA5, 0x3C, 0x01, 0xFE}
	bits := Unpack32(data)
	if !slices.Equal(bits[:], Unpack(data[:])) {
		t.Error("Unpack32 differs from Unpack")
	}
	if got := Pack32(bits); got != data {
		t.Errorf("Pack32(Unpack32) = %x, want %x", got, data)
	}
	var raw [32]Bit
	for i := range raw {
		raw[i] = bits[i] | 0x30
	}
	if Pack32(raw) != data {
		t.Error("Pack32 used more than the low bit")
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
