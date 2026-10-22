package ipscsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func ctl(op ipsc.Opcode, id uint32) []byte {
	b, _ := (&ipsc.Control{Opcode: op, ID: id, Mode: 0x6a}).AppendBinary(nil)
	return b
}

var (
	other = ipscsession.Addr{IP: [4]byte{10, 0, 0, 7}, Port: 7}
	moved = ipscsession.Addr{IP: [4]byte{10, 0, 0, 8}, Port: 8}
)

func listedPeer(t *testing.T) *ipscsession.Peer {
	t.Helper()
	p, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr, MaxMissed: 2})
	p.Tick(0, nil)
	p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil)
	list := ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 9, IP: masterAddr.IP}, {ID: 7, IP: other.IP, Port: other.Port, Mode: 0x6a}})
	p.Packet(0, masterAddr, list, nil)
	if p.Peers() != 1 {
		t.Fatal(p.Peers())
	}
	return p
}

func TestPeerRegistration(t *testing.T) {
	t.Parallel()

	p, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr, MaxMissed: 2})
	if out := p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil); len(out) != 0 {
		t.Fatal("packet before the first Tick")
	}
	p.Tick(0, nil)
	if out := p.Packet(0, other, ctl(ipsc.OpRegisterReply, 1), nil); len(out) != 0 {
		t.Fatal("registration reply from another address")
	}
	if out := p.Packet(0, masterAddr, ctl(ipsc.OpPeerListReply, 1), nil); len(out) != 0 {
		t.Fatal("packet of the master before registration")
	}
	p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil)
	if out := p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil); len(out) != 0 {
		t.Fatal("second registration reply")
	}
	list := ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 9, IP: masterAddr.IP}, {ID: 7, IP: other.IP, Port: other.Port, Mode: 0x6a}})
	if out := p.Packet(0, masterAddr, list, nil); len(out) != 1 || p.Peers() != 1 || p.PeerAt(0).ID != 7 || p.PeerAt(-1).ID != 0 {
		t.Fatalf("list %v %d", kinds(out), p.Peers())
	}
	voice := []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 7, 1}
	if out := p.Packet(0, other, voice, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("voice of a listed peer before its link is up %v", kinds(out))
	}
	if out := p.Packet(0, other, ctl(ipsc.OpPeerRegisterRequest, 7), nil); len(out) != 1 || ipsc.Opcode(out[0].Packet[0]) != ipsc.OpPeerRegisterReply || out[0].Addr != other {
		t.Fatalf("peer registration %v", kinds(out))
	}
}

func TestPeerLinks(t *testing.T) {
	t.Parallel()

	s := time.Second
	p := listedPeer(t)
	out := p.Tick(5*s, nil)
	if len(out) != 2 || ipsc.Opcode(out[1].Packet[0]) != ipsc.OpPeerRegisterRequest || out[1].Addr != other {
		t.Fatalf("tick %x", sends(out))
	}
	if out = p.Packet(0, other, ctl(ipsc.OpPeerRegisterReply, 7), nil); len(out) != 1 || out[0].Kind != ipscsession.OutputUp {
		t.Fatalf("peer up %v", kinds(out))
	}
	p.Packet(0, other, ctl(ipsc.OpPeerRegisterReply, 7), nil)
	if out = p.Packet(0, other, ctl(ipsc.OpPeerAliveRequest, 7), nil); len(out) != 1 || ipsc.Opcode(out[0].Packet[0]) != ipsc.OpPeerAliveReply {
		t.Fatalf("peer keepalive %v", kinds(out))
	}
	for _, op := range []ipsc.Opcode{ipsc.OpStatus, ipsc.OpAliveReply, ipsc.OpPeerAliveReply, ipsc.OpRegisterRequest} {
		if out = p.Packet(0, masterAddr, ctl(op, 1), nil); len(out) != 0 {
			t.Fatalf("%v from the master", op)
		}
	}
	if out = p.Packet(0, masterAddr, ctl(ipsc.OpPeerAliveRequest, 1), nil); len(out) != 1 {
		t.Fatal("peer keepalive of the master")
	}
	if out = p.Packet(0, other, ctl(ipsc.OpRegisterRequest, 7), nil); len(out) != 0 {
		t.Fatal("registration request to a peer")
	}
	if out = p.Packet(0, other, ctl(ipsc.OpGroupVoice, 99), nil); len(out) != 0 {
		t.Fatal("packet of an unknown sender")
	}
}

