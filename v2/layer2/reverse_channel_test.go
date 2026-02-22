package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildRCEmbeddedData constructs valid 32-bit embedded data for RC with
// a given 4-bit payload, encoded through Single Burst BPTC with odd parity.
func buildRCEmbeddedData(payload byte) [32]bit.Bit {
	var infoBits [11]bit.Bit
	copy(infoBits[0:4], bit.BitsFromUint8(payload&0x0F, 4))
	crcVal := crc.CalculateCRC7(infoBits[:4]) ^ pdu.CRC7MaskRC
	for i := 0; i < 7; i++ {
		infoBits[4+i] = bit.Bit((crcVal >> (6 - i)) & 1)
	}
	return bptc.EncodeSingleBurstBPTC(infoBits, true)
}

func TestRC_DecodeFromEmbeddedData_RoundTrip(t *testing.T) {
	t.Parallel()
	for payload := byte(0); payload < 16; payload++ {
		embData := buildRCEmbeddedData(payload)
		rc, fecResult := layer2.DecodeRCFromEmbeddedData(embData)
		if fecResult.Uncorrectable {
			t.Errorf("payload=%d: unexpected uncorrectable error", payload)
			continue
		}
		if byte(rc.RCCommand) != payload {
			t.Errorf("payload=%d: got %d", payload, rc.RCCommand)
		}
	}
}

func TestRC_EncodeRCToEmbeddedData_RoundTrip(t *testing.T) {
	t.Parallel()
	rc := pdu.ReverseChannel{RCCommand: enums.RCCommandFromInt(10)}
	embData := layer2.EncodeRCToEmbeddedData(&rc)

	decoded, fecResult := layer2.DecodeRCFromEmbeddedData(embData)
	if fecResult.Uncorrectable {
		t.Fatal("round-trip failed: uncorrectable")
	}
	if int(decoded.RCCommand) != 10 {
		t.Errorf("expected command=10, got %d", decoded.RCCommand)
	}
}

func TestRC_CombinedFEC(t *testing.T) {
	t.Parallel()
	embData := buildRCEmbeddedData(3)
	_, fecResult := layer2.DecodeRCFromEmbeddedData(embData)

	// BitsChecked should include BPTC (32) + CRC (11)
	expectedBitsChecked := 32 + 11
	if fecResult.BitsChecked != expectedBitsChecked {
		t.Errorf("BitsChecked: got %d, want %d", fecResult.BitsChecked, expectedBitsChecked)
	}
}

func TestRC_BurstDetection_PI1_LCSS0(t *testing.T) {
	t.Parallel()

	var bitData [264]bit.Bit

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.SingleFragmentLCorCSBK,
	}
	embBits := pdu.EncodeEmbeddedSignalling(&embSig)
	copy(bitData[108:116], embBits[:8])
	copy(bitData[148:156], embBits[8:16])

	rcData := buildRCEmbeddedData(9)
	copy(bitData[116:148], rcData[:])

	packed := bit.PackBits264(bitData)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if !burst.HasEmbeddedSignalling {
		t.Fatal("expected HasEmbeddedSignalling=true")
	}
	if !burst.HasReverseChannel {
		t.Fatal("expected HasReverseChannel=true")
	}
	if burst.ReverseChannel == nil {
		t.Fatal("ReverseChannel should not be nil")
	}
	if int(burst.ReverseChannel.RCCommand) != 9 {
		t.Errorf("expected RCCommand=9, got %d", burst.ReverseChannel.RCCommand)
	}
}

func TestRC_BurstDetection_PI0_NoRC(t *testing.T) {
	t.Parallel()
	var bitData [264]bit.Bit

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: false,
		LCSS:                               enums.SingleFragmentLCorCSBK,
	}
	embBits := pdu.EncodeEmbeddedSignalling(&embSig)
	copy(bitData[108:116], embBits[:8])
	copy(bitData[148:156], embBits[8:16])

	packed := bit.PackBits264(bitData)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if burst.HasReverseChannel {
		t.Error("should NOT have HasReverseChannel when PI=0")
	}
}

func TestRC_BurstDetection_PI1_LCSS_NonSingle_NoRC(t *testing.T) {
	t.Parallel()
	var bitData [264]bit.Bit

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.FirstFragmentLC,
	}
	embBits := pdu.EncodeEmbeddedSignalling(&embSig)
	copy(bitData[108:116], embBits[:8])
	copy(bitData[148:156], embBits[8:16])

	packed := bit.PackBits264(bitData)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if burst.HasReverseChannel {
		t.Error("should NOT have HasReverseChannel when LCSS != SingleFragment")
	}
}

func TestRC_MsSourcedRcSync_HasEmbeddedSignalling(t *testing.T) {
	t.Parallel()
	var bitData [264]bit.Bit

	syncVal := int64(enums.MsSourcedRcSync)
	for i := 0; i < 48; i++ {
		bitData[108+i] = bit.Bit((syncVal >> (47 - i)) & 1)
	}

	packed := bit.PackBits264(bitData)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		_ = err
	}

	if burst.SyncPattern != enums.MsSourcedRcSync {
		t.Errorf("expected SyncPattern=MsSourcedRcSync, got %v", burst.SyncPattern)
	}
}

func TestRC_BurstFECStats_RC(t *testing.T) {
	t.Parallel()
	var bitData [264]bit.Bit

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.SingleFragmentLCorCSBK,
	}
	embBits := pdu.EncodeEmbeddedSignalling(&embSig)
	copy(bitData[108:116], embBits[:8])
	copy(bitData[148:156], embBits[8:16])

	rcData := buildRCEmbeddedData(0)
	copy(bitData[116:148], rcData[:])

	packed := bit.PackBits264(bitData)
	burst, _ := layer2.NewBurstFromBytes(packed)

	if burst.FEC.RC.BitsChecked == 0 {
		t.Error("FEC.RC.BitsChecked should be > 0 for RC burst")
	}
	agg := burst.FEC.Aggregate()
	if agg.BitsChecked < burst.FEC.RC.BitsChecked {
		t.Error("Aggregate should include RC bits")
	}
}

func TestRC_ToString(t *testing.T) {
	t.Parallel()
	var bitData [264]bit.Bit

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.SingleFragmentLCorCSBK,
	}
	embBits := pdu.EncodeEmbeddedSignalling(&embSig)
	copy(bitData[108:116], embBits[:8])
	copy(bitData[148:156], embBits[8:16])

	rcData := buildRCEmbeddedData(5)
	copy(bitData[116:148], rcData[:])

	packed := bit.PackBits264(bitData)
	burst, _ := layer2.NewBurstFromBytes(packed)

	s := burst.ToString()
	if s == "" {
		t.Error("ToString should not be empty")
	}
}
