package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestRepeater_ForeignColorCodeVoice(t *testing.T) {
	t.Parallel()
	rf := bs.Source{RF: true}
	for _, decoded := range []bool{false, true} {
		var scratch *layer2.Burst
		if decoded {
			scratch = layer2.NewBurst()
		}
		r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
		out := runWith(t, r, call(t, 2, 91, 3120001, 3, enums.Slot1, rf, 0), 1500*time.Millisecond, scratch)
		if len(filter(out, bs.OutputKey)) != 0 || r.State() != bs.StateHibernating || r.SlotState(enums.Slot1) != bs.SlotIdle {
			t.Errorf("decoded %v: a CC 2 call keyed a CC 1 repeater: state %v, slot 1 %v", decoded, r.State(), r.SlotState(enums.Slot1))
		}

		r = newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, CallHang: 3 * time.Second})
		own := call(t, 1, 91, 3120001, 1, enums.Slot1, rf, 0)
		end := own[len(own)-1].at
		out = runWith(t, r, merge(own, call(t, 2, 92, 3120009, 3, enums.Slot1, rf, end+300*time.Millisecond)), end+1500*time.Millisecond, scratch)
		if r.SlotState(enums.Slot1) != bs.SlotCallHang {
			t.Errorf("decoded %v: a CC 2 call in the call hang time of a CC 1 call: slot 1 %v", decoded, r.SlotState(enums.Slot1))
		}
		for _, o := range filter(out, bs.OutputBurst) {
			if o.Slot != enums.Slot1 || o.Due <= end+120*time.Millisecond {
				continue
			}
			if b, err := layer2.NewBurstFromBytes(o.Burst); err != nil || b.Kind != layer2.KindData || b.SlotType.DataType != enums.DataTypeTerminatorWithLC {
				t.Errorf("decoded %v: slot 1 burst at %v in the hang time is not a Terminator with LC", decoded, o.Due)
				break
			}
		}
	}
}
