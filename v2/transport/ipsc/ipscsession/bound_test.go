package ipscsession_test

import (
	"testing"
	"time"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

type octetBound interface {
	MaxOutputOctets() int
}

func distinctOctets(out []ipscsession.Output) int {
	seen := map[[2]uintptr]bool{}
	n := 0
	for _, o := range out {
		if len(o.Packet) == 0 {
			continue
		}
		k := [2]uintptr{uintptr(unsafe.Pointer(&o.Packet[0])), uintptr(len(o.Packet))}
		if !seen[k] {
			seen[k] = true
			n += len(o.Packet)
		}
	}
	return n
}

func TestMaxOutputOctets(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	const peers = 6
	m, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key, Timeout: 60 * time.Second, MaxPeers: peers})
	if err != nil {
		t.Fatal(err)
	}
	type pk struct {
		from, to ipscsession.Addr
		p        []byte
	}
	var queue []pk
	ps := make([]*ipscsession.Peer, peers)
	addrs := make([]ipscsession.Addr, peers)
	worst := map[string]int{}
	check := func(name string, b octetBound, from ipscsession.Addr, out []ipscsession.Output) {
		t.Helper()
		if n := distinctOctets(out); n > b.MaxOutputOctets() {
			t.Fatalf("%s: outputs hold %d octets, MaxOutputOctets %d", name, n, b.MaxOutputOctets())
		} else {
			worst[name] = max(worst[name], n)
		}
		for _, o := range out {
			if o.Kind == ipscsession.OutputSend {
				queue = append(queue, pk{from, o.Addr, append([]byte(nil), o.Packet...)})
			}
		}
	}
	for i := range ps {
		addrs[i] = ipscsession.Addr{IP: [4]byte{10, 0, 0, byte(i + 2)}, Port: 50000}
		if ps[i], err = ipscsession.NewPeer(&ipscsession.PeerConfig{ID: uint32(100 + i), Master: masterAddr, Auth: true, Key: key, MaxMissed: 3, MaxPeers: peers}); err != nil {
			t.Fatal(err)
		}
	}
	pump := func(now time.Duration) {
		for range 100000 {
			if len(queue) == 0 {
				return
			}
			q := queue[0]
			queue = queue[1:]
			if q.to == masterAddr {
				check("master packet", m, masterAddr, m.Packet(now, q.from, q.p, nil))
				continue
			}
			for i, a := range addrs {
				if a == q.to {
					check("peer packet", ps[i], a, ps[i].Packet(q.from, q.p, nil))
				}
			}
		}
		t.Fatal("network does not settle")
	}
	for now := time.Duration(0); now < 400*time.Second; now += time.Second {
		check("master tick", m, masterAddr, m.Tick(now, nil))
		for i, p := range ps {
			if now < 200*time.Second || i%2 == 0 {
				check("peer tick", p, addrs[i], p.Tick(now, nil))
			}
		}
		pump(now)
	}
	for i, p := range ps {
		check("peer close", p, addrs[i], p.Close(nil))
	}
	check("master close", m, masterAddr, m.Close(nil))
	if worst["master packet"] == 0 || worst["peer packet"] == 0 {
		t.Fatalf("the network sent nothing: %v", worst)
	}
	t.Log(worst)
}
