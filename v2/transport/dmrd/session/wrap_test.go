package session_test

import (
	"math"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func TestTimersDoNotWrap(t *testing.T) {
	t.Parallel()

	now := time.Duration(math.MaxInt64) - time.Second
	p, _ := session.NewPeer(&session.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1234, Config: mmdvmConfig(1234), Backoff: time.Duration(math.MaxInt64)})
	if out := p.Tick(now, nil); len(out) != 1 {
		t.Fatalf("first tick %v", kinds(out))
	}
	if at, _ := p.Deadline(); at < now {
		t.Fatalf("peer deadline %v before now", at)
	}
	if out := p.Tick(now, nil); len(out) != 0 {
		t.Fatalf("second tick %v", kinds(out))
	}
	p.Packet(now, mmdvmCtl(dmrd.CommandMSTNAK, 1234), nil)
	if at, _ := p.Deadline(); at < now {
		t.Fatalf("wait %v before now", at)
	}
	m, _ := session.NewMaster(&session.MasterConfig{Dialect: dmrd.DialectMMDVM})
	m.Packet(now, ctl(dmrd.CommandRPTL, 5), nil)
	if out := m.Tick(now, nil); len(out) != 0 {
		t.Fatalf("master timed out at once %v", kinds(out))
	}
	b, _ := session.NewBridge(&session.BridgeConfig{Key: []byte("k"), Enhanced: true})
	b.Tick(now, nil)
	if out := b.Tick(now, nil); len(out) != 0 {
		t.Fatalf("bridge sent again %v", kinds(out))
	}
}
