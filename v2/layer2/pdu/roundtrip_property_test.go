package pdu_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// samplesPerValue is the number of random payloads per forced discriminator
// value.
const samplesPerValue = 200

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
	cw := reedsolomon.Encode([9]byte(data))
	cw[9] ^= byte(mask >> 16)
	cw[10] ^= byte(mask >> 8)
	cw[11] ^= byte(mask)
	var out [96]bit.Bit
	copy(out[:], bit.UnpackBits(cw[:]))
	return out
}

// forceOpcode sets the 6-bit CSBKO or FLCO in octet 0 and the FID in octet 1.
func forceOpcode(b []byte, opcode int, fid byte) {
	b[0] = b[0]&0xC0 | byte(opcode)
	b[1] = fid
}

// manufacturerFID returns a random non-zero FID.
func manufacturerFID(rng *rand.Rand) byte {
	return byte(1 + rng.UintN(255))
}

type roundTrip struct {
	name string
	// values is the number of forced discriminator values; build gets each
	// value samplesPerValue times.
	values int
	build  func(value int) [96]bit.Bit
	trip   func([96]bit.Bit) ([96]bit.Bit, fec.Result, error)
}

// TestPDURoundTripKeepsEveryBit forces every discriminator value (FLCO and
// CSBKO with the SFID and with a manufacturer FID, Format with and without
// appended blocks) so that rare payload variants are reached.
func TestPDURoundTripKeepsEveryBit(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var cases []roundTrip
	for _, lc := range []struct {
		dt   elements.DataType
		mask uint32
	}{
		{elements.DataTypeVoiceLCHeader, 0x969696},
		{elements.DataTypeTerminatorWithLC, 0x999999},
	} {
		cases = append(cases, roundTrip{
			name:   "FullLinkControl " + elements.DataTypeToName(lc.dt),
			values: 64 * 2,
			build: func(v int) [96]bit.Bit {
				b := randomPayload(rng, 9)
				fid := byte(0)
				if v%2 == 1 {
					fid = manufacturerFID(rng)
				}
				forceOpcode(b, v/2, fid)
				return withRS(t, b, lc.mask)
			},
			trip: func(in [96]bit.Bit) ([96]bit.Bit, fec.Result, error) {
				d, r := pdu.DecodeFullLinkControl(in, lc.dt)
				out, err := pdu.EncodeFullLinkControl(&d)
				return out, r, err
			},
		})
	}
	for _, trunking := range []bool{false, true} {
		name := "CSBK"
		if trunking {
			name = "CSBK trunking"
		}
		cases = append(cases, roundTrip{
			name:   name,
			values: 64 * 2,
			build: func(v int) [96]bit.Bit {
				b := randomPayload(rng, 10)
				fid := byte(0)
				if v%2 == 1 {
					fid = manufacturerFID(rng)
				}
				forceOpcode(b, v/2, fid)
				return withCRC(b, 0xA5A5)
			},
			trip: func(in [96]bit.Bit) ([96]bit.Bit, fec.Result, error) {
				d, r := pdu.DecodeCSBK(in, trunking)
				out, err := pdu.EncodeCSBK(&d)
				return out, r, err
			},
		})
	}
	cases = append(cases,
		roundTrip{
			name:   "DataHeader",
			values: 16 * 2,
			build: func(v int) [96]bit.Bit {
				b := randomPayload(rng, 10)
				b[0] = b[0]&0xF0 | byte(v/2)
				if v%2 == 0 {
					// Appended Blocks = 0 (octet 0 bits 5-4, octet 1 bits 3-0)
					// selects SP_HEAD for Format 0b1110.
					b[0] &^= 0x30
					b[1] &^= 0x0F
				}
				return withCRC(b, 0xCCCC)
			},
			trip: func(in [96]bit.Bit) ([96]bit.Bit, fec.Result, error) {
				d, r := pdu.DecodeDataHeader(in)
				out, err := pdu.EncodeDataHeader(&d)
				return out, r, err
			},
		},
		roundTrip{
			name:   "MBCHeader",
			values: 1,
			build:  func(int) [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0xAAAA) },
			trip: func(in [96]bit.Bit) ([96]bit.Bit, fec.Result, error) {
				d, r := pdu.DecodeMBCHeader(in)
				out, err := pdu.EncodeMBCHeader(&d)
				return out, r, err
			},
		},
		roundTrip{
			name:   "PIHeader",
			values: 1,
			build:  func(int) [96]bit.Bit { return withCRC(randomPayload(rng, 10), 0x6969) },
			trip: func(in [96]bit.Bit) ([96]bit.Bit, fec.Result, error) {
				d, r := pdu.DecodePIHeader(in)
				out, err := pdu.EncodePIHeader(&d)
				return out, r, err
			},
		},
	)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lost := map[int]int{}
			var encodeErrs int
			var firstErr error
			for v := range c.values {
				for range samplesPerValue {
					in := c.build(v)
					out, r, err := c.trip(in)
					if r.Uncorrectable {
						t.Fatalf("value %d: valid input decoded as uncorrectable", v)
					}
					if err != nil {
						encodeErrs++
						if firstErr == nil {
							firstErr = err
						}
						continue
					}
					for i := range in {
						if in[i] != out[i] {
							lost[i]++
						}
					}
				}
			}
			if encodeErrs > 0 {
				t.Errorf("%d decoded PDUs failed to re-encode, first: %v", encodeErrs, firstErr)
			}
			if len(lost) > 0 {
				t.Errorf("Encode(Decode(x)) changed bits (bit index: count): %v", lost)
			}
		})
	}
}

func TestShortLCRoundTripKeepsEveryBit(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for slco := range 16 {
		for range samplesPerValue {
			var data [24]bit.Bit
			for i := range data {
				data[i] = bit.Bit(rng.UintN(2))
			}
			in := buildShortLCBits(byte(slco), data)
			d, r := pdu.DecodeShortLC(in)
			if r.Uncorrectable {
				t.Fatal("valid Short LC decoded as uncorrectable")
			}
			if out := must(pdu.EncodeShortLC(&d)); out != in {
				t.Fatalf("Encode(Decode(x)) changed bits: SLCO %d\nin  %v\nout %v", d.SLCO, in, out)
			}
		}
	}
}
