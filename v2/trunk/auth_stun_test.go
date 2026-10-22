package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

var (
	_ trunk.Authenticator = (*auth.Key)(nil)
	_ trunk.Authenticator = (*auth.Keys)(nil)
)

func TestStun_AuthKeys(t *testing.T) {
	t.Parallel()
	keyA := auth.Key{K: table615Key}
	keyB := auth.Key{K: table615Key, PSN: 0x010203}
	cfgs := threeMS()
	cfgs[0].Auth = &keyA
	cfgs[1].Auth = &keyB
	cfg := tsccConfig()
	cfg.Auth = &auth.Keys{Entries: []auth.Entry{{Addr: 3120001, Key: keyA}, {Addr: 3120002, Key: keyA}}}
	s := registeredSim(t, cfg, cfgs...)
	a, b := s.ms[0], s.ms[1]

	if err := s.tscc.Stun(s.now, 3120001, false); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.Stun(s.now, 3120002, false); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return len(stuns(s)) == 2 })
	for _, e := range stuns(s) {
		switch e.Address {
		case 3120001:
			if e.Ack != enums.AckReasonMSAccepted || e.Reason != trunk.ReasonNone || !a.ms.Stunned() {
				t.Errorf("A with its key: %+v, stunned %v", e, a.ms.Stunned())
			}
		case 3120002:
			if e.Ack != enums.AckReasonRecipientRefused || e.Reason != trunk.ReasonAuthentication || b.ms.Stunned() {
				t.Errorf("B with another PSN: %+v, stunned %v", e, b.ms.Stunned())
			}
		}
	}

	s.events = nil
	if err := s.tscc.Stun(s.now, 3120001, true); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return len(stuns(s)) == 1 })
	if e := stuns(s); len(e) != 1 || e[0].Ack != enums.AckReasonMSAccepted || a.ms.Stunned() {
		t.Errorf("revive A: %+v, stunned %v", e, a.ms.Stunned())
	}

	s.events = nil
	if err := s.tscc.Kill(s.now, 3120001); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return len(stuns(s)) == 1 })
	if e := stuns(s); len(e) != 1 || e[0].Stun != trunk.StunKill || e[0].Ack != enums.AckReasonMSAccepted || !a.ms.Killed() {
		t.Errorf("kill A: %+v, killed %v", e, a.ms.Killed())
	}
}
