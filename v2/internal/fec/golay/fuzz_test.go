package golay

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/golay"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

type golaySeed struct {
	data uint16
	mask uint32
}

func golaySeeds() []golaySeed {
	return []golaySeed{
		{uint16(0x0A5), uint32(0)},
		{uint16(0x3C3), uint32(0x00000101)},
		{uint16(0xFFF), uint32(0x00800007)},
		{uint16(0x123), uint32(0x0100000F)},
		{uint16(0x9B1), uint32(0xFFFFFFFF)},
	}
}

func checkGolay(t testing.TB, data uint16, mask uint32) {
	t.Helper()
	data &= 0xFFF

	cw20 := Encode2087(uint8(data))
	rx20 := cw20 ^ mask
	n20 := bits.OnesCount32(mask & 0xFFFFF)
	got20, res20 := Decode2087(rx20)
	if n20 <= 3 && (got20 != cw20 || res20 != (fec.Result{BitsChecked: 20, ErrorsCorrected: n20})) {
		t.Fatalf("Golay(20,8) errors %05x: got %05x %+v, want %05x", mask, got20, res20, cw20)
	}
	if !res20.Uncorrectable && (!Check2087(got20) || bits.OnesCount32(got20^rx20&0xFFFFF) != res20.ErrorsCorrected || res20.ErrorsCorrected > 3) {
		t.Fatalf("Decode2087(%08x) = %05x %+v is not a codeword at that distance", rx20, got20, res20)
	}
	var in20 [20]unpacked.Bit
	unpacked.PutUint(in20[:], uint64(rx20))
	if want, wantRes := ref.Decode2087(in20); got20 != uint32(unpacked.Uint(want[:])) || res20 != wantRes {
		t.Fatalf("Decode2087(%05x) = %05x %+v, reference %05x %+v", rx20&0xFFFFF, got20, res20, unpacked.Uint(want[:]), wantRes)
	}

	cw23 := Encode23127(data)
	if want := ref.Encode23127(data); cw23 != want {
		t.Fatalf("Encode23127(%03x) = %06x, reference %06x", data, cw23, want)
	}
	rx23 := cw23 ^ mask
	n23 := bits.OnesCount32(mask & (1<<23 - 1))
	wide23 := rx23>>23 != 0
	got23, res23 := Decode23127(rx23)
	if n23 <= 3 && (got23 != data || res23 != (fec.Result{BitsChecked: 23, ErrorsCorrected: n23, Uncorrectable: wide23})) {
		t.Fatalf("Golay(23,12) errors %08x: got %03x %+v, want %03x", mask, got23, res23, data)
	}
	if res23.Uncorrectable != wide23 || res23.ErrorsCorrected > 3 || bits.OnesCount32(Encode23127(got23)^rx23&(1<<23-1)) != res23.ErrorsCorrected {
		t.Fatalf("Decode23127(%08x) = %03x %+v is not a codeword at that distance", rx23, got23, res23)
	}
	if want, wantRes := ref.Decode23127(rx23); got23 != want || res23 != wantRes {
		t.Fatalf("Decode23127(%08x) = %03x %+v, reference %03x %+v", rx23, got23, res23, want, wantRes)
	}

	cw24 := Encode24128(data)
	if want := ref.Encode24128(data); cw24 != want {
		t.Fatalf("Encode24128(%03x) = %06x, reference %06x", data, cw24, want)
	}
	rx24 := cw24 ^ mask
	n24 := bits.OnesCount32(mask & (1<<24 - 1))
	wide24 := rx24>>24 != 0
	got24, res24 := Decode24128(rx24)
	switch {
	case n24 <= 3 && (got24 != data || res24 != (fec.Result{BitsChecked: 24, ErrorsCorrected: n24, Uncorrectable: wide24})):
		t.Fatalf("Golay(24,12) errors %08x: got %03x %+v, want %03x", mask, got24, res24, data)
	case n24 == 4 && !res24.Uncorrectable:
		t.Fatalf("Golay(24,12) errors %08x: accepted %03x %+v", mask, got24, res24)
	}
	if !res24.Uncorrectable && (res24.ErrorsCorrected > 3 || bits.OnesCount32(Encode24128(got24)^rx24) != res24.ErrorsCorrected) {
		t.Fatalf("Decode24128(%08x) = %03x %+v is not a codeword at that distance", rx24, got24, res24)
	}
	if want, wantRes := ref.Decode24128(rx24); got24 != want || res24 != wantRes {
		t.Fatalf("Decode24128(%08x) = %03x %+v, reference %03x %+v", rx24, got24, res24, want, wantRes)
	}
}

func FuzzGolay(f *testing.F) {
	for _, s := range golaySeeds() {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data uint16, mask uint32) {
		checkGolay(t, data, mask)
	})
}
