package layer2_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func dataBurst(dt enums.DataType, d layer2.Data) layer2.Burst {
	return layer2.Burst{
		SyncPattern: enums.SyncBSSourcedData,
		Kind:        layer2.KindData,
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
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		t.Run(dt.String(), func(t *testing.T) {
			t.Parallel()

			lc := groupLC(enums.FIDStandardized)
			lc.BurstDataType = dt
			frame := must(lcBurst(must(lc.Encode()), dt, 1))

			b := must(layer2.NewBurstFromBytes(frame))
			flc, ok := b.Data.(*pdu.FullLinkControl)
			if !ok {
				t.Fatalf("Data is %T", b.Data)
			}
			gv, ok := flc.AsGroupVoice()
			if !ok {
				t.Fatalf("decoded LC %s is not a group voice LC", flc.String())
			}
			gv.GroupAddress = 9
			mustSet(flc.SetGroupVoice(gv))

			got := reencode(t, b)
			if got.FEC.PDU.Uncorrectable || got.FEC.PDU.ErrorsCorrected != 0 || got.FEC.Payload.ErrorsCorrected != 0 {
				t.Fatalf("re-encoded LC does not decode clean: %+v", got.FEC)
			}
			gotLC, ok := got.Data.(*pdu.FullLinkControl)
			if !ok {
				t.Fatalf("Data is %T", got.Data)
			}
			if g, gok := gotLC.AsGroupVoice(); !gok || g.GroupAddress != 9 || g.SourceAddress != 3191868 {
				t.Fatalf("decoded LC %s, want group 9", gotLC.String())
			}
		})
	}
}

