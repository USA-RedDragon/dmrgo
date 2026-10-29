package ipscsession_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

// Example registers a Peer with a Master, as a MOTOTRBO repeater
// registers with the master of its IPSC system, passing each packet that
// one sends to the other.
func Example() {
	master, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	peerAddr := ipscsession.Addr{IP: [4]byte{192, 168, 0, 231}, Port: 50000}
	masterAddr := ipscsession.Addr{IP: [4]byte{192, 168, 0, 10}, Port: 50000}
	peer, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 315950, Master: masterAddr})

	now := time.Duration(0)
	for range 2 {
		var queue [][]byte
		for _, o := range peer.Tick(now, nil) {
			queue = append(queue, o.Packet)
		}
		for len(queue) > 0 {
			pk := queue[0]
			queue = queue[1:]
			fmt.Println("peer sends", ipsc.Opcode(pk[0]))
			for _, r := range master.Packet(now, peerAddr, pk, nil, nil) {
				if r.Kind != ipscsession.OutputSend {
					fmt.Println("master event", r.Kind, r.ID)
					continue
				}
				fmt.Println("master sends", ipsc.Opcode(r.Packet[0]))
				for _, e := range peer.Packet(now, masterAddr, r.Packet, nil) {
					if e.Kind == ipscsession.OutputSend {
						queue = append(queue, e.Packet)
						continue
					}
					fmt.Println("peer event", e.Kind, e.ID)
				}
			}
		}
		now += ipscsession.AliveDefault
	}
	// Output:
	// peer sends RegisterRequest
	// master sends RegisterReply
	// peer event Up 1
	// master event Up 315950
	// peer sends PeerListRequest
	// master sends PeerListReply
	// peer event Peers 0
	// peer sends AliveRequest
	// master sends AliveReply
}

// ExamplePeer runs a Master and two Peers in memory. Each Peer registers
// with the master, gets the peer list, and registers with the other peer,
// so the voice of one repeater can go straight to the other.
func ExamplePeer() {
	masterAddr := ipscsession.Addr{IP: [4]byte{192, 168, 0, 10}, Port: 50000}
	addrA := ipscsession.Addr{IP: [4]byte{192, 168, 0, 231}, Port: 50000}
	addrB := ipscsession.Addr{IP: [4]byte{192, 168, 0, 232}, Port: 50000}
	master, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	a, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 315950, Master: masterAddr})
	b, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 315951, Master: masterAddr})
	if err != nil {
		fmt.Println(err)
		return
	}
	type packet struct {
		from, to ipscsession.Addr
		p        []byte
	}
	var queue []packet
	take := func(from ipscsession.Addr, name string, out []ipscsession.Output) {
		for _, o := range out {
			if o.Kind == ipscsession.OutputSend {
				queue = append(queue, packet{from, o.Addr, append([]byte(nil), o.Packet...)})
			} else if name == "A" && o.Kind == ipscsession.OutputPeers {
				fmt.Println(name, o.Kind, a.Peers())
			} else if name == "A" {
				fmt.Println(name, o.Kind, o.ID)
			}
		}
	}
	for now := time.Duration(0); now <= ipscsession.AliveDefault; now += ipscsession.AliveDefault {
		take(addrA, "A", a.Tick(now, nil))
		take(addrB, "B", b.Tick(now, nil))
		for len(queue) > 0 {
			k := queue[0]
			queue = queue[1:]
			switch k.to {
			case masterAddr:
				take(masterAddr, "master", master.Packet(now, k.from, k.p, nil, nil))
			case addrA:
				take(addrA, "A", a.Packet(now, k.from, k.p, nil))
			case addrB:
				take(addrB, "B", b.Packet(now, k.from, k.p, nil))
			}
		}
	}
	info, _ := a.Peer(315951)
	fmt.Println("A has", a.Peers(), "peer:", info.ID, "up", info.Up)
	// Output:
	// A Up 1
	// A Peers 1
	// A Peers 1
	// A Up 315951
	// A has 1 peer: 315951 up true
}

// ExampleMaster runs a Master with authentication. A peer with the key
// registers, a packet with another key is dropped, and the peer that
// stops sending times out.
func ExampleMaster() {
	key := [20]byte{1, 2, 3, 4}
	master, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	if err != nil {
		fmt.Println(err)
		return
	}
	masterAddr := ipscsession.Addr{IP: [4]byte{192, 168, 0, 10}, Port: 50000}
	good, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 315950, Master: masterAddr, Auth: true, Key: key})
	bad, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 315951, Master: masterAddr, Auth: true, Key: [20]byte{9}})
	addrs := []ipscsession.Addr{{IP: [4]byte{192, 168, 0, 231}, Port: 50000}, {IP: [4]byte{192, 168, 0, 232}, Port: 50000}}
	out := make([]ipscsession.Output, 0, master.MaxOutputs())
	for i, peer := range []*ipscsession.Peer{good, bad} {
		for _, req := range peer.Tick(0, nil) {
			for _, o := range master.Packet(0, addrs[i], req.Packet, nil, out[:0]) {
				switch o.Kind {
				case ipscsession.OutputUp:
					fmt.Println(o.Kind, o.ID)
				case ipscsession.OutputDropped:
					fmt.Println(o.Kind, o.Reason)
				default:
				}
			}
		}
	}
	fmt.Println("peers", master.Peers())
	at, _ := master.Deadline()
	for _, o := range master.Tick(at, out[:0]) {
		fmt.Println(at, o.Kind, o.ID, o.Reason)
	}
	// Output:
	// Up 315950
	// Dropped Auth
	// peers 1
	// 2m0s Down 315950 Timeout
}
