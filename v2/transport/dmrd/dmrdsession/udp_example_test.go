//go:build !tinygo

package dmrdsession_test

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

// ExamplePeer_loopback logs repeater 3009990 into a master over UDP on the loopback
// interface, first with the MMDVM network protocol and then with HBRP, and
// sends the voice LC header of a call to the master once the login is up.
func ExamplePeer_loopback() {
	call, _ := hex.DecodeString("444d5244002dedcf00005b002dedc6a1506290d021f40d58182015f8599012205c4d5d7f77fd757362300e202b9229044141c303910000")
	for _, dialect := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP} {
		if err := login(dialect, call); err != nil {
			fmt.Println(err)
		}
	}
	// Output:
	// MMDVM
	//   repeater sends RPTL
	//   master sends RPTACK
	//   repeater sends RPTK
	//   master sends RPTACK
	//   repeater sends RPTC
	//   master sends RPTACK
	//   repeater is Running
	//   repeater sends DMRD
	//   master gets a call from 3009999 to 91 on TS2
	// HBRP
	//   repeater sends RPTL
	//   master sends MSTACK
	//   repeater sends RPTK
	//   master sends MSTACK
	//   repeater sends RPTC
	//   master sends MSTACK
	//   repeater is Running
	//   repeater sends DMRD
	//   master gets a call from 3009999 to 91 on TS2
}

func login(dialect dmrd.Dialect, call []byte) error {
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

	peer, master, err := newMachines(dialect, rptcConfig(dialect))
	if err != nil {
		return err
	}
	return passPackets(dialect, call, masterConn, peerConn, peer, master)
}

func rptcConfig(dialect dmrd.Dialect) []byte {
	if dialect == dmrd.DialectMMDVM {
		var c dmrd.MMDVMConfig
		c.Reset()
		c.SetID(3009990)
		return c[:]
	}
	var c dmrd.HBRPConfig
	c.Reset()
	c.SetID(3009990)
	return c[:]
}

func newMachines(dialect dmrd.Dialect, rptc []byte) (*dmrdsession.Peer, *dmrdsession.Master, error) {
	peer, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dialect, ID: 3009990, Password: []byte("passw0rd"), Config: rptc})
	if err != nil {
		return nil, nil, err
	}
	master, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dialect})
	if err != nil {
		return nil, nil, err
	}
	return peer, master, nil
}

func passPackets(dialect dmrd.Dialect, call []byte, masterConn, peerConn *net.UDPConn, peer *dmrdsession.Peer, master *dmrdsession.Master) error {
	start := time.Now()
	toMaster, toPeer := 0, 0
	send := func(conn *net.UDPConn, to net.Addr, n *int, outs []dmrdsession.Output) {
		for _, o := range outs {
			if o.Kind == dmrdsession.OutputSend {
				if _, err := conn.WriteTo(o.Packet, to); err == nil {
					*n++
				}
			}
		}
	}
	read := func(conn *net.UDPConn, buf []byte) ([]byte, net.Addr, error) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFrom(buf)
		return buf[:n], from, err
	}
	send(peerConn, masterConn.LocalAddr(), &toMaster, peer.Tick(time.Since(start), nil))

	fmt.Println(dialect)
	buf := make([]byte, 1500)
	for toMaster+toPeer > 0 {
		for ; toMaster > 0; toMaster-- {
			p, from, err := read(masterConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("  repeater sends", dmrd.Classify(p, dialect))
			for _, o := range master.Packet(time.Since(start), p, nil) {
				switch o.Kind {
				case dmrdsession.OutputSend:
					send(masterConn, from, &toPeer, []dmrdsession.Output{o})
				case dmrdsession.OutputLogin:
					var salt [4]byte
					if _, err := rand.Read(salt[:]); err != nil {
						return err
					}
					send(masterConn, from, &toPeer, master.Admit(time.Since(start), binary.BigEndian.Uint32(salt[:]), []byte("passw0rd"), nil))
				case dmrdsession.OutputConfig:
					send(masterConn, from, &toPeer, master.Accept(time.Since(start), nil))
				case dmrdsession.OutputData:
					d, err := dmrd.DecodeData(o.Packet)
					if err != nil {
						return err
					}
					fmt.Println("  master gets a call from", d.Source, "to", d.Destination, "on", d.Slot)
				default:
				}
			}
		}
		for ; toPeer > 0; toPeer-- {
			p, _, err := read(peerConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("  master sends", dmrd.Classify(p, dialect))
			for _, o := range peer.Packet(time.Since(start), p, nil) {
				send(peerConn, masterConn.LocalAddr(), &toMaster, []dmrdsession.Output{o})
				if o.Kind == dmrdsession.OutputUp {
					fmt.Println("  repeater is", peer.State())
					if _, err := peerConn.WriteTo(call, masterConn.LocalAddr()); err == nil {
						toMaster++
					}
				}
			}
		}
	}
	return nil
}
