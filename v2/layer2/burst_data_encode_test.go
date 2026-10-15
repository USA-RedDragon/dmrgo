package layer2_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func dataBurst(dt enums.DataType, d layer2.Data) layer2.Burst {
	return layer2.Burst{
		SyncPattern: enums.SyncBSSourcedData,
		IsData:      true,
		HasSlotType: true,
		SlotType:    pdu.SlotType{ColorCode: 1, DataType: dt},
		Data:        d,
	}
}

func reencode(t *testing.T, b *layer2.Burst) *layer2.Burst {
	t.Helper()
	got, err := layer2.NewBurstFromBytes(must(b.Encode()))
	if err != nil {
		t.Fatalf("decode of the encoded burst: %v", err)
	}
	return got
}

func TestBurstEncode_EditedLCGoesOnAir(t *testing.T) {
	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		t.Run(dt.String(), func(t *testing.T) {
			lc := groupLC(enums.FIDStandardized)
			lc.DataType = dt
			lcBits := must(lc.Encode())
			var lcBytes [12]byte
			copy(lcBytes[:], bit.PackBits(lcBits[:]))
			frame := must(layer2.BuildLCDataBurst(lcBytes, dt, 1))

			b := must(layer2.NewBurstFromBytes(frame))
			flc, ok := b.Data.(*pdu.FullLinkControl)
			if !ok {
				t.Fatalf("Data is %T", b.Data)
			}
			flc.GroupVoice.GroupAddress = 9
			flc.UnitToUnit = nil

			got := reencode(t, b)
			if got.FEC.PDU.Uncorrectable || got.FEC.PDU.ErrorsCorrected != 0 || got.FEC.Payload.ErrorsCorrected != 0 {
				t.Fatalf("re-encoded LC does not decode clean: %+v", got.FEC)
			}
			gotLC, ok := got.Data.(*pdu.FullLinkControl)
			if !ok || gotLC.GroupVoice == nil || gotLC.GroupVoice.GroupAddress != 9 || gotLC.GroupVoice.SourceAddress != 3191868 {
				t.Fatalf("decoded LC %s, want group 9", gotLC.String())
			}
		})
	}
}

