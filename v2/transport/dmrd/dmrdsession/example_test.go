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

// ExamplePeer runs an HBRP Peer whose master does not answer. The Peer
// sends the RPTL again every Retry, ends the session after Timeout, and
// logs in again after the Retry wait.
func ExamplePeer() {
	var rptc dmrd.HBRPConfig
	copy(rptc[:], "RPTC")
	rptc.SetID(2620001)
	peer, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{
		Dialect: dmrd.DialectHBRP, ID: 2620001, Password: []byte("passw0rd"), Config: rptc[:],
		Retry: 5 * time.Second, Timeout: 15 * time.Second,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs)
	for {
		at, ok := peer.Deadline()
		if !ok || at > 25*time.Second {
			break
		}
		for _, o := range peer.Tick(at, out[:0]) {
			switch o.Kind {
			case dmrdsession.OutputSend:
				fmt.Println(at, "send", dmrd.Classify(o.Packet, dmrd.DialectHBRP))
			case dmrdsession.OutputDown:
				fmt.Println(at, o.Kind, o.Reason, "in", o.State)
			default:
				fmt.Println(at, o.Kind)
			}
		}
	}
	fmt.Println(peer.State())
	// Output:
	// 0s send RPTL
	// 5s send RPTL
	// 10s send RPTL
	// 15s Down Timeout in Login
	// 20s send RPTL
	// 25s send RPTL
	// Login
}

// ExampleMaster ends the session of a repeater that stops sending, and
// answers its next packet with MSTNAK, so that the repeater logs in again.
func ExampleMaster() {
	master, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs)
	for _, o := range master.Resume(0, 3129100, out[:0]) {
		fmt.Println(o.Kind, o.ID)
	}
	at, _ := master.Deadline()
	for _, o := range master.Tick(at, out[:0]) {
		fmt.Println(at, o.Kind, o.Reason, "in", o.State)
	}
	ping := dmrd.Control{Command: dmrd.CommandRPTPING, ID: 3129100}
	p, _ := ping.AppendBinary(nil, dmrd.DialectMMDVM)
	for _, o := range master.Packet(at, p, out[:0]) {
		fmt.Println("answer", dmrd.Classify(o.Packet, dmrd.DialectMMDVM))
	}
	// Output:
	// Up 3129100
	// 1m0s Down Timeout in Running
	// answer MSTNAK
}

// ExampleBridge runs two ends of an enhanced OpenBridge link in memory.
// The keepalive of one end brings the link up at the other, a signed
// packet passes, a packet with a bad signature is dropped, and the link
// goes down when the keepalives stop.
func ExampleBridge() {
	cfg := dmrdsession.BridgeConfig{Key: []byte("bridge passphrase"), Enhanced: true}
	a, _ := dmrdsession.NewBridge(&cfg)
	b, err := dmrdsession.NewBridge(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs)
	for _, ka := range a.Tick(0, out[:0]) {
		for _, o := range b.Packet(0, ka.Packet, nil) {
			fmt.Println("b:", o.Kind)
		}
	}
	sq := dmrd.BridgeControl{Command: dmrd.CommandBCSQ, TalkGroup: 91, Stream: 7}
	p, _ := sq.AppendBinary(nil)
	if err := a.SignInPlace(p); err != nil {
		fmt.Println(err)
		return
	}
	for _, o := range b.Packet(time.Second, p, out[:0]) {
		fmt.Println("b:", o.Kind, o.Command)
	}
	p[len(p)-1] ^= 1
	fmt.Println("b: bad signature,", len(b.Packet(2*time.Second, p, out[:0])), "outputs")
	for {
		at, _ := b.Deadline()
		out = b.Tick(at, out[:0])
		if len(out) > 0 && out[0].Kind == dmrdsession.OutputDown {
			fmt.Println("b:", at, out[0].Kind, out[0].Reason, b.State())
			break
		}
	}
	// Output:
	// b: Up
	// b: Message BCSQ
	// b: bad signature, 0 outputs
	// b: 1m1s Down Timeout Idle
}
