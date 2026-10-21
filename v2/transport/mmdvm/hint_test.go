package mmdvm_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestDMRDataSetHint(t *testing.T) {
	t.Parallel()

	var d mmdvm.DMRData
	for i := range layer2.VoiceBurstsPerSuperframe {
		pos, _ := enums.VoiceBurstAt(i)
		h := layer2.Hint{Slot: enums.Slot2, Kind: layer2.KindVoice, VoiceBurst: pos}
		if err := d.SetHint(h); err != nil {
			t.Fatalf("%v: %v", pos, err)
		}
		if got := d.Hint(); got.Slot != enums.Slot2 || got.Kind != layer2.KindVoice || got.VoiceBurst != pos {
			t.Errorf("%v: %+v", pos, got)
		}
	}
	h := layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeCSBK, HasDataType: true}
	if err := d.SetHint(h); err != nil || d.Control != mmdvm.ControlDataSync|3 || d.Slot != enums.Slot2 {
		t.Fatalf("CSBK: %v %+v", err, d)
	}
	want := d
	for _, h := range []layer2.Hint{
		{}, {Kind: layer2.KindReverseChannel}, {Kind: layer2.Kind(7)}, {Kind: layer2.KindVoice}, {Kind: layer2.KindVoice, VoiceBurst: 8},
		{Kind: layer2.KindData, DataType: enums.DataTypeCSBK}, {Kind: layer2.KindData, DataType: 16, HasDataType: true},
	} {
		if err := d.SetHint(h); err == nil || d != want {
			t.Errorf("%+v: %v %+v", h, err, d)
		}
	}
	if err := d.SetHint(layer2.Hint{Slot: enums.Slot1, SyncLost: true}); err != nil || !d.Lost || d.Command() != mmdvm.CommandDMRLost1 {
		t.Fatalf("lost: %v %+v", err, d)
	}
}

func TestDMRDataSetHintNil(t *testing.T) {
	t.Parallel()

	var d *mmdvm.DMRData
	if err := d.SetHint(layer2.Hint{SyncLost: true}); err == nil {
		t.Fatal("nil DMRData: no error")
	}
}
