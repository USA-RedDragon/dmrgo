package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// piHeaderVector is a captured PI header (ok-dmrlib test vector) with a
// valid CRC-CCITT under mask 0x6969.
func piHeaderVector() [96]bit.Bit {
	var bits [96]bit.Bit
	copy(bits[:], bit.Unpack([]byte{0x21, 0x10, 0x02, 0x17, 0x7A, 0xFC, 0x73, 0x00, 0x00, 0x09, 0x0D, 0xDA}))
	return bits
}

func TestPIHeader_DecodeVector(t *testing.T) {
	t.Parallel()
	pi, fecResult := pdu.DecodePIHeader(piHeaderVector())
	if fecResult.Uncorrectable {
		t.Fatal("valid PI header reported uncorrectable")
	}
	if pi.AlgorithmID != 0x21 || pi.MFID != 0x10 || pi.KeyID != 0x02 {
		t.Errorf("ALGID/MFID/KeyID = %02X/%02X/%02X, want 21/10/02", pi.AlgorithmID, pi.MFID, pi.KeyID)
	}
	if pi.MessageIndicator != 0x177AFC73 {
		t.Errorf("MessageIndicator = %08X, want 177AFC73", pi.MessageIndicator)
	}
	if pi.Destination != 9 {
		t.Errorf("Destination = %d, want 9", pi.Destination)
	}
	if must(pi.Encode()) != piHeaderVector() {
		t.Error("re-encode does not match the vector")
	}
	if pi.DataType() != enums.DataTypePIHeader {
		t.Error("DataType mismatch")
	}
	if pi.String() == "" {
		t.Error("String should not be empty")
	}
}

func TestPIHeader_CRCMask(t *testing.T) {
	t.Parallel()
	bits := piHeaderVector()
	// Removing the 0x6969 mask must make the CRC fail.
	for i, m := range bit.Unpack([]byte{0x69, 0x69}) {
		bits[80+i] ^= m
	}
	if _, fecResult := pdu.DecodePIHeader(bits); !fecResult.Uncorrectable {
		t.Error("PI header CRC without the 0x6969 mask should fail")
	}
}

func TestPIHeader_RoundTrip(t *testing.T) {
	t.Parallel()
	original := pdu.PIHeader{AlgorithmID: 0x25, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 0xFFFFFE}
	decoded, fecResult := pdu.DecodePIHeader(must(original.Encode()))
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if decoded.AlgorithmID != original.AlgorithmID || decoded.MFID != original.MFID ||
		decoded.KeyID != original.KeyID || decoded.MessageIndicator != original.MessageIndicator ||
		decoded.Destination != original.Destination {
		t.Errorf("round trip = %+v, want %+v", decoded, original)
	}
}

// Raw holds the 80 bits before the CRC for non-DMRA layouts, and encoding
// uses the named fields only.
func TestPIHeader_Raw(t *testing.T) {
	t.Parallel()
	vector := piHeaderVector()
	pi, _ := pdu.DecodePIHeader(vector)
	if [80]bit.Bit(vector[:80]) != pi.Raw {
		t.Errorf("Raw = %x, want %x", bit.Pack(pi.Raw[:]), bit.Pack(vector[:80]))
	}
	pi.Raw = [80]bit.Bit{}
	if must(pi.Encode()) != vector {
		t.Error("PIHeader.Encode should ignore Raw")
	}
}
