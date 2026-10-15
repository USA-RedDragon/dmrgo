package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// piHeaderVector is a captured PI header (ok-dmrlib test vector) with a
// valid CRC-CCITT under mask 0x6969.
func piHeaderVector() [96]bit.Bit {
	var bits [96]bit.Bit
	copy(bits[:], bit.UnpackBits([]byte{0x21, 0x10, 0x02, 0x17, 0x7A, 0xFC, 0x73, 0x00, 0x00, 0x09, 0x0D, 0xDA}))
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
	if must(pdu.EncodePIHeader(&pi)) != piHeaderVector() {
		t.Error("re-encode does not match the vector")
	}
	pi.DataType = elements.DataTypePIHeader
	if pi.GetDataType() != elements.DataTypePIHeader {
		t.Error("GetDataType mismatch")
	}
	if pi.ToString() == "" {
		t.Error("ToString should not be empty")
	}
}

func TestPIHeader_CRCMask(t *testing.T) {
	t.Parallel()
	bits := piHeaderVector()
	// Removing the 0x6969 mask must make the CRC fail.
	for i, m := range bit.UnpackBits([]byte{0x69, 0x69}) {
		bits[80+i] ^= m
	}
	if _, fecResult := pdu.DecodePIHeader(bits); !fecResult.Uncorrectable {
		t.Error("PI header CRC without the 0x6969 mask should fail")
	}
}

func TestPIHeader_RoundTrip(t *testing.T) {
	t.Parallel()
	original := pdu.PIHeader{AlgorithmID: 0x25, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 0xFFFFFE}
	decoded, fecResult := pdu.DecodePIHeader(must(pdu.EncodePIHeader(&original)))
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if decoded.AlgorithmID != original.AlgorithmID || decoded.MFID != original.MFID ||
		decoded.KeyID != original.KeyID || decoded.MessageIndicator != original.MessageIndicator ||
		decoded.Destination != original.Destination {
		t.Errorf("round trip = %+v, want %+v", decoded, original)
	}
}