func TestBurstEncode_HandBuiltData(t *testing.T) {
	lc := groupLC(enums.FIDStandardized)
	csbk := &pdu.CSBK{
		CSBKOpcode:  pdu.CSBKPreamblePDU,
		PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2},
	}
	header := &pdu.DataHeader{
		Format: pdu.FormatUnconfirmed,
		UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			Group:          true,
			SAP:            4,
			FullMessage:    true,
			BlocksToFollow: 3,
		},
	}
	header.UnconfirmedDataHeader.LLIDDestination[23] = 1
	header.UnconfirmedDataHeader.LLIDSource[22] = 1
	r12 := &pdu.Rate12Data{Data: [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}}
	r34 := &pdu.Rate34Data{Data: [18]byte{0xFF, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 0xAA}}
	r1 := &pdu.Rate1Data{Data: [24]byte{0x80, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 0x01}}

	tests := []struct {
		dt    enums.DataType
		data  layer2.Data
		check func(t *testing.T, got layer2.Data)
	}{
		{enums.DataTypeVoiceLCHeader, &lc, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.FullLinkControl)
			if !ok || g.GroupVoice == nil || *g.GroupVoice != *lc.GroupVoice || g.DataType != enums.DataTypeVoiceLCHeader {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeTerminatorWithLC, &lc, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.FullLinkControl)
			if !ok || g.GroupVoice == nil || *g.GroupVoice != *lc.GroupVoice || g.DataType != enums.DataTypeTerminatorWithLC {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeCSBK, csbk, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.CSBK)
			if !ok || g.CSBKOpcode != pdu.CSBKPreamblePDU || g.PreamblePDU == nil || *g.PreamblePDU != *csbk.PreamblePDU {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeDataHeader, header, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.DataHeader)
			if !ok || g.Format != pdu.FormatUnconfirmed || g.UnconfirmedDataHeader == nil || *g.UnconfirmedDataHeader != *header.UnconfirmedDataHeader {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeIdle, &pdu.PRFill{}, func(t *testing.T, got layer2.Data) {
			if _, ok := got.(*pdu.PRFill); !ok {
				t.Errorf("got %T", got)
			}
		}},
		{enums.DataTypeRate12, r12, func(t *testing.T, got layer2.Data) {
			if g, ok := got.(*pdu.Rate12Data); !ok || g.Data != r12.Data {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeRate34, r34, func(t *testing.T, got layer2.Data) {
			if g, ok := got.(*pdu.Rate34Data); !ok || g.Data != r34.Data {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeRate1, r1, func(t *testing.T, got layer2.Data) {
			if g, ok := got.(*pdu.Rate1Data); !ok || g.Data != r1.Data {
				t.Errorf("got %+v", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.dt.String(), func(t *testing.T) {
			b := dataBurst(tt.dt, tt.data)
			got := reencode(t, &b)
			if got.SlotType.DataType != tt.dt || got.SlotType.ColorCode != 1 {
				t.Fatalf("slot type %s", got.SlotType.String())
			}
			if got.FEC.PDU.Uncorrectable {
				t.Fatalf("PDU check failed: %+v", got.FEC)
			}
			tt.check(t, got.Data)
			if again := must(got.Encode()); again != must(b.Encode()) {
				t.Error("decode then Encode changed the burst")
			}
		})
	}
	if lc.DataType != 0 {
		t.Error("Encode changed the DataType of the caller's LC")
	}
}

func TestBurstEncode_IdleKeepsDecodedPayload(t *testing.T) {
	payload := [12]byte{0x12, 0x34}
	frame := must(layer2.BuildLCDataBurst(payload, enums.DataTypeIdle, 1))
	b := must(layer2.NewBurstFromBytes(frame))
	if got := must(b.Encode()); got != frame {
		t.Errorf("decoded idle burst: got %x, want %x", got, frame)
	}
	b.Data = &pdu.PRFill{DataType: enums.DataTypeIdle}
	got := must(layer2.NewBurstFromBytes(must(b.Encode())))
	if p, _, _ := got.BPTCPayload(); !pdu.IsPRFill(bitsOf(p)) {
		t.Errorf("new PR FILL encoded %x, want Table D.2", p)
	}
}

func bitsOf(p [12]byte) [96]bit.Bit {
	var bits [96]bit.Bit
	copy(bits[:], bit.UnpackBits(p[:]))
	return bits
}

func TestBurstEncode_DataErrors(t *testing.T) {
	csbk := &pdu.CSBK{CSBKOpcode: 0x3F}
	tests := []struct {
		name  string
		burst layer2.Burst
		want  string
	}{
		{"NilData", dataBurst(enums.DataTypeCSBK, nil), "no Data"},
		{"CSBKAsRate34", dataBurst(enums.DataTypeRate34, csbk), "cannot carry Data of type *pdu.CSBK"},
		{"LCAsCSBK", dataBurst(enums.DataTypeCSBK, &pdu.FullLinkControl{}), "cannot carry"},
		{"TypedNil", dataBurst(enums.DataTypeCSBK, (*pdu.CSBK)(nil)), "cannot carry"},
		{"ReservedType", dataBurst(enums.DataTypeReserved, csbk), "cannot carry"},
		{"BadField", dataBurst(enums.DataTypeCSBK, &pdu.CSBK{CSBKOpcode: 0x40}), "CSBKOpcode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.burst.Encode()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

// A decoded burst without Data, here a CSBK whose CRC failed, encodes the
// decoded payload, so a bridge can still forward it; a data type that
// carries a different number of information bits is an error.
func TestBurstEncode_DecodedPayloadWithoutData(t *testing.T) {
	frame := must(layer2.BuildLCDataBurst([12]byte{0xDE, 0xAD}, enums.DataTypeCSBK, 1))
	b, err := layer2.NewBurstFromBytes(frame)
	if err == nil || b.Data != nil {
		t.Fatalf("CSBK with a bad CRC: err %v, Data %v", err, b.Data)
	}
	if got := must(b.Encode()); got != frame {
		t.Errorf("got %x, want %x", got, frame)
	}
	for _, dt := range []enums.DataType{enums.DataTypeRate34, enums.DataTypeMBCHeader} {
		b.SlotType.DataType = dt
		if _, err := b.Encode(); err == nil || !strings.Contains(err.Error(), "set Data") {
			t.Errorf("CSBK payload as %s: err %v", dt.String(), err)
		}
	}

	idle := must(layer2.NewBurstFromBytes(must(layer2.BuildLCDataBurst([12]byte{1}, enums.DataTypeIdle, 1))))
	idle.SlotType.DataType = enums.DataTypeCSBK
	if _, err := idle.Encode(); err == nil {
		t.Error("decoded PR FILL as a CSBK: no error")
	}
}
