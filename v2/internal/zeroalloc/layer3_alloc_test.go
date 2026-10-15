package zeroalloc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// layer3Entries returns the entries of package layer3, with the Service
// Options of the voice LC header in layer2/testdata/voice.bin.
func layer3Entries(t testing.TB) []entry {
	t.Helper()
	header := splitBurst(captureRawBursts(t, "voice")[0])
	lcBits, _ := bptc.Decode196(header.info)
	soBits := [8]bit.Bit(lcBits[8:16])
	soByte := byte(bit.Uint(soBits[:]))
	so := layer3.DecodeServiceOptions(soBits)
	if so.Byte() != soByte || layer3.ServiceOptionsFromByte(soByte) != so {
		t.Fatalf("service options %+v, octet %#x", so, soByte)
	}
	cdef := layer3.CdefParms{Channel: 12, TXMHz: 439, TXKHz: 4000, RXMHz: 444, RXKHz: 4000}
	cdefBits, err := cdef.Encode()
	if err != nil || layer3.DecodeCdefParms(cdefBits) != cdef {
		t.Fatalf("CdefParms round trip: %v", err)
	}
	var (
		b    byte
		hz   uint64
		mhz  float64
		bits [8]bit.Bit
		sl   []bit.Bit
	)
	out := []entry{
		{"layer3.DecodeServiceOptions", "voice LC header", 0, func() { so = layer3.DecodeServiceOptions(soBits) }},
		{"layer3.ServiceOptionsFromByte", "voice LC header", 0, func() { so = layer3.ServiceOptionsFromByte(soByte) }},
		{"layer3.ServiceOptions.Byte", "voice LC header", 0, func() { b = so.Byte() }},
		{"layer3.ServiceOptions.Encode", "voice LC header", 0, func() { bits, err = so.Encode() }},
		{"layer3.ServiceOptions.EncodeBits", "voice LC header, fresh slice", 1, func() { sl, err = so.EncodeBits() }},
		{"layer3.MotorolaServiceOptions", "", 0, func() { so = layer3.MotorolaServiceOptions() }},
		{"layer3.DecodeCdefParms", "", 0, func() { cdef = layer3.DecodeCdefParms(cdefBits) }},
		{"layer3.CdefParms.Encode", "", 0, func() { cdefBits, err = cdef.Encode() }},
		{"layer3.CdefParms.EncodeBits", "fresh slice", 1, func() { sl, err = cdef.EncodeBits() }},
		{"layer3.CdefParms.TXFrequencyHz", "", 0, func() { hz = cdef.TXFrequencyHz() }},
		{"layer3.CdefParms.RXFrequencyHz", "", 0, func() { hz = cdef.RXFrequencyHz() }},
		{"layer3.CdefParms.TXFrequencyMHz", "", 0, func() { mhz = cdef.TXFrequencyMHz() }},
		{"layer3.CdefParms.RXFrequencyMHz", "", 0, func() { mhz = cdef.RXFrequencyMHz() }},
	}
	_, _, _, _, _ = b, hz, mhz, bits, sl
	return out
}
