package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildRCEmbeddedData(payload byte) [4]byte {
	crcVal := crc.CRC7([]byte{payload << 4}, 4) ^ pdu.CRC7MaskRC
	info := uint16(payload&0x0F)<<7 | uint16(crcVal&0x7F)
	var tx [4]byte
	bits.SetField(tx[:], 0, 32, uint64(bptc.EncodeSingleBurst(info, true)))
	return tx
}

func TestRC_DecodeFromEmbeddedData_RoundTrip(t *testing.T) {
	t.Parallel()
	for payload := byte(0); payload < 16; payload++ {
		embData := buildRCEmbeddedData(payload)
		rc := layer2.DecodeEmbeddedRC(embData)
		fecResult := rc.FEC
		if fecResult.Uncorrectable {
			t.Errorf("payload=%d: unexpected uncorrectable error", payload)
			continue
		}
		if byte(rc.RCCommand) != payload {
			t.Errorf("payload=%d: got %d", payload, rc.RCCommand)
		}
	}
}

func TestRC_EncodeEmbeddedRC_RoundTrip(t *testing.T) {
	t.Parallel()
	rc := pdu.ReverseChannel{RCCommand: enums.RCCommand(10)}
	embData, err := layer2.EncodeEmbeddedRC(&rc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded := layer2.DecodeEmbeddedRC(embData)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("round-trip failed: uncorrectable")
	}
	if int(decoded.RCCommand) != 10 {
		t.Errorf("expected command=10, got %d", decoded.RCCommand)
	}
}

func TestRC_EncodeEmbeddedRC_OutOfRange(t *testing.T) {
	t.Parallel()
	rc := pdu.ReverseChannel{RCCommand: enums.RCCommand(0x14)}
	if _, err := layer2.EncodeEmbeddedRC(&rc); err == nil {
		t.Error("expected error for out-of-range RC command")
	}
}

func TestRC_CombinedFEC(t *testing.T) {
	t.Parallel()
	embData := buildRCEmbeddedData(3)
	fecResult := layer2.DecodeEmbeddedRC(embData).FEC

	expectedBitsChecked := 32
	if fecResult.BitsChecked != expectedBitsChecked {
		t.Errorf("BitsChecked: got %d, want %d", fecResult.BitsChecked, expectedBitsChecked)
	}
}

func TestRC_BurstDetection_PI1_LCSS0(t *testing.T) {
	t.Parallel()

	var packed [33]byte

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSSingleFragment,
	}
	embBits := must(embSig.Encode())
	bits.Copy(packed[:], 108, embBits[:], 0, 8)
	bits.Copy(packed[:], 148, embBits[:], 8, 8)

	rcData := buildRCEmbeddedData(9)
	bits.Copy(packed[:], 116, rcData[:], 0, 32)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if !burst.HasEmbeddedSignalling() {
		t.Fatal("expected HasEmbeddedSignalling=true")
	}
	if !burst.HasReverseChannel() {
		t.Fatal("expected HasReverseChannel=true")
	}
	if int(burst.ReverseChannel.RCCommand) != 9 {
		t.Errorf("expected RCCommand=9, got %d", burst.ReverseChannel.RCCommand)
	}
}

func TestRC_BurstDetection_PI0_NoRC(t *testing.T) {
	t.Parallel()
	var packed [33]byte

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: false,
		LCSS:                               enums.LCSSSingleFragment,
	}
	embBits := must(embSig.Encode())
	bits.Copy(packed[:], 108, embBits[:], 0, 8)
	bits.Copy(packed[:], 148, embBits[:], 8, 8)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if burst.HasReverseChannel() {
		t.Error("should NOT have HasReverseChannel when PI=0")
	}
}

func TestRC_BurstDetection_PI1_LCSS_NonSingle_NoRC(t *testing.T) {
	t.Parallel()
	var packed [33]byte

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSFirstFragment,
	}
	embBits := must(embSig.Encode())
	bits.Copy(packed[:], 108, embBits[:], 0, 8)
	bits.Copy(packed[:], 148, embBits[:], 8, 8)

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}

	if burst.HasReverseChannel() {
		t.Error("should NOT have HasReverseChannel when LCSS != SingleFragment")
	}
}

func TestRC_MsSourcedRcSync_HasEmbeddedSignalling(t *testing.T) {
	t.Parallel()
	var packed [33]byte

	bits.SetField64(packed[:], 108, 48, uint64(enums.SyncMSSourcedRC))

	burst, err := layer2.NewBurstFromBytes(packed)
	if err != nil {
		_ = err
	}

	if burst.SyncPattern != enums.SyncMSSourcedRC {
		t.Errorf("expected SyncPattern=MsSourcedRcSync, got %v", burst.SyncPattern)
	}
}

