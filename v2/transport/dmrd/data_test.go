package dmrd_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

type dmrdVector struct {
	hex     string
	seq     uint8
	src     uint32
	dst     uint32
	rpt     uint32
	slot    enums.Slot
	private bool
	frame   dmrd.FrameType
	value   uint8
	stream  uint32
}

func dmrdVectors() []dmrdVector {
	return []dmrdVector{
		{"444d52440030b43c00005b003010afa10000016b20580b4015ec0f6871000d005c4dff57d75df5d361a416203d5235242181850277", 0, 3191868, 91, 3149999, enums.Slot2, false, dmrd.FrameDataSync, 1, 0x16b},
		{"444d52440130b43c00005b003010af900000016b298d0a33fd2bbc43cc28124ac5a7f7d5dd57dfd138d2514ab1ba41155039c90579", 1, 3191868, 91, 3149999, enums.Slot2, false, dmrd.FrameVoiceSync, 0, 0x16b},
		{"444d52440230b43c00005b003010af810000016bcb375d2a6f00ccf547faacd653f730009090038be3fa9874d4cd7a0a1be7a21f8c", 2, 3191868, 91, 3149999, enums.Slot2, false, dmrd.FrameVoice, 1, 0x16b},
		{"444d52440030b43c2febf6003010af660000018820f7370570bee26393b7cda7ddadff57d75df5de6accf9067160278e89420a3c95", 0, 3191868, 3140598, 3149999, enums.Slot1, true, dmrd.FrameDataSync, 6, 0x188},
		{"444d52440130b43c000009003010af1000000005298d0a33fd2bbc43cc28124ac5a755fd7df75f7138d2514ab1ba41155039c90579", 1, 3191868, 9, 3149999, enums.Slot1, false, dmrd.FrameVoiceSync, 0, 5},
		{"444d52440230b43c2febf6003010afc1000000bc8a3e8c022d191619eeaa053b03c73181811093849ea5bcfdf812bca894359e323a", 2, 3191868, 3140598, 3149999, enums.Slot2, true, dmrd.FrameVoice, 1, 0xbc},
	}
}

func TestDecodeDataVectors(t *testing.T) {
	t.Parallel()

	for _, v := range dmrdVectors() {
		p, err := hex.DecodeString(v.hex)
		if err != nil {
			t.Fatal(err)
		}
		d, err := dmrd.DecodeData(p)
		if err != nil {
			t.Fatalf("%s: %v", v.hex[:40], err)
		}
		if d.Seq != v.seq || d.Source != v.src || d.Destination != v.dst || d.Repeater != v.rpt || d.Slot != v.slot ||
			d.Private != v.private || d.Frame != v.frame || d.Value != v.value || d.Stream != v.stream || d.Size != 53 {
			t.Errorf("%s: got %v", v.hex[:40], d.String())
		}
		if !bytes.Equal(d.Burst[:], p[20:53]) {
			t.Errorf("burst %x, want %x", d.Burst, p[20:53])
		}
		out, err := d.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Errorf("encode %x %v, want %x", out, err, p)
		}
	}
}

func TestDataHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    dmrd.Data
		kind layer2.Kind
		pos  enums.VoiceBurstType
		dt   enums.DataType
		hasD bool
	}{
		{dmrd.Data{Frame: dmrd.FrameVoiceSync}, layer2.KindVoice, enums.VoiceBurstA, 0, false},
		{dmrd.Data{Frame: dmrd.FrameVoice, Value: 5}, layer2.KindVoice, enums.VoiceBurstF, 0, false},
		{dmrd.Data{Frame: dmrd.FrameVoice, Value: 9}, layer2.KindVoice, enums.VoiceBurstUnknown, 0, false},
		{dmrd.Data{Frame: dmrd.FrameDataSync, Value: 2}, layer2.KindData, enums.VoiceBurstUnknown, enums.DataTypeTerminatorWithLC, true},
		{dmrd.Data{Frame: dmrd.FrameReserved, Value: 3}, layer2.KindData, enums.VoiceBurstUnknown, enums.DataTypeCSBK, true},
	}
	for _, tt := range tests {
		tt.d.Slot, tt.d.Seq, tt.d.Stream = enums.Slot2, 200, 77
		h := tt.d.Hint()
		if h.Kind != tt.kind || h.VoiceBurst != tt.pos || h.DataType != tt.dt || h.HasDataType != tt.hasD {
			t.Errorf("%v: hint %+v", tt.d.String(), h)
		}
		if h.Slot != enums.Slot2 || h.Seq != 200 || !h.HasSeq || h.SeqBits != 8 || h.Stream != 77 || !h.HasStream {
			t.Errorf("hint %+v", h)
		}
	}
}

