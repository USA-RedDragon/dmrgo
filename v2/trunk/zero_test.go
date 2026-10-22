package trunk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestZeroMachines(t *testing.T) {
	t.Parallel()
	raw := aloha(t)
	h := layer2.Hint{Slot: enums.Slot1}
	var cach [3]byte
	var b layer2.Burst

	var ts trunk.TSCC
	if out := ts.Tick(0, nil); len(out) != 0 {
		t.Errorf("zero TSCC Tick: %d outputs", len(out))
	}
	if out := ts.Inbound(0, 1, &raw, h, nil); len(out) != 0 {
		t.Errorf("zero TSCC Inbound: %d outputs", len(out))
	}
	if _, ok := ts.Deadline(); ok {
		t.Error("zero TSCC has a Deadline")
	}
	var a trunk.Appended
	for name, err := range map[string]error{
		"Stun":          ts.Stun(0, 1, false),
		"Kill":          ts.Kill(0, 1),
		"SendStatus":    ts.SendStatus(0, false, 1, 2, 3),
		"Interrupt":     ts.Interrupt(0, 10, enums.Slot1, enums.RCCommand(0)),
		"SendShortData": ts.SendShortData(0, false, 1, &a),
		"PollShortData": ts.PollShortData(0, 1, 1),
		"MassRegister":  ts.MassRegister(1, 1, 1),
	} {
		if !errors.Is(err, dmrgo.ErrZeroValue) {
			t.Errorf("zero TSCC %s = %v", name, err)
		}
	}

	var ms trunk.MS
	if out := ms.Burst(0, &raw, h, &cach, nil); len(out) != 0 {
		t.Errorf("zero MS Burst: %d outputs", len(out))
	}
	if out := ms.Decoded(0, &b, h, &cach, nil); len(out) != 0 {
		t.Errorf("zero MS Decoded: %d outputs", len(out))
	}
	if out := ms.PowerOff(0, nil); len(out) != 0 {
		t.Errorf("zero MS PowerOff: %d outputs", len(out))
	}
	_, ierr := ms.Interrupt(0, false, nil)
	for name, err := range map[string]error{
		"Call":       ms.Call(0, true, 9, layer3.VoiceServiceOptions{}),
		"SendStatus": ms.SendStatus(0, false, 1, 3),
		"Interrupt":  ierr,
	} {
		if !errors.Is(err, dmrgo.ErrZeroValue) {
			t.Errorf("zero MS %s = %v", name, err)
		}
	}

	var f trunk.Follower
	var cp trunk.CapacityPlusFollower
	if out := f.Burst(0, &raw, h, &cach, nil); len(out) != 0 {
		t.Errorf("zero Follower Burst: %d outputs", len(out))
	}
	if out := cp.Burst(0, &raw, h, &cach, nil); len(out) != 0 {
		t.Errorf("zero CapacityPlusFollower Burst: %d outputs", len(out))
	}
	if out := f.Tick(0, nil); len(out) != 0 {
		t.Errorf("zero Follower Tick: %d outputs", len(out))
	}
}
