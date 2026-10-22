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
