//go:build !tinygo

package ipscsession_test

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

// ExamplePeer_loopback registers peer 314926 with a master over UDP on the loopback
// interface, with the authentication key 123456789abcdef1234, and sends
// the master the voice LC header of a call once it is registered.
func ExamplePeer_loopback() {
	if err := register(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// master gets RegisterRequest
	// master: Up 314926
	// peer gets RegisterReply
	// peer: Up 1
	// master gets PeerListRequest
	// peer gets PeerListReply
	// peer: Peers 0
	// master gets GroupVoice
	// master: Data 314926 GroupVoice from 3191868 to 9
}

func register() error {
	loopback := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	masterConn, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		return err
	}
	defer masterConn.Close()
	peerConn, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		return err
	}
	defer peerConn.Close()

	peer, master, err := newMachines(masterConn.LocalAddr())
	if err != nil {
		return err
	}
	call, _ := hex.DecodeString("800004ce2e1d30b43c0000090200000d9e2080dd7b5f482e7d3e0000000001c0000a800a006000000000000930b43ce9f8ed0071466f")
	return passPackets(call, masterConn, peerConn, peer, master)
}

func newMachines(masterAt net.Addr) (*ipscsession.Peer, *ipscsession.Master, error) {
	key, err := ipscauth.ParseKey("123456789abcdef1234")
	if err != nil {
		return nil, nil, err
	}
	at, err := netip.ParseAddrPort(masterAt.String())
	if err != nil {
		return nil, nil, err
	}
	masterAddr := ipscsession.Addr{IP: at.Addr().As4(), Port: at.Port()}
	master, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	if err != nil {
		return nil, nil, err
	}
	peer, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 314926, Master: masterAddr, Auth: true, Key: key})
	if err != nil {
		return nil, nil, err
	}
	return peer, master, nil
}

func passPackets(call []byte, masterConn, peerConn *net.UDPConn, peer *ipscsession.Peer, master *ipscsession.Master) error {
	start := time.Now()
	toMaster, toPeer := 0, 0
	send := func(conn *net.UDPConn, n *int, outs []ipscsession.Output) error {
		for _, o := range outs {
			if o.Kind != ipscsession.OutputSend {
				continue
			}
			to := &net.UDPAddr{IP: o.Addr.IP[:], Port: int(o.Addr.Port)}
			if _, err := conn.WriteTo(o.Packet, to); err != nil {
				return err
			}
			*n++
		}
		return nil
	}
	read := func(conn *net.UDPConn, buf []byte) ([]byte, ipscsession.Addr, error) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, ipscsession.Addr{}, err
		}
		a, _ := ipscsession.AddrOf(from.IP, from.Port)
		return buf[:n], a, nil
	}
	if err := send(peerConn, &toMaster, peer.Tick(time.Since(start), nil)); err != nil {
		return err
	}

	buf := make([]byte, ipscsession.MaxPacket)
	for toMaster+toPeer > 0 {
		for ; toMaster > 0; toMaster-- {
			p, from, err := read(masterConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("master gets", ipsc.Opcode(p[0]))
			outs := master.Packet(time.Since(start), from, p, nil, nil)
			for _, o := range outs {
				switch o.Kind {
				case ipscsession.OutputData:
					u, err := ipsc.DecodeUser(o.Packet)
					if err != nil {
						return err
					}
					fmt.Println("master:", o.Kind, o.ID, u.Opcode, "from", u.Source, "to", u.Destination)
				case ipscsession.OutputSend:
				default:
					fmt.Println("master:", o.Kind, o.ID)
				}
			}
			if err := send(masterConn, &toPeer, outs); err != nil {
				return err
			}
		}
		for ; toPeer > 0; toPeer-- {
			p, from, err := read(peerConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("peer gets", ipsc.Opcode(p[0]))
			outs := peer.Packet(time.Since(start), from, p, nil)
			for _, o := range outs {
				if o.Kind == ipscsession.OutputSend {
					continue
				}
				fmt.Println("peer:", o.Kind, o.ID)
				if o.Kind == ipscsession.OutputPeers {
					if err := sendSigned(peer, peerConn, masterConn.LocalAddr(), call); err != nil {
						return err
					}
					toMaster++
				}
			}
			if err := send(peerConn, &toMaster, outs); err != nil {
				return err
			}
		}
	}
	return nil
}

func sendSigned(peer *ipscsession.Peer, conn *net.UDPConn, to net.Addr, packet []byte) error {
	signed, err := peer.Sign(append([]byte(nil), packet...))
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(signed, to)
	return err
}
