package hytera_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestFrameHintTakesBurstSlotType(t *testing.T) {
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypeCSBK, enums.DataTypeDataHeader, enums.DataTypeRate12, enums.DataTypeRate34} {
		raw, err := layer2.BuildDataBurst([12]byte{0xBD, 0, 1, 2, 3, 4, 5, 6, 7, 8}, dt, 1, layer2.SyncSourceBS)
		if dt == enums.DataTypeRate34 {
			raw, err = layer2.BuildRate34DataBurst([18]byte{1, 2, 3}, 1, layer2.SyncSourceBS)
		}
		if err != nil {
			t.Fatal(err)
		}
		f := hytera.Frame{Slot: enums.Slot1, SlotType: hytera.SlotTypeTerminator, ColorCode: 1, Burst: raw}
		if h := f.Hint(0); h.Kind != layer2.KindData || !h.HasDataType || h.DataType != dt {
			t.Errorf("%v labelled as a terminator: %+v", dt, h)
		}
	}

	raw, err := layer2.BuildDataBurst([12]byte{1, 2, 3}, enums.DataTypeCSBK, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	good := raw
	for m := 1; m < 1<<10; m++ {
		raw = good
		for i := range 10 {
			if m>>i&1 == 1 {
				raw[12+i/8] ^= 0x20 >> (i % 8)
			}
		}
		raw[19] ^= 0x0F
		if raw.SlotType().FEC.Uncorrectable {
			break
		}
	}
	if st := raw.SlotType(); !st.FEC.Uncorrectable {
		t.Fatalf("slot type still decodes: %+v", st)
	}
	f := hytera.Frame{SlotType: hytera.SlotTypeCSBK, Burst: raw}
	if h := f.Hint(0); h.DataType != enums.DataTypeCSBK {
		t.Errorf("damaged slot type: %+v", h)
	}
	f = hytera.Frame{SlotType: hytera.SlotTypeTerminator}
	if h := f.Hint(0); h.DataType != enums.DataTypeTerminatorWithLC {
		t.Errorf("empty burst: %+v", h)
	}
}
