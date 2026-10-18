package hamming

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// hamming743Seed is one seed of FuzzHamming743.
type hamming743Seed struct {
	data uint8
	mask uint8
}

// hamming743Seeds returns the seeds of FuzzHamming743.
func hamming743Seeds() []hamming743Seed {
	return []hamming743Seed{
		{uint8(0x0), uint8(0)},
		{uint8(0x9), uint8(0x08)},
		{uint8(0xF), uint8(0xC1)},
	}
}

// checkHamming743 checks the properties of FuzzHamming743 for one input.
func checkHamming743(t testing.TB, data uint8, mask uint8) {
	t.Helper()
	data &= 0xF
	cw := Encode743(data)
	rx := cw ^ mask
	n := bits.OnesCount8(mask & 0x7F)
	got, res := Decode743(rx)
	if n <= 1 && (got != cw || res != (fec.Result{BitsChecked: 7, ErrorsCorrected: n})) {
		t.Fatalf("errors %02x: got %02x %+v, want %02x", mask, got, res, cw)
	}
	if res.Uncorrectable || !Check743(got) || got > 0x7F || bits.OnesCount8(got^rx&0x7F) != res.ErrorsCorrected || res.ErrorsCorrected > 1 {
		t.Fatalf("Decode743(%02x) = %02x %+v is not a codeword at that distance", rx, got, res)
	}
	var in [7]unpacked.Bit
	unpacked.PutUint(in[:], uint64(rx))
	want, wantRes := ref.Decode743(in)
	if uint64(got) != unpacked.Uint(want[:]) || res != wantRes || Check743(rx) != ref.Check743(in) {
		t.Fatalf("Decode743(%02x) = %02x %+v, reference %02x %+v", rx, got, res, unpacked.Uint(want[:]), wantRes)
	}
}

// FuzzHamming743 encodes fuzzed data with Hamming (7,4,3), adds the error
// pattern mask and decodes. Up to 1 error decodes to the codeword with
// ErrorsCorrected equal to the count. The code is perfect, so every decode
// returns a codeword at most 1 bit from the received word and never sets
// Uncorrectable. Bits above the 7th are ignored. Decode and Check agree with
// the reference codec.
func FuzzHamming743(f *testing.F) {
	for _, s := range hamming743Seeds() {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data uint8, mask uint8) {
		checkHamming743(t, data, mask)
	})
}
