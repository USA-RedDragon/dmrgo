package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestMS_ShortDataAckInTheSlotOfTheUDT(t *testing.T) {
	t.Parallel()
	r := onTSCC(t, nil)
	var a trunk.Appended
	if err := a.SetText(enums.UDTUTF16BE, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	in := download(t, pdu.UDTHeader{LLIDDestination: msAddr, LLIDSource: peer}, &a)
	out := make([]trunk.Output, 0, trunk.MaxOutputs*len(in))
	for _, b := range in {
		r.now += 60 * time.Millisecond
		out = append(out, r.ms.Burst(r.now, &b, layer2.Hint{Slot: enums.Slot2}, &idle, nil)...)
	}
	wantKinds(t, "UDT to the MS on TS2", out, trunk.OutputUDT, trunk.OutputBurst)
	if o := out[1]; o.Slot != enums.Slot2 || o.Hint.Slot != enums.Slot2 || o.Due != r.now+60*time.Millisecond {
		t.Errorf("C_ACKU on %v (hint %v) due %v, want TS2 due %v", o.Slot, o.Hint.Slot, o.Due, r.now+60*time.Millisecond)
	}
}