func TestRC_BurstFECStats_RC(t *testing.T) {
	t.Parallel()
	var packed [33]byte

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSSingleFragment,
	}
	embBits := must(embSig.Encode())
	bits.Copy(packed[:], 108, embBits[:], 0, 8)
	bits.Copy(packed[:], 148, embBits[:], 8, 8)

	rcData := buildRCEmbeddedData(0)
	bits.Copy(packed[:], 116, rcData[:], 0, 32)

	burst, _ := layer2.NewBurstFromBytes(packed)

	if burst.FEC.RC.BitsChecked == 0 {
		t.Error("FEC.RC.BitsChecked should be > 0 for RC burst")
	}
	agg := burst.FEC.Aggregate()
	if agg.BitsChecked < burst.FEC.RC.BitsChecked {
		t.Error("Aggregate should include RC bits")
	}
}

func TestRC_String(t *testing.T) {
	t.Parallel()
	var packed [33]byte

	embSig := pdu.EmbeddedSignalling{
		ColorCode:                          0,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSSingleFragment,
	}
	embBits := must(embSig.Encode())
	bits.Copy(packed[:], 108, embBits[:], 0, 8)
	bits.Copy(packed[:], 148, embBits[:], 8, 8)

	rcData := buildRCEmbeddedData(5)
	bits.Copy(packed[:], 116, rcData[:], 0, 32)

	burst, _ := layer2.NewBurstFromBytes(packed)

	s := burst.String()
	if s == "" {
		t.Error("String should not be empty")
	}
}

// ETSI TS 102 361-1 clause 6.4.1, Table E.12
func buildStandaloneRCBurst(cc int, cmd enums.RCCommand) [33]byte {
	var packed [33]byte
	embBits := must((&pdu.EmbeddedSignalling{
		ColorCode: enums.ColorCode(cc),
		LCSS:      enums.LCSSSingleFragment,
	}).Encode())
	rcData, err := layer2.EncodeEmbeddedRC(&pdu.ReverseChannel{RCCommand: cmd})
	if err != nil {
		panic(err)
	}
	bits.Copy(packed[:], 84, embBits[:], 0, 8)
	bits.Copy(packed[:], 92, rcData[:], 0, 16)
	bits.SetField64(packed[:], 108, 48, uint64(enums.SyncMSSourcedRC))
	bits.Copy(packed[:], 156, rcData[:], 16, 16)
	bits.Copy(packed[:], 172, embBits[:], 8, 8)
	return packed
}

func TestRC_StandaloneBurst_RoundTrip(t *testing.T) {
	t.Parallel()
	frame := buildStandaloneRCBurst(7, enums.RCCeaseTransmissionRequest)

	burst, err := layer2.NewBurstFromBytes(frame)
	if err != nil {
		t.Fatal(err)
	}
	if burst.SyncPattern != enums.SyncMSSourcedRC || (burst.Kind == layer2.KindData) {
		t.Fatalf("SyncPattern=%s IsData=%v", burst.SyncPattern.String(), (burst.Kind == layer2.KindData))
	}
	if !burst.HasEmbeddedSignalling() || burst.EmbeddedSignalling.ColorCode != 7 || burst.FEC.EMB.Uncorrectable {
		t.Errorf("EMB: %s", burst.EmbeddedSignalling.String())
	}
	if burst.EmbeddedSignalling.LCSS != enums.LCSSSingleFragment {
		t.Errorf("LCSS = %s", burst.EmbeddedSignalling.LCSS.String())
	}
	if !burst.HasReverseChannel() {
		t.Fatal("standalone RC burst should carry an RC PDU")
	}
	if burst.ReverseChannel.RCCommand != enums.RCCeaseTransmissionRequest || burst.FEC.RC.Uncorrectable {
		t.Errorf("RC: %s", burst.ReverseChannel.String())
	}
	if burst.VoiceSlots != ([3][9]byte{}) {
		t.Error("standalone RC burst should not parse voice")
	}

	if got := must(burst.Encode()); got != frame {
		t.Errorf("encode mismatch\ngot  %x\nwant %x", got, frame)
	}
}

func TestRC_StandaloneBurst_EncodeFromFields(t *testing.T) {
	t.Parallel()
	burst := layer2.Burst{
		Kind:               layer2.KindReverseChannel,
		SyncPattern:        enums.SyncMSSourcedRC,
		EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 7},
		ReverseChannel:     pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand},
	}
	want := buildStandaloneRCBurst(7, enums.RCCeaseTransmissionCommand)
	if got := must(burst.Encode()); got != want {
		t.Errorf("encode mismatch\ngot  %x\nwant %x", got, want)
	}
}
