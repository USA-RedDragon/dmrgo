//go:build !tinygo

package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsMS(t *testing.T) {
	r := newRig(t, &trunk.MSConfig{Address: msAddr, Seed: 3})
	a := aloha(t)
	regOK := ack(t, enums.ReasonRegAccepted)
	wait := ack(t, enums.ReasonWait)
	grant := tvGrant(t, 9, 5, true)
	allocs := testing.AllocsPerRun(5, func() {
		r.ms.Reset()
		r.feed(a, &idle)
		r.feed(a, &idle)
		r.feed(regOK, &idle)
		_ = r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{})
		r.feed(a, &idle)
		r.feed(wait, &idle)
		r.feed(grant, &idle)
		r.out = r.ms.EndCall(r.now, r.out[:0])
		r.out = r.ms.Tick(r.now+time.Minute, r.out[:0])
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}
