package session_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
)

type keyedNet struct {
	t     *testing.T
	m     *session.Master
	peers map[session.Addr]*session.Peer
	keys  map[uint32][20]byte
	ev    map[session.Addr][]session.Output
}

func (n *keyedNet) toMaster(now time.Duration, from session.Addr, p []byte) {
	r, err := ipsc.DecodeRaw(p)
	if err != nil {
		n.t.Fatal(err)
	}
	n.deliver(now, session.Addr{}, n.m.PacketWithKey(now, from, n.keys[r.ID], p, nil))
}

func (n *keyedNet) deliver(now time.Duration, from session.Addr, out []session.Output) {
	for _, o := range out {
		if o.Kind != session.OutputSend {
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

	a := session.Addr{IP: [4]byte{10, 0, 0, 2}, Port: 1}
	b := session.Addr{IP: [4]byte{10, 0, 0, 3}, Port: 1}
	ka, _ := auth.ParseKey("aaaa")
	kb, _ := auth.ParseKey("bbbb")
	m, _ := session.NewMaster(&session.MasterConfig{ID: 1})
	pa, _ := session.NewPeer(&session.PeerConfig{ID: 2, Master: masterAddr, Auth: true, Key: ka})
	pb, _ := session.NewPeer(&session.PeerConfig{ID: 3, Master: masterAddr, Auth: true, Key: kb})
	n := &keyedNet{t: t, m: m, peers: map[session.Addr]*session.Peer{a: pa, b: pb}, keys: map[uint32][20]byte{2: ka, 3: kb}, ev: map[session.Addr][]session.Output{}}
	for now := time.Duration(0); now <= 20*time.Second; now += session.AliveDefault {
		n.deliver(now, a, pa.Tick(now, nil))
		n.deliver(now, b, pb.Tick(now, nil))
	}
	if m.Peers() != 2 || pa.State() != session.StateRunning || pb.State() != session.StateRunning || pa.Peers() != 0 || pb.Peers() != 0 {
		t.Fatalf("peers %d, states %v %v, lists %d %d", m.Peers(), pa.State(), pb.State(), pa.Peers(), pb.Peers())
	}
	if count(n.ev[a], session.OutputPeers, session.ReasonNone) == 0 || count(n.ev[a], session.OutputDown, session.ReasonTimeout) != 0 {
		t.Fatalf("peer a events %v", kinds(n.ev[a]))
	}
	voice := []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 2, 1}
	signed := pa.Sign(append([]byte(nil), voice...))
	if out := m.PacketWithKey(time.Minute, a, ka, signed, nil); len(out) != 1 || out[0].Kind != session.OutputData {
		t.Fatalf("voice with its key %+v", out)
	}
	if out := m.PacketWithKey(time.Minute, a, kb, signed, nil); !isDrop(out, 0, session.ReasonAuth) {
		t.Fatalf("voice with another key %+v", out)
	}
	if out := m.Packet(time.Minute, a, voice, nil); !isDrop(out, 2, session.ReasonAuth) {
		t.Fatalf("voice of a keyed peer without its key %+v", out)
	}
	toB := m.SignFor(3, append([]byte(nil), 0x80, 0, 0, 0, 1, 7))
	if out := pb.Packet(time.Minute, masterAddr, toB, nil); len(out) != 1 || out[0].Kind != session.OutputData {
		t.Fatalf("SignFor %+v", out)
	}
	if out := pa.Packet(time.Minute, masterAddr, toB, nil); len(out) != 0 {
		t.Fatal("packet signed for b taken by a")
	}
	n.ev = map[session.Addr][]session.Output{}
	n.deliver(time.Minute, masterAddr, m.Close(time.Minute, nil))
	if count(n.ev[a], session.OutputDown, session.ReasonClosed) != 1 || count(n.ev[b], session.OutputDown, session.ReasonClosed) != 1 {
		t.Fatalf("close a %v b %v", kinds(n.ev[a]), kinds(n.ev[b]))
	}
}

func TestMasterKeyedAndSharedLists(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	own, _ := auth.ParseKey("cccc")
	m, _ := session.NewMaster(&session.MasterConfig{ID: 1, Auth: true, Key: key})
	m.Packet(0, xprAddr, m.Sign(ctl(ipsc.OpRegisterRequest, 7)), nil)
	m.AdoptWithKey(0, other, 8, 0x6a, ipsc.Flags{}, own, nil)
	out := m.Packet(0, xprAddr, m.Sign(ctl(ipsc.OpPeerListRequest, 7)), nil)
	list, err := ipsc.PeerListOf(out[0].Packet[:len(out[0].Packet)-auth.TrailerLen])
	if len(out) != 1 || err != nil || list.Len() != 1 || list.Entry(0).ID != 7 {
		t.Fatalf("shared list %+v %v", out, err)
	}
	out = m.PacketWithKey(0, other, own, m.SignFor(8, ctl(ipsc.OpPeerListRequest, 8)), nil)
	list, err = ipsc.PeerListOf(out[0].Packet[:len(out[0].Packet)-auth.TrailerLen])
	if len(out) != 1 || out[0].Addr != other || err != nil || list.Len() != 1 || list.Entry(0).ID != 8 {
		t.Fatalf("own list %+v %v", out, err)
	}
}

func TestMasterAdopt(t *testing.T) {
	t.Parallel()

	m, _ := session.NewMaster(&session.MasterConfig{ID: 1, MaxPeers: 1})
	out := m.Adopt(0, xprAddr, 7, 0x6a, ipsc.Flags{}, nil)
	if len(out) != 1 || out[0].Kind != session.OutputUp || out[0].ID != 7 || m.Peers() != 1 || m.PeerAt(0).Mode != 0x6a {
		t.Fatalf("adopt %+v", out)
	}
	if out = m.Adopt(time.Second, xprAddr, 7, 0x6a, ipsc.Flags{}, nil); len(out) != 0 {
		t.Fatalf("adopt again %+v", out)
	}
	if out = m.Packet(time.Second, xprAddr, []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 7, 1}, nil); len(out) != 1 || out[0].Kind != session.OutputData {
		t.Fatalf("voice after adopt %+v", out)
	}
	if out = m.Adopt(time.Second, other, 8, 0x6a, ipsc.Flags{}, nil); !isDrop(out, 8, session.ReasonFull) {
		t.Fatalf("adopt into a full table %+v", out)
	}
	if at, _ := m.Deadline(); at != time.Second+session.TimeoutDefault {
		t.Fatalf("deadline %v", at)
	}
	var zero session.Master
	if out = zero.Adopt(0, xprAddr, 7, 0, ipsc.Flags{}, nil); len(out) != 0 {
		t.Fatal("zero Master adopted")
	}
	if out = zero.PacketWithKey(0, xprAddr, [20]byte{}, ctl(ipsc.OpRegisterRequest, 7), nil); len(out) != 0 {
		t.Fatal("zero Master took a keyed packet")
	}
}