func TestPeerListAndLeave(t *testing.T) {
	t.Parallel()

	s := time.Second
	p := listedPeer(t)
	p.Packet(0, other, ctl(ipsc.OpPeerRegisterReply, 7), nil)
	list := ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 7, IP: moved.IP, Port: moved.Port}})
	out := p.Packet(0, masterAddr, list, nil)
	if len(out) != 2 || out[0].Reason != ipscsession.ReasonList || p.PeerAt(0).Addr != moved || p.PeerAt(0).Up {
		t.Fatalf("moved peer %+v", out)
	}
	p.Packet(0, moved, ctl(ipsc.OpPeerRegisterReply, 7), nil)
	out = nil
	for now := 10 * s; now <= 25*s; now += 5 * s {
		out = p.Tick(now, out)
	}
	if count(out, ipscsession.OutputDown, ipscsession.ReasonTimeout) != 2 || p.State() != ipscsession.StateRegistering {
		t.Fatalf("missed keepalives %+v", out)
	}
	p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 2), nil)
	if id, _ := p.Master(); id != 2 {
		t.Fatal("new master ID")
	}
	out = p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 3), nil)
	if len(out) != 2 || out[0].Kind != ipscsession.OutputDown || out[1].Kind != ipscsession.OutputUp {
		t.Fatalf("master ID change %v", kinds(out))
	}
	list = ipsc.AppendPeerListReply(nil, 3, []ipsc.PeerEntry{{ID: 7, IP: moved.IP, Port: moved.Port}})
	p.Packet(0, masterAddr, list, nil)
	p.Packet(0, moved, ctl(ipsc.OpPeerRegisterReply, 7), nil)
	out = p.Packet(0, moved, ctl(ipsc.OpDeregisterRequest, 7), nil)
	if len(out) != 1 || out[0].Reason != ipscsession.ReasonClosed || p.Peers() != 0 {
		t.Fatalf("peer leaves %+v", out)
	}
	out = p.Packet(0, masterAddr, ctl(ipsc.OpDeregisterRequest, 3), nil)
	if len(out) != 1 || out[0].Reason != ipscsession.ReasonClosed || p.State() != ipscsession.StateRegistering {
		t.Fatalf("master leaves %+v", out)
	}
	if out = p.Close(0, nil); len(out) != 0 || p.State() != ipscsession.StateIdle {
		t.Fatalf("close while registering %v", kinds(out))
	}
	if out = p.Close(0, nil); len(out) != 0 {
		t.Fatal("second close")
	}
	if out = p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 3), nil); len(out) != 0 {
		t.Fatal("packet after close")
	}
}

