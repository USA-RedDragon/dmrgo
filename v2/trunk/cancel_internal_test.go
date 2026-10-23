package trunk

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCancelCallEndsTheWaitForTheGrant(t *testing.T) {
	t.Parallel()
	const addr, caller, other = 3120001, 3120002, 3120003
	m, err := NewMS(&MSConfig{Address: addr, NoRegistration: true})
	if err != nil {
		t.Fatal(err)
	}
	h := layer2.Hint{Slot: enums.Slot1}
	for _, c := range []struct {
		kind    enums.ServiceKind
		from    uint32
		pending bool
		outputs int
	}{
		{enums.ServiceKindIndividualVoice, caller, true, 2},
		{enums.ServiceKindCancelCall, other, true, 1},
		{enums.ServiceKindCancelCall, caller, false, 2},
	} {
		a := pdu.AhoyPDU{ServiceKind: uint8(c.kind), TargetAddress: addr, SourceAddress: c.from}
		out := m.ahoyAny(time.Second, &a, &h, nil)
		if m.pending != c.pending || len(out) != c.outputs {
			t.Errorf("after %v from %d: pending %v, %d outputs, want %v and %d", c.kind, c.from, m.pending, len(out), c.pending, c.outputs)
		}
	}
}
