package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	trellis34 "github.com/USA-RedDragon/dmrgo/v2/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildDataBurst(dataType elements.DataType, payload [196]bit.Bit) [33]byte {
	var bits [264]bit.Bit
	copy(bits[:98], payload[:98])
	copy(bits[166:], payload[98:])
	slot := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 1, DataType: dataType})
	copy(bits[98:108], slot[:10])
	copy(bits[156:166], slot[10:])
	syncVal := int64(enums.BsSourcedData)
	for i := 0; i < 48; i++ {
		bits[108+i] = bit.Bit((syncVal >> (47 - i)) & 1)
	}
	return bit.PackBits264(bits)
}

func TestBurst_Rate34Data_Full144Bits_RoundTrip(t *testing.T) {
	var want [18]byte
	for i := range want {
		want[i] = byte(0xA0 + i)
	}
	var info [144]bit.Bit
	copy(info[:], bit.UnpackBits(want[:]))
	raw := buildDataBurst(elements.DataTypeRate34, trellis34.New().Encode(info))

	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	rt, ok := b.Data.(*pdu.Rate34Data)
	if !ok {
		t.Fatalf("Data is %T, want *pdu.Rate34Data", b.Data)
	}
	if rt.Data != want {
		t.Errorf("Data = %X, want %X", rt.Data, want)
	}
	if b.Encode() != raw {
		t.Error("Encode() does not reproduce the original burst")
	}
}

func TestBurst_Rate1Data_Full192Bits_RoundTrip(t *testing.T) {
	var want [24]byte
	for i := range want {
		want[i] = byte(0x30 + i)
	}
	info := bit.UnpackBits(want[:])
	var payload [196]bit.Bit
	copy(payload[:96], info[:96])
	copy(payload[100:], info[96:])
	raw := buildDataBurst(elements.DataTypeRate1, payload)

	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	rt, ok := b.Data.(*pdu.Rate1Data)
	if !ok {
		t.Fatalf("Data is %T, want *pdu.Rate1Data", b.Data)
	}
	if rt.Data != want {
		t.Errorf("Data = %X, want %X", rt.Data, want)
	}
	if b.Encode() != raw {
		t.Error("Encode() does not reproduce the original burst")
	}
}
