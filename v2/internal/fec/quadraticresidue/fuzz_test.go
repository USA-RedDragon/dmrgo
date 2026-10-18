package quadraticresidue

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/quadraticresidue"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// qr1676Seed is one seed of FuzzQR1676.
type qr1676Seed struct {
	data uint8
	mask uint16
}

// qr1676Seeds returns the seeds of FuzzQR1676.
func qr1676Seeds() []qr1676Seed {
	return []qr1676Seed{
		{uint8(0x10), uint16(0)},
		{uint8(0x5A), uint16(0x0101)},
		{uint8(0x7F), uint16(0x8003)},
	}
}

// checkQR1676 checks the properties of FuzzQR1676 for one input.
func checkQR1676(t testing.TB, data uint8, mask uint16) {
	t.Helper()
	data &= 0x7F
	cw := Encode(data)
	rx := cw ^ mask
	n := bits.OnesCount16(mask)
	got, res := Decode(rx)
	if n <= 2 && (got != cw || res != (fec.Result{BitsChecked: 16, ErrorsCorrected: n})) {
		t.Fatalf("errors %04x: got %04x %+v, want %04x", mask, got, res, cw)
	}
	if res.Uncorrectable {
		if got != rx || res.ErrorsCorrected != 0 {
			t.Fatalf("Decode(%04x) = %04x %+v, want the received word", rx, got, res)
		}
	} else if !Check(got) || bits.OnesCount16(got^rx) != res.ErrorsCorrected || res.ErrorsCorrected > 2 {
		t.Fatalf("Decode(%04x) = %04x %+v is not a codeword at that distance", rx, got, res)
	}
	var in [16]unpacked.Bit
	unpacked.PutUint(in[:], uint64(rx))
	want, wantRes := ref.Decode(in)
	if uint64(got) != unpacked.Uint(want[:]) || res != wantRes || Check(rx) != ref.Check(in) {
		t.Fatalf("Decode(%04x) = %04x %+v, reference %04x %+v", rx, got, res, unpacked.Uint(want[:]), wantRes)
	}
}

// FuzzQR1676 encodes fuzzed data with QR (16,7,6), adds the error pattern
// mask and decodes. Up to 2 errors decode to the codeword with
// ErrorsCorrected equal to their count. A decode without Uncorrectable
// returns a codeword at most 2 and exactly ErrorsCorrected bits away, and an
// uncorrectable one the received word. Decode and Check agree with the
// reference codec.
func FuzzQR1676(f *testing.F) {
	for _, s := range qr1676Seeds() {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data uint8, mask uint16) {
		checkQR1676(t, data, mask)
	})
}
