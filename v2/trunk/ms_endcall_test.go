package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestMS_EndCallKeepsTheSlot(t *testing.T) {
	t.Parallel()
	for _, late := range []time.Duration{0, time.Millisecond, 45 * time.Millisecond, 60 * time.Millisecond, 130 * time.Millisecond} {
		r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
		a := aloha(t)
		r.feed(a, &idle)
		if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		r.feed(a, &idle)
		out := r.feed(tvGrant(t, 9, 5, true), &idle)
		wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)

		heard := r.now + 30*time.Millisecond
		term := terminator(t, 9)
		r.ms.Burst(heard, &term, layer2.Hint{Slot: enums.Slot2}, nil, r.out[:0])
		other := r.now + 60*time.Millisecond
		r.ms.Burst(other, &term, layer2.Hint{Slot: enums.Slot1}, nil, r.out[:0])

		now := heard + late
		out = r.ms.EndCall(now, r.out[:0])
		first := heard
		for first < now {
			first += 60 * time.Millisecond
		}
		for i := range constants.NMaint {
			if want := first + time.Duration(i)*60*time.Millisecond; out[i].Kind != trunk.OutputBurst || out[i].Due != want || out[i].Slot != enums.Slot2 {
				t.Errorf("EndCall %v after the TS2 burst: P_MAINT %d due %v slot %v, want %v", late, i, out[i].Due, out[i].Slot, want)
			}
		}
	}
}
