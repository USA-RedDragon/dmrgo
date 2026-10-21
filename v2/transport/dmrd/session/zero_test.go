package session_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func TestZeroValues(t *testing.T) {
	t.Parallel()

	lines := readCapture(t, "hblink3-openbridge.txt")
	pkts := [][]byte{lines[0].p, mmdvmConfig(1), ctl(dmrd.CommandRPTL, 1), ack(1), nil}
	var b session.Bridge
	if err := b.Sign(append([]byte(nil), lines[0].p...)); err == nil {
		t.Fatal("zero Bridge signed")
	}
	var p session.Peer
	var m session.Master
	for _, pk := range pkts {
		if out := b.Packet(0, pk, nil); len(out) != 0 {
			t.Fatalf("zero Bridge took %x", pk)
		}
		if out := p.Packet(0, pk, nil); len(out) != 0 {
			t.Fatalf("zero Peer took %x", pk)
		}
		if out := m.Packet(0, pk, nil); len(out) != 0 {
			t.Fatalf("zero Master took %x", pk)
		}
	}
	if len(b.Tick(0, nil)) != 0 || len(p.Tick(0, nil)) != 0 || len(m.Tick(0, nil)) != 0 {
		t.Fatal("zero machine ticked")
	}
	if _, ok := b.Deadline(); ok {
		t.Fatal("zero Bridge deadline")
	}
	if _, ok := p.Deadline(); ok {
		t.Fatal("zero Peer deadline")
	}
	if _, ok := m.Deadline(); ok {
		t.Fatal("zero Master deadline")
	}
	if len(m.Admit(1, nil, nil)) != 0 || len(m.Refuse(0, nil)) != 0 || len(m.Close(0, nil)) != 0 || len(p.Close(0, nil)) != 0 {
		t.Fatal("zero machine sent")
	}
	p.Reset()
	m.Reset()
	if b.State() != session.StateIdle || p.State() != session.StateIdle || m.State() != session.StateIdle {
		t.Fatal("zero state")
	}
}
