package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildDataBurst(dataType enums.DataType, payload [196]bit.Bit) [33]byte {
	var bits [264]bit.Bit
	copy(bits[:98], payload[:98])
	copy(bits[166:], payload[98:])
	slot := must((&pdu.SlotType{ColorCode: 1, DataType: dataType}).Encode())
	copy(bits[98:108], slot[:10])
	copy(bits[156:166], slot[10:])
	syncVal := int64(enums.SyncBSSourcedData)
	for i := 0; i < 48; i++ {
		bits[108+i] = bit.Bit((syncVal >> (47 - i)) & 1)
	}
	return [33]byte(bit.Pack(bits[:]))
}

func TestBurst_Rate34Data_Full144Bits_RoundTrip(t *testing.T) {
	t.Parallel()

	var want [18]byte
	for i := range want {
		want[i] = byte(0xA0 + i)
	}
	var info [144]bit.Bit
	copy(info[:], bit.Unpack(want[:]))
	raw := buildDataBurst(enums.DataTypeRate34, trellisInfo(info))

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
	if must(b.Encode()) != raw {
		t.Error("Encode() does not reproduce the original burst")
	}
}

// The trellis result of a Rate 3/4 block is in FEC.Payload. The block has
// no PDU-level FEC, so FEC.PDU stays empty and Aggregate counts the trellis
// corrections once.
func TestBurst_Rate34Data_FEC(t *testing.T) {
	t.Parallel()

	var info [144]bit.Bit
	for i := range info {
		info[i] = bit.Bit(i % 3 % 2)
	}
	raw := buildDataBurst(enums.DataTypeRate34, trellisInfo(info))
	raw[2] ^= 0x10

	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b.FEC.Payload.BitsChecked != 196 || b.FEC.Payload.ErrorsCorrected != 1 || b.FEC.Payload.Uncorrectable {
		t.Errorf("FEC.Payload = %+v, want 196 bits checked, 1 error corrected", b.FEC.Payload)
	}
	if b.FEC.PDU != (fec.Result{}) {
		t.Errorf("FEC.PDU = %+v, want empty", b.FEC.PDU)
	}
	if got := b.FEC.Aggregate().ErrorsCorrected; got != 1 {
		t.Errorf("Aggregate().ErrorsCorrected = %d, want 1", got)
	}
	if rt, ok := b.Data.(*pdu.Rate34Data); !ok || string(rt.Data[:]) != string(bit.Pack(info[:])) {
		t.Error("data not corrected")
	}
}

func TestBurst_PIHeader_Decode(t *testing.T) {
	t.Parallel()

	var info [96]bit.Bit
	copy(info[:], bit.Unpack([]byte{0x21, 0x10, 0x02, 0x17, 0x7A, 0xFC, 0x73, 0x00, 0x00, 0x09, 0x0D, 0xDA}))
	raw := buildDataBurst(enums.DataTypePIHeader, encode196Bits(info))

	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	pi, ok := b.Data.(*pdu.PIHeader)
	if !ok {
		t.Fatalf("Data is %T, want *pdu.PIHeader", b.Data)
	}
	if pi.AlgorithmID != 0x21 || pi.KeyID != 0x02 || pi.MessageIndicator != 0x177AFC73 || pi.Destination != 9 {
		t.Errorf("PI header = %+v", pi)
	}
	if pi.DataType() != enums.DataTypePIHeader {
		t.Errorf("DataType = %v, want PI header", pi.DataType())
	}
	if must(b.Encode()) != raw {
		t.Error("Encode() does not reproduce the original burst")
	}
}

func TestBurst_Rate1Data_Full192Bits_RoundTrip(t *testing.T) {
	t.Parallel()

	var want [24]byte
	for i := range want {
		want[i] = byte(0x30 + i)
	}
	info := bit.Unpack(want[:])
	var payload [196]bit.Bit
	copy(payload[:96], info[:96])
	copy(payload[100:], info[96:])
	raw := buildDataBurst(enums.DataTypeRate1, payload)

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
	if must(b.Encode()) != raw {
		t.Error("Encode() does not reproduce the original burst")
	}
}

// trellisInfo returns the 196 rate 3/4 trellis coded bits of info.
func trellisInfo(info [144]bit.Bit) [196]bit.Bit {
	enc := trellis.Encode([18]byte(bit.Pack(info[:])))
	var bits [196]bit.Bit
	copy(bits[:], bit.Unpack(enc[:]))
	return bits
}
