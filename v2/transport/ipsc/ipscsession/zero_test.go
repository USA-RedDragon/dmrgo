package ipscsession_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func TestZeroValues(t *testing.T) {
	t.Parallel()

	var p ipscsession.Peer
	var m ipscsession.Master
	for _, pk := range [][]byte{ctl(ipsc.OpRegisterRequest, 7), ctl(ipsc.OpRegisterReply, 1), {0x80, 0, 0, 0, 7, 1}, nil} {
		if out := p.Packet(0, masterAddr, pk, nil); len(out) != 0 {
			t.Fatalf("zero Peer took %x", pk)
		}
		if out := m.Packet(0, masterAddr, pk, nil); len(out) != 0 {
			t.Fatalf("zero Master took %x", pk)
		}
	}
	if len(p.Tick(0, nil)) != 0 || len(m.Tick(0, nil)) != 0 || len(p.Close(0, nil)) != 0 || len(m.Close(0, nil)) != 0 {
		t.Fatal("zero machine sent")
	}
	if _, ok := p.Deadline(); ok {
		t.Fatal("zero Peer deadline")
	}
	if _, ok := m.Deadline(); ok {
		t.Fatal("zero Master deadline")
	}
	if p.State() != ipscsession.StateIdle || m.State() != ipscsession.StateIdle || p.Peers() != 0 || m.Peers() != 0 {
		t.Fatal("zero state")
	}
	if got, err := p.Sign([]byte{1}); err != nil || len(got) != 1 {
		t.Fatal("zero Peer signed")
	}
	p.Reset()
	m.Reset()
	var np *ipscsession.Peer
	var nm *ipscsession.Master
	if got, err := np.Sign([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(got) != 1 {
		t.Fatalf("nil Peer Sign %x, %v", got, err)
	}
	if got, err := nm.Sign([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(got) != 1 {
		t.Fatalf("nil Master Sign %x, %v", got, err)
	}
	if got, err := nm.SignFor(7, []byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(got) != 1 {
		t.Fatalf("nil Master SignFor %x, %v", got, err)
	}
}