func TestMasterTransitions(t *testing.T) {
	t.Parallel()

	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Timeout: 10 * time.Second})
	if _, ok := m.Deadline(); ok {
		t.Fatal("deadline without peers")
	}
	for _, pk := range [][]byte{{byte(ipsc.OpGroupVoice), 0, 0, 0, 7, 1}, ctl(ipsc.OpPeerListRequest, 7), ctl(ipsc.OpDeregisterRequest, 7)} {
		if out := m.Packet(0, xprAddr, pk, nil, nil); !isDrop(out, 7, ipscsession.ReasonUnregistered) {
			t.Fatalf("%x from a peer that is not registered: %+v", pk, out)
		}
	}
	if out := m.Packet(0, xprAddr, ctl(ipsc.OpRegisterReply, 7), nil, nil); len(out) != 0 {
		t.Fatal("reply of a peer that is not registered")
	}
	for _, pk := range [][]byte{nil, {0x80, 0, 0}, make([]byte, ipscsession.MaxPacket+1)} {
		if out := m.Packet(0, xprAddr, pk, nil, nil); !isDrop(out, 0, ipscsession.ReasonMalformed) {
			t.Fatalf("packet of %d octets: %+v", len(pk), out)
		}
	}
	if out := m.Packet(0, xprAddr, []byte{byte(ipsc.OpRegisterRequest), 0, 0, 0, 7}, nil, nil); !isDrop(out, 7, ipscsession.ReasonMalformed) {
		t.Fatalf("short registration %+v", out)
	}
	m.Packet(0, xprAddr, ctl(ipsc.OpRegisterRequest, 7), nil, nil)
	m.Packet(0, masterAddr, ctl(ipsc.OpRegisterRequest, 8), nil, nil)
	if out := m.Packet(0, xprAddr, []byte{byte(ipsc.OpPeerAliveRequest), 0, 0, 0, 7}, nil, nil); !isDrop(out, 7, ipscsession.ReasonMalformed) {
		t.Fatalf("short keepalive of a registered peer %+v", out)
	}
	for _, pk := range [][]byte{ctl(ipsc.OpAliveReply, 7), ctl(ipsc.OpPeerAliveReply, 7), {byte(ipsc.OpStatus), 0, 0, 0, 7, 0, 0, 0, 0}} {
		if out := m.Packet(5*time.Second, xprAddr, pk, nil, nil); len(out) != 0 {
			t.Fatalf("%x answered", pk)
		}
	}
	out := m.Tick(10*time.Second, nil)
	if len(out) != 2 || out[0].ID != 8 || out[0].Reason != ipscsession.ReasonTimeout || out[1].Addr != xprAddr {
		t.Fatalf("expiry %+v", out)
	}
	m.Close(0, nil)
	if out = m.Packet(11*time.Second, xprAddr, ctl(ipsc.OpRegisterRequest, 7), nil, nil); len(out) != 0 || len(m.Tick(time.Hour, nil)) != 0 || len(m.Close(0, nil)) != 0 {
		t.Fatal("master after close")
	}
	if _, ok := m.Deadline(); ok {
		t.Fatal("deadline after close")
	}
}

func TestPeerListFullTable(t *testing.T) {
	t.Parallel()

	p, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr, MaxPeers: 1})
	p.Tick(0, nil)
	p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil)
	p.Packet(0, masterAddr, ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 200, IP: other.IP, Port: 1}}), nil)
	p.Packet(0, masterAddr, ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 300, IP: moved.IP, Port: 2}}), nil)
	if p.Peers() != 1 || p.PeerAt(0).ID != 300 || p.PeerAt(0).Addr != (ipscsession.Addr{IP: moved.IP, Port: 2}) {
		t.Fatalf("peers %d %+v", p.Peers(), p.PeerAt(0))
	}
}

func isDrop(out []ipscsession.Output, id uint32, r ipscsession.Reason) bool {
	return len(out) == 1 && out[0].Kind == ipscsession.OutputDropped && out[0].ID == id && out[0].Reason == r
}

func TestMasterDropsAuth(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	reg := must(m.Sign(ctl(ipsc.OpRegisterRequest, 7)))
	bad := append([]byte(nil), reg...)
	bad[len(bad)-1] ^= 1
	for _, pk := range [][]byte{bad, ctl(ipsc.OpRegisterRequest, 7), {1, 2}} {
		if out := m.Packet(0, xprAddr, pk, nil, nil); !isDrop(out, 0, ipscsession.ReasonAuth) || out[0].Addr != xprAddr {
			t.Fatalf("%x: %+v", pk, out)
		}
	}
	if out := m.Packet(0, xprAddr, reg, nil, nil); len(out) != 2 || m.Peers() != 1 {
		t.Fatalf("good trailer %+v", out)
	}
}
