package ipscsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

type keyedNet struct {
	t     *testing.T
	m     *ipscsession.Master
	peers map[ipscsession.Addr]*ipscsession.Peer
	keys  map[uint32][20]byte
	ev    map[ipscsession.Addr][]ipscsession.Output
}

func (n *keyedNet) toMaster(now time.Duration, from ipscsession.Addr, p []byte) {
	r, err := ipsc.DecodeRaw(p)
	if err != nil {
		n.t.Fatal(err)
	}
	k := n.keys[r.ID]
	n.deliver(now, ipscsession.Addr{}, n.m.Packet(now, from, p, &k, nil))
}

func (n *keyedNet) deliver(now time.Duration, from ipscsession.Addr, out []ipscsession.Output) {
	for _, o := range out {
		if o.Kind != ipscsession.OutputSend {
			n.ev[from] = append(n.ev[from], o)
			continue
		}
		pk := append([]byte(nil), o.Packet...)
		if o.Addr == masterAddr {
			n.toMaster(now, from, pk)
			continue
		}
		if p := n.peers[o.Addr]; p != nil {
			n.deliver(now, o.Addr, p.Packet(now, masterAddr, pk, nil))
		}
	}
}

func TestMasterKeysOfPeers(t *testing.T) {
	t.Parallel()

	a := ipscsession.Addr{IP: [4]byte{10, 0, 0, 2}, Port: 1}
	b := ipscsession.Addr{IP: [4]byte{10, 0, 0, 3}, Port: 1}
	ka, _ := ipscauth.ParseKey("aaaa")
	kb, _ := ipscauth.ParseKey("bbbb")
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	pa, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 2, Master: masterAddr, Auth: true, Key: ka})
	pb, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 3, Master: masterAddr, Auth: true, Key: kb})
	n := &keyedNet{t: t, m: m, peers: map[ipscsession.Addr]*ipscsession.Peer{a: pa, b: pb}, keys: map[uint32][20]byte{2: ka, 3: kb}, ev: map[ipscsession.Addr][]ipscsession.Output{}}
	for now := time.Duration(0); now <= 20*time.Second; now += ipscsession.AliveDefault {
		n.deliver(now, a, pa.Tick(now, nil))
		n.deliver(now, b, pb.Tick(now, nil))
	}
	if m.Peers() != 2 || pa.State() != ipscsession.StateRunning || pb.State() != ipscsession.StateRunning || pa.Peers() != 0 || pb.Peers() != 0 {
		t.Fatalf("peers %d, states %v %v, lists %d %d", m.Peers(), pa.State(), pb.State(), pa.Peers(), pb.Peers())
	}
	if count(n.ev[a], ipscsession.OutputPeers, ipscsession.ReasonNone) == 0 || count(n.ev[a], ipscsession.OutputDown, ipscsession.ReasonTimeout) != 0 {
		t.Fatalf("peer a events %v", kinds(n.ev[a]))
	}
	voice := []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 2, 1}
	signed := must(pa.Sign(append([]byte(nil), voice...)))
	if out := m.Packet(time.Minute, a, signed, &ka, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("voice with its key %+v", out)
	}
	if out := m.Packet(time.Minute, a, signed, &kb, nil); !isDrop(out, 0, ipscsession.ReasonAuth) {
		t.Fatalf("voice with another key %+v", out)
	}
	if out := m.Packet(time.Minute, a, voice, nil, nil); !isDrop(out, 2, ipscsession.ReasonAuth) {
		t.Fatalf("voice of a keyed peer without its key %+v", out)
	}
	toB := must(m.SignFor(3, append([]byte(nil), 0x80, 0, 0, 0, 1, 7)))
	if out := pb.Packet(0, masterAddr, toB, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("SignFor %+v", out)
	}
	if out := pa.Packet(0, masterAddr, toB, nil); len(out) != 0 {
		t.Fatal("packet signed for b taken by a")
	}
	n.ev = map[ipscsession.Addr][]ipscsession.Output{}
	n.deliver(time.Minute, masterAddr, m.Close(time.Minute, nil))
	if count(n.ev[a], ipscsession.OutputDown, ipscsession.ReasonClosed) != 1 || count(n.ev[b], ipscsession.OutputDown, ipscsession.ReasonClosed) != 1 {
		t.Fatalf("close a %v b %v", kinds(n.ev[a]), kinds(n.ev[b]))
	}
}

