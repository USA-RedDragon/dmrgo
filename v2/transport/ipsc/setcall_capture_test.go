package ipsc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestXPR8400SetCallPicksTheOpcode(t *testing.T) {
	t.Parallel()

	seen := map[ipsc.BurstType]int{}
	for _, c := range readXPR(t) {
		switch ipsc.Opcode(c.p[0]) {
		case ipsc.OpGroupVoice, ipsc.OpPrivateVoice, ipsc.OpGroupData, ipsc.OpPrivateData:
		default:
			continue
		}
		u, err := ipsc.DecodeUser(c.p)
		if err != nil {
			t.Fatalf("%x: %v", c.p, err)
		}
		switch u.Burst.Type() {
		case 0x00, ipsc.BurstSyncLost, 0x26:
			continue
		default:
		}
		want := u.Opcode
		group, target, source := u.Call()
		u.Opcode = 0
		if err := u.SetCall(group, target, source); err != nil {
			t.Fatal(err)
		}
		seen[u.Burst.Type()]++
		if u.Opcode != want {
			t.Errorf("burst type %d: SetCall on a User with no opcode set %v, the repeater sent %v", u.Burst.Type(), u.Opcode, want)
		}
	}
	if seen[0x01] == 0 || seen[0x02] == 0 || seen[ipsc.BurstVoice] == 0 || seen[0x06] == 0 {
		t.Fatalf("capture lacks a burst type: %v", seen)
	}
}
