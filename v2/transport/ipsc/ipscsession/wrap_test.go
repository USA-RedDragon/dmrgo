package ipscsession_test

import (
	"math"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func TestTimersDoNotWrap(t *testing.T) {
	t.Parallel()

	now := time.Duration(math.MaxInt64) - time.Second
	p, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr})
	if out := p.Tick(now, nil); len(out) != 1 {
		t.Fatalf("first tick %v", kinds(out))
	}
	if out := p.Tick(now, nil); len(out) != 0 {
		t.Fatalf("second tick %v", kinds(out))
	}
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	m.Packet(now, xprAddr, ctl(ipsc.OpRegisterRequest, 7), nil)
	if out := m.Tick(now, nil); len(out) != 0 || m.Peers() != 1 {
		t.Fatalf("master dropped the peer at once %v", kinds(out))
	}
	if at, _ := m.Deadline(); at < now {
		t.Fatalf("master deadline %v before now", at)
	}
}