func TestMasterKeyedAndSharedLists(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	own, _ := ipscauth.ParseKey("cccc")
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	m.Packet(0, xprAddr, must(m.Sign(ctl(ipsc.OpRegisterRequest, 7))), nil, nil)
	m.Adopt(0, other, 8, &own, nil)
	out := m.Packet(0, xprAddr, must(m.Sign(ctl(ipsc.OpPeerListRequest, 7))), nil, nil)
	list, err := ipsc.PeerListOf(out[0].Packet[:len(out[0].Packet)-ipscauth.TrailerLen])
	if len(out) != 1 || err != nil || list.Len() != 1 || list.Entry(0).ID != 7 {
		t.Fatalf("shared list %+v %v", out, err)
	}
	out = m.Packet(0, other, must(m.SignFor(8, ctl(ipsc.OpPeerListRequest, 8))), &own, nil)
	list, err = ipsc.PeerListOf(out[0].Packet[:len(out[0].Packet)-ipscauth.TrailerLen])
	if len(out) != 1 || out[0].Addr != other || err != nil || list.Len() != 1 || list.Entry(0).ID != 8 {
		t.Fatalf("own list %+v %v", out, err)
	}
}

func TestMasterAdoptKnown(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	own, _ := ipscauth.ParseKey("cccc")
	voice := func(id byte) []byte { return []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, id, 1} }
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key, AdoptKnown: true})
	out := m.Packet(0, xprAddr, must(m.Sign(voice(7))), nil, nil)
	if len(out) != 2 || out[0].Kind != ipscsession.OutputUp || out[1].Kind != ipscsession.OutputData || out[1].ID != 7 {
		t.Fatalf("adopt with the config key %+v", out)
	}
	if p := m.PeerAt(0); p.ID != 7 || p.Mode != ipscsession.ModeDefault || p.Addr != xprAddr {
		t.Fatalf("adopted peer %+v", p)
	}
	signer, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: own})
	out = m.Packet(0, other, must(signer.Sign(voice(8))), &own, nil)
	if len(out) != 2 || out[0].Kind != ipscsession.OutputUp || out[1].Kind != ipscsession.OutputData || out[1].ID != 8 {
		t.Fatalf("adopt with an own key %+v", out)
	}
	if out = m.Packet(0, other, must(signer.Sign(voice(9))), nil, nil); !isDrop(out, 0, ipscsession.ReasonAuth) {
		t.Fatalf("wrong key %+v", out)
	}
	if out = m.Packet(0, other, must(m.Sign(ctl(ipsc.OpDeregisterRequest, 9))), nil, nil); !isDrop(out, 9, ipscsession.ReasonUnregistered) {
		t.Fatalf("de-registration of an unknown peer %+v", out)
	}
	open, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, AdoptKnown: true})
	if out = open.Packet(0, xprAddr, voice(7), nil, nil); !isDrop(out, 7, ipscsession.ReasonUnregistered) {
		t.Fatalf("adopt without a key %+v", out)
	}
}

func TestMasterAdopt(t *testing.T) {
	t.Parallel()

	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, MaxPeers: 1})
	out := m.Adopt(0, xprAddr, 7, nil, nil)
	if len(out) != 1 || out[0].Kind != ipscsession.OutputUp || out[0].ID != 7 || m.Peers() != 1 || m.PeerAt(0).Mode != 0x6a || !peerOK(m.Peer(7)) || peerOK(m.Peer(8)) {
		t.Fatalf("adopt %+v", out)
	}
	if out = m.Adopt(time.Second, xprAddr, 7, nil, nil); len(out) != 0 {
		t.Fatalf("adopt again %+v", out)
	}
	if out = m.Packet(time.Second, xprAddr, []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 7, 1}, nil, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("voice after adopt %+v", out)
	}
	if out = m.Adopt(time.Second, other, 8, nil, nil); !isDrop(out, 8, ipscsession.ReasonFull) {
		t.Fatalf("adopt into a full table %+v", out)
	}
	if at, _ := m.Deadline(); at != time.Second+ipscsession.TimeoutDefault {
		t.Fatalf("deadline %v", at)
	}
	var zero ipscsession.Master
	if out = zero.Adopt(0, xprAddr, 7, nil, nil); len(out) != 0 {
		t.Fatal("zero Master adopted")
	}
	if out = zero.Packet(0, xprAddr, ctl(ipsc.OpRegisterRequest, 7), &[20]byte{}, nil); len(out) != 0 {
		t.Fatal("zero Master took a keyed packet")
	}
}
