package dmrdsession_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

// Example logs a Peer into a Master over the MMDVM network protocol, as
// DMRGateway logs into a master, passing each packet that one sends to the
// other.
func Example() {
	var rptc dmrd.MMDVMConfig
	copy(rptc[:], "RPTC")
	rptc.SetID(3129100)
	peer, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{
		Dialect: dmrd.DialectMMDVM, ID: 3129100, Password: []byte("passw0rd"), Config: rptc[:], Options: []byte("TS2=91;"),
	})
	master, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})

	now := time.Duration(0)
	var toMaster, toPeer [][]byte
	keep := func(q [][]byte, o dmrdsession.Output) [][]byte {
		return append(q, append([]byte(nil), o.Packet...))
	}
	for _, o := range peer.Tick(now, nil) {
		toMaster = keep(toMaster, o)
	}
	for len(toMaster) > 0 {
		for _, p := range toMaster {
			fmt.Println("repeater:", dmrd.Classify(p, dmrd.DialectMMDVM))
			for _, o := range master.Packet(now, p, nil) {
				switch o.Kind {
				case dmrdsession.OutputSend:
					toPeer = keep(toPeer, o)
				case dmrdsession.OutputLogin:
					for _, a := range master.Admit(now, 0xfcb17c08, []byte("passw0rd"), nil) {
						toPeer = keep(toPeer, a)
					}
				case dmrdsession.OutputConfig:
					fmt.Println("master event:", o.Kind)
					for _, a := range master.Accept(now, nil) {
						if a.Kind == dmrdsession.OutputSend {
							toPeer = keep(toPeer, a)
						} else {
							fmt.Println("master event:", a.Kind)
						}
					}
				default:
					fmt.Println("master event:", o.Kind)
				}
			}
		}
		toMaster = nil
		for _, p := range toPeer {
			fmt.Println("master:", dmrd.Classify(p, dmrd.DialectMMDVM))
			for _, o := range peer.Packet(now, p, nil) {
				if o.Kind == dmrdsession.OutputSend {
					toMaster = keep(toMaster, o)
				} else {
					fmt.Println("repeater event:", o.Kind)
				}
			}
		}
		toPeer = nil
	}
	// Output:
	// repeater: RPTL
	// master: RPTACK
	// repeater: RPTK
	// master: RPTACK
	// repeater: RPTC
	// master event: Config
	// master event: Up
	// master: RPTACK
	// repeater: RPTO
	// master event: Options
	// master: RPTACK
	// repeater event: Up
}
