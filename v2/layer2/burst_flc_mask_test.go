package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func burstInfoBits(raw [33]byte) [96]bit.Bit {
	all := bit.UnpackBytesToBits264(raw)
	var dataBits [196]bit.Bit
	copy(dataBits[:98], all[:98])
	copy(dataBits[98:], all[166:])
	info, _ := bptc.Decode196(dataBits)
	return info
}

// Real Voice LC Headers and Terminators carry the Data Type CRC Mask in their
// RS(12,9) parity (ETSI TS 102 361-1 B.3.12). The RS checks depend on a
// correct RS(12,9) codec.
func TestBurst_FullLCCapturesUseDataTypeMask(t *testing.T) {
	seen := map[enums.DataType]int{}
	for _, file := range []string{"testdata/parrot_kerchunk.bin", "testdata/voice.bin"} {
		for i, raw := range loadBursts(t, file) {
			b, err := layer2.NewBurstFromBytes(raw)
			if err != nil || !b.IsData {
				continue
			}
			dt := b.SlotType.DataType
			if dt != enums.DataTypeVoiceLCHeader && dt != enums.DataTypeTerminatorWithLC {
				continue
			}
			seen[dt]++
			flc, ok := b.Data.(*pdu.FullLinkControl)
			if !ok {
				t.Fatalf("%s burst %d: Data is %T", file, i, b.Data)
			}
			if flc.DataType() != dt {
				t.Errorf("%s burst %d: DataType = %v, want %v", file, i, flc.DataType(), dt)
			}
			if b.FEC.PDU.Uncorrectable || b.FEC.PDU.ErrorsCorrected != 0 {
				t.Errorf("%s burst %d (%s): RS = %+v, want a clean codeword",
					file, i, dt.String(), b.FEC.PDU)
			}
			info := burstInfoBits(raw)
			if got := must(flc.Encode()); got != info {
				t.Errorf("%s burst %d (%s): re-encoded LC %x, captured %x",
					file, i, dt.String(), bit.PackBits(got[:]), bit.PackBits(info[:]))
			}
		}
	}
	if seen[enums.DataTypeVoiceLCHeader] == 0 || seen[enums.DataTypeTerminatorWithLC] == 0 {
		t.Fatalf("captures did not contain both LC types: %v", seen)
	}
}
