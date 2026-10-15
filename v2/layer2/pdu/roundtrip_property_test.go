package pdu_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/fec/reed_solomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func randomPayload(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.UintN(256))
	}
	return b
}

func withCRC(data []byte, mask uint16) [96]bit.Bit {
	c := crc.CalculateCRCCCITT(data) ^ mask
	var out [96]bit.Bit
	copy(out[:], bit.UnpackBits(append(append([]byte{}, data...), byte(c>>8), byte(c))))
	return out
}

func withRS(t *testing.T, data []byte, mask uint32) [96]bit.Bit {
	t.Helper()
	cw, err := reedsolomon.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	cw[9] ^= byte(mask >> 16)
	cw[10] ^= byte(mask >> 8)
	cw[11] ^= byte(mask)
	var out [96]bit.Bit
	copy(out[:], bit.UnpackBits(cw))
	return out
}

func TestPDURoundTripKeepsEveryBit(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	cases := []struct {
		name  string
		build func() [96]bit.Bit
		trip  func([96]bit.Bit) ([96]bit.Bit, fec.FECResult, error)
	}{
		{"FullLinkControl", func() [96]bit.Bit { return withRS(t, randomPayload(rng, 9), 0x969696) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodeFullLinkControl(in, elements.DataTypeVoiceLCHeader)
				out, err := pdu.EncodeFullLinkControl(&d)
				return out, r, err
			}},
		{"CSBK", func() [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0xA5A5) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodeCSBK(in, false)
				out, err := pdu.EncodeCSBK(&d)
				return out, r, err
			}},
		{"CSBK trunking", func() [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0xA5A5) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodeCSBK(in, true)
				out, err := pdu.EncodeCSBK(&d)
				return out, r, err
			}},
		{"DataHeader", func() [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0xCCCC) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodeDataHeader(in)
				out, err := pdu.EncodeDataHeader(&d)
				return out, r, err
			}},
		{"MBCHeader", func() [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0xAAAA) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodeMBCHeader(in)
				out, err := pdu.EncodeMBCHeader(&d)
				return out, r, err
			}},
		{"PIHeader", func() [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0x6969) },
			func(in [96]bit.Bit) ([96]bit.Bit, fec.FECResult, error) {
				d, r := pdu.DecodePIHeader(in)
				out, err := pdu.EncodePIHeader(&d)
				return out, r, err
			}},
	}
	for _, c := range cases {
		lost := map[int]int{}
		var encodeErrs int
		for range 20000 {
			in := c.build()
			out, r, err := c.trip(in)
			if r.Uncorrectable {
				t.Fatalf("%s: valid input decoded as uncorrectable", c.name)
			}
			if err != nil {
				encodeErrs++
				continue
			}
			for i := range in {
				if in[i] != out[i] {
					lost[i]++
				}
			}
		}
		if encodeErrs > 0 {
			t.Errorf("%s: %d of 20000 decoded PDUs failed to re-encode", c.name, encodeErrs)
		}
		if len(lost) > 0 {
			t.Errorf("%s: Encode(Decode(x)) changed bits (bit index: count): %v", c.name, lost)
		}
	}
}

func TestShortLCRoundTripKeepsEveryBit(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for range 20000 {
		var data [24]bit.Bit
		for i := range data {
			data[i] = bit.Bit(rng.UintN(2))
		}
		in := buildShortLCBits(byte(rng.UintN(16)), data)
		d, r := pdu.DecodeShortLC(in)
		if r.Uncorrectable {
			t.Fatal("valid Short LC decoded as uncorrectable")
		}
		if out := must(pdu.EncodeShortLC(&d)); out != in {
			t.Fatalf("Encode(Decode(x)) changed bits: SLCO %d\nin  %v\nout %v", d.SLCO, in, out)
		}
	}
}
