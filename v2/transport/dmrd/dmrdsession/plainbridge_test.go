package dmrdsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestPlainBridgeLiveness(t *testing.T) {
	t.Parallel()

	s := time.Second
	a, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("k")})
	b, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("k"), Timeout: 30 * s})
	sq := dmrd.BridgeControl{Command: dmrd.CommandBCSQ, TalkGroup: 91, Stream: 7}
	p, _ := sq.AppendBinary(nil)
	if err := a.SignInPlace(p); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Deadline(); ok || b.State() != dmrdsession.StateIdle {
		t.Fatalf("before a packet: state %v", b.State())
	}
	sameKinds(t, kinds(b.Packet(0, p, nil)), dmrdsession.OutputUp, dmrdsession.OutputMessage)
	if b.State() != dmrdsession.StateRunning {
		t.Fatalf("state %v after a good packet", b.State())
	}
	if at, ok := b.Deadline(); !ok || at != 30*s {
		t.Fatalf("deadline %v %v", at, ok)
	}
	sameKinds(t, kinds(b.Packet(10*s, p, nil)), dmrdsession.OutputMessage)
	if out := b.Tick(39*s, nil); len(out) != 0 {
		t.Fatalf("early tick %v", kinds(out))
	}
	out := b.Tick(40*s, nil)
	if len(out) != 1 || out[0].Kind != dmrdsession.OutputDown || out[0].Reason != dmrdsession.ReasonTimeout || out[0].State != dmrdsession.StateRunning {
		t.Fatalf("timeout %+v", out)
	}
	if _, ok := b.Deadline(); ok || b.State() != dmrdsession.StateIdle {
		t.Fatalf("after the timeout: state %v", b.State())
	}
	sameKinds(t, kinds(b.Packet(41*s, p, nil)), dmrdsession.OutputUp, dmrdsession.OutputMessage)
}