func TestBurstEncode_HandBuiltData(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	header0, term := lc, lc
	header0.BurstDataType = enums.DataTypeVoiceLCHeader
	term.BurstDataType = enums.DataTypeTerminatorWithLC
	wantGV, _ := lc.AsGroupVoice()
	wantPre := pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2}
	csbk := &pdu.CSBK{}
	mustSet(csbk.SetPreamble(wantPre))
	wantU := pdu.UnconfirmedDataHeader{
		Group:           true,
		SAP:             4,
		FullMessageFlag: true,
		BlocksToFollow:  3,
	}
	wantU.LLIDDestination = 1 << 23
	wantU.LLIDSource = 1 << 22
	header := &pdu.DataHeader{}
	mustSet(header.SetUnconfirmed(wantU))
	r12 := &pdu.Rate12Data{Data: [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}}
	r34 := &pdu.Rate34Data{Data: [18]byte{0xFF, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 0xAA}}
	r1 := &pdu.Rate1Data{Data: [24]byte{0x80, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 0x01}}

	tests := []struct {
		dt    enums.DataType
		data  layer2.Data
		check func(t *testing.T, got layer2.Data)
	}{
		{enums.DataTypeVoiceLCHeader, &header0, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.FullLinkControl)
			if gv, gok := g.AsGroupVoice(); !ok || !gok || gv != wantGV || g.DataType() != enums.DataTypeVoiceLCHeader {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeTerminatorWithLC, &term, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.FullLinkControl)
			if gv, gok := g.AsGroupVoice(); !ok || !gok || gv != wantGV || g.DataType() != enums.DataTypeTerminatorWithLC {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeCSBK, csbk, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.CSBK)
			if p, pok := g.AsPreamble(); !ok || g.Opcode != pdu.CSBKPreamble || !pok || p != wantPre {
				t.Errorf("got %+v", got)
			}
		}},
		{enums.DataTypeDataHeader, header, func(t *testing.T, got layer2.Data) {
			g, ok := got.(*pdu.DataHeader)
			if u, uok := g.AsUnconfirmed(); !ok || g.Format() != pdu.FormatUnconfirmed || !uok || u != wantU {
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
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.dt.String(), func(t *testing.T) {
			t.Parallel()

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
}

func TestBurstEncode_FullLCDataTypeMustMatchSlotType(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	lc.BurstDataType = enums.DataTypeVoiceLCHeader
	b := dataBurst(enums.DataTypeTerminatorWithLC, &lc)
	if _, err := b.Encode(); err == nil || !strings.Contains(err.Error(), "Data of another data type") {
		t.Errorf("Encode of a Voice LC Header LC in a terminator burst: %v, want a data type error", err)
	}
}

func TestBuildBurst_TypedNilData(t *testing.T) {
	t.Parallel()

	for _, d := range []layer2.Data{
		(*pdu.FullLinkControl)(nil), (*pdu.CSBK)(nil), (*pdu.PIHeader)(nil), (*pdu.DataHeader)(nil),
		(*pdu.MBCHeader)(nil), (*pdu.MBCContinuation)(nil), (*pdu.CGAPContinuation)(nil),
		(*pdu.MVAPContinuation)(nil), (*pdu.BCAPContinuation)(nil), (*pdu.VNAPContinuation)(nil),
		(*pdu.UnifiedSingleBlockData)(nil), (*pdu.Rate12Data)(nil), (*pdu.Rate34Data)(nil),
		(*pdu.Rate1Data)(nil), (*pdu.PRFill)(nil),
	} {
		if _, err := layer2.BuildBurst(d, 1, layer2.SyncSourceBS); !errors.Is(err, dmrgo.ErrNilArgument) {
			t.Errorf("BuildBurst(%T(nil)) = %v, want dmrgo.ErrNilArgument", d, err)
		}
	}
}

type customData struct{ info [12]byte }

func (c *customData) String() string            { return "custom" }
func (c *customData) DataType() enums.DataType  { return enums.DataTypeRate12 }
func (c *customData) Encode() ([12]byte, error) { return c.info, nil }

func TestBurstEncode_CustomData(t *testing.T) {
	t.Parallel()

	b := dataBurst(enums.DataTypeRate12, &customData{info: [12]byte{0xDE, 0xAD}})
	if _, err := b.Encode(); err == nil || !strings.Contains(err.Error(), "not a PDU type") {
		t.Errorf("Encode of a custom Data: %v, want a not a PDU type error", err)
	}
	if _, err := layer2.BuildBurst(b.Data, 1, layer2.SyncSourceBS); err == nil || !strings.Contains(err.Error(), "not a PDU type") {
		t.Errorf("BuildBurst of a custom Data: %v, want a not a PDU type error", err)
	}
}

func TestBurstEncode_IdleKeepsDecodedPayload(t *testing.T) {
	t.Parallel()

	payload := [12]byte{0x12, 0x34}
	frame := must(lcBurst(payload, enums.DataTypeIdle, 1))
	b := must(layer2.NewBurstFromBytes(frame))
	if got := must(b.Encode()); got != frame {
		t.Errorf("decoded idle burst: got %x, want %x", got, frame)
	}
	fresh := layer2.Burst{Kind: layer2.KindData, SyncPattern: b.SyncPattern, SlotType: b.SlotType, Data: &pdu.PRFill{}}
	raw := must(fresh.Encode())
	if p, _ := raw.BPTCPayload(); !pdu.IsPRFill(p) {
		t.Errorf("new PR FILL encoded %x, want Table D.2", p)
	}
}

func TestBurstEncode_DataErrors(t *testing.T) {
	t.Parallel()

	csbk := &pdu.CSBK{Opcode: 0x3F}
	tests := []struct {
		name  string
		burst layer2.Burst
		want  string
	}{
		{"NilData", dataBurst(enums.DataTypeCSBK, nil), "no Data"},
		{"CSBKAsRate34", dataBurst(enums.DataTypeRate34, csbk), "cannot carry Data of another data type"},
		{"LCAsCSBK", dataBurst(enums.DataTypeCSBK, &pdu.FullLinkControl{}), "cannot carry"},
		{"TypedNil", dataBurst(enums.DataTypeCSBK, (*pdu.CSBK)(nil)), "nil argument"},
		{"ReservedType", dataBurst(enums.DataTypeReserved, csbk), "cannot carry"},
		{"BadField", dataBurst(enums.DataTypeCSBK, &pdu.CSBK{Opcode: 0x40}), "CSBK.Opcode"},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.burst.Encode()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestBurstEncode_DecodedPayloadWithoutData(t *testing.T) {
	t.Parallel()

	frame := must(lcBurst([12]byte{0xDE, 0xAD}, enums.DataTypeCSBK, 1))
	b, err := layer2.NewBurstFromBytes(frame)
	if err != nil || b.Data == nil || !b.FEC.PDU.Uncorrectable {
		t.Fatalf("CSBK with a bad CRC: err %v, Data %v, FEC %+v", err, b.Data, b.FEC.PDU)
	}
	if got := must(b.Encode()); got != frame {
		t.Errorf("unchanged Data: got %x, want %x", got, frame)
	}
	csbk, ok := b.Data.(*pdu.CSBK)
	if !ok {
		t.Fatalf("Data is %T, want *pdu.CSBK", b.Data)
	}
	csbk.LastBlock = !csbk.LastBlock
	if again := must(layer2.NewBurstFromBytes(must(b.Encode()))); again.FEC.PDU.Uncorrectable {
		t.Error("changed Data was not coded with a new CRC")
	}
	csbk.LastBlock = !csbk.LastBlock
	b.Data = nil
	if got := must(b.Encode()); got != frame {
		t.Errorf("nil Data: got %x, want %x", got, frame)
	}
	for _, dt := range []enums.DataType{enums.DataTypeRate34, enums.DataTypeMBCHeader} {
		b.SlotType.DataType = dt
		if _, err := b.Encode(); err == nil || !strings.Contains(err.Error(), "set Data") {
			t.Errorf("CSBK payload as %s: err %v", dt.String(), err)
		}
	}

	idle := must(layer2.NewBurstFromBytes(must(lcBurst([12]byte{1}, enums.DataTypeIdle, 1))))
	idle.SlotType.DataType = enums.DataTypeCSBK
	if _, err := idle.Encode(); err == nil {
		t.Error("decoded PR FILL as a CSBK: no error")
	}
}