func TestDataSetKind(t *testing.T) {
	t.Parallel()

	var d dmrd.Data
	if err := d.SetKind(layer2.KindVoice, enums.VoiceBurstA, 0); err != nil || d.Frame != dmrd.FrameVoiceSync || d.Value != 0 {
		t.Fatalf("A: %v %v %d", err, d.Frame, d.Value)
	}
	if err := d.SetKind(layer2.KindVoice, enums.VoiceBurstE, 0); err != nil || d.Frame != dmrd.FrameVoice || d.Value != 4 {
		t.Fatalf("E: %v %v %d", err, d.Frame, d.Value)
	}
	if err := d.SetKind(layer2.KindData, 0, enums.DataTypeRate34); err != nil || d.Frame != dmrd.FrameDataSync || d.Value != 8 {
		t.Fatalf("data: %v %v %d", err, d.Frame, d.Value)
	}
	for _, k := range []layer2.Kind{layer2.KindUnknown, layer2.KindReverseChannel} {
		if err := d.SetKind(k, 0, 0); err == nil {
			t.Errorf("kind %v: no error", k)
		}
	}
	if err := d.SetKind(layer2.KindVoice, enums.VoiceBurstUnknown, 0); !errors.Is(err, layer2.ErrVoicePosition) {
		t.Errorf("unknown position: %v, want layer2.ErrVoicePosition", err)
	}
	if err := d.SetKind(layer2.KindData, 0, 16); !errors.Is(err, layer2.ErrDataType) {
		t.Errorf("data type 16: %v, want layer2.ErrDataType", err)
	}
}

func TestDataSizes(t *testing.T) {
	t.Parallel()

	d := dmrd.Data{Seq: 9, Source: 1, Destination: 2, Repeater: 3, Slot: enums.Slot1, Stream: 4, BER: 5, RSSI: 70, Size: 55}
	d.UUID[0] = 0xAA
	p, err := d.AppendBinary([]byte{0xFF})
	if err != nil || len(p) != 56 || p[0] != 0xFF || p[54] != 5 || p[55] != 70 {
		t.Fatalf("55: %x %v", p, err)
	}
	got, err := dmrd.DecodeData(p[1:])
	if err != nil || got.BER != 5 || got.RSSI != 70 || got.Size != 55 || got.UUID != [16]byte{} {
		t.Fatalf("decode 55: %v %v", got.String(), err)
	}
	d.Size = 71
	p, err = d.AppendBinary(nil)
	if err != nil || len(p) != 71 || p[55] != 0xAA {
		t.Fatalf("71: %x %v", p, err)
	}
	if got, err := dmrd.DecodeData(p); err != nil || got != d {
		t.Fatalf("decode 71: %v %v", got.String(), err)
	}
	d.Size = 0
	if p, err = d.AppendBinary(nil); err != nil || len(p) != 53 {
		t.Fatalf("0: %d %v", len(p), err)
	}
	d.Size = 54
	if p, err = d.AppendBinary([]byte{1}); err == nil || !bytes.Equal(p, []byte{1}) {
		t.Fatalf("54: %x %v", p, err)
	}
}

func TestDecodeDataErrors(t *testing.T) {
	t.Parallel()

	p, _ := hex.DecodeString(dmrdVectors()[0].hex)
	tests := []struct {
		p    []byte
		want error
	}{
		{p[:52], dmrd.ErrShort},
		{nil, dmrd.ErrShort},
		{append(append([]byte{}, p...), 0), dmrd.ErrLength},
		{append(append([]byte{}, p...), make([]byte, 19)...), dmrd.ErrLength},
		{append([]byte("DMRX"), p[4:]...), dmrd.ErrSignature},
	}
	for _, tt := range tests {
		if _, err := dmrd.DecodeData(tt.p); !errors.Is(err, tt.want) {
			t.Errorf("len %d: %v, want %v", len(tt.p), err, tt.want)
		}
	}
}

func TestDataEncodeRange(t *testing.T) {
	t.Parallel()

	for _, d := range []dmrd.Data{
		{Slot: enums.Slot1, Source: 1 << 24},
		{Slot: enums.Slot1, Destination: 1 << 24},
		{Slot: enums.SlotUnknown},
		{Slot: enums.Slot1, Value: 16},
		{Slot: enums.Slot1, Frame: 4},
	} {
		if p, err := d.AppendBinary([]byte{7}); err == nil || !bytes.Equal(p, []byte{7}) {
			t.Errorf("%v: %x %v", d.String(), p, err)
		}
	}
}
