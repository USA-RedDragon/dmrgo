package session_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
)

func TestAllocsSession(t *testing.T) {
	key, _ := ipscKey()
	m, _ := session.NewMaster(&session.MasterConfig{ID: 1, Auth: true, Key: key})
	a, _ := session.NewPeer(&session.PeerConfig{ID: 2, Master: masterAddr, Auth: true, Key: key})
	b, _ := session.NewPeer(&session.PeerConfig{ID: 3, Master: masterAddr, Auth: true, Key: key})
	aAddr := session.Addr{IP: [4]byte{10, 0, 0, 2}, Port: 1}
	bAddr := session.Addr{IP: [4]byte{10, 0, 0, 3}, Port: 1}
	cAddr := session.Addr{IP: [4]byte{10, 0, 0, 4}, Port: 1}
	ao := make([]session.Output, 0, a.MaxOutputs())
	type pk struct {
		from, to session.Addr
		n        int
		b        [256]byte
	}
	q := make([]pk, 256)
	head, tail := 0, 0
	voice := make([]byte, 0, 32)
	now := time.Duration(0)
	queue := func(from session.Addr, out []session.Output) {
		for _, o := range out {
			if o.Kind == session.OutputSend && tail < len(q) {
				q[tail].from, q[tail].to, q[tail].n = from, o.Addr, copy(q[tail].b[:], o.Packet)
				tail++
			}
		}
	}
	drain := func() {
		for head < tail {
			e := &q[head]
			head++
			switch e.to {
			case masterAddr:
				queue(masterAddr, m.Packet(now, e.from, e.b[:e.n], ao[:0]))
			case aAddr:
				queue(aAddr, a.Packet(now, e.from, e.b[:e.n], ao[:0]))
			case bAddr:
				queue(bAddr, b.Packet(now, e.from, e.b[:e.n], ao[:0]))
			}
		}
		head, tail = 0, 0
	}
	run := func() {
		for range 4 {
			queue(aAddr, a.Tick(now, ao[:0]))
			drain()
			queue(bAddr, b.Tick(now, ao[:0]))
			drain()
			queue(masterAddr, m.Tick(now, ao[:0]))
			drain()
			now += session.AliveDefault
		}
		voice = a.Sign(append(voice[:0], 0x80, 0, 0, 0, 2, 1))
		ao = b.Packet(now, aAddr, voice, ao[:0])
		ao = m.AdoptWithKey(now, cAddr, 4, 0x6a, ipsc.Flags{}, key, ao[:0])
		ao = m.PacketWithKey(now, cAddr, key, m.SignFor(4, append(voice[:0], 0x80, 0, 0, 0, 4, 1)), ao[:0])
		ao = m.Adopt(now, cAddr, 4, 0x6a, ipsc.Flags{}, ao[:0])
	}
	run()
	run()
	if m.Peers() != 3 || a.Peers() != 1 || !a.PeerAt(0).Up {
		t.Fatalf("peers %d %d %+v", m.Peers(), a.Peers(), a.PeerAt(0))
	}
	if n := testing.AllocsPerRun(100, run); n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
