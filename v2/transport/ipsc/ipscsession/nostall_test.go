package ipscsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func sentTo(out []ipscsession.Output, op ipsc.Opcode, to ipscsession.Addr) bool {
	for _, o := range out {
		if o.Kind == ipscsession.OutputSend && len(o.Packet) > 0 && ipsc.Opcode(o.Packet[0]) == op && o.Addr == to {
			return true
		}
	}
	return false
}

func TestPeerLinkSetupWithoutTicks(t *testing.T) {
	t.Parallel()

	p, _ := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr})
	p.Tick(0, nil)
	out := p.Packet(0, masterAddr, ctl(ipsc.OpRegisterReply, 1), nil)
	if len(out) > p.MaxOutputs() || !sentTo(out, ipsc.OpPeerListRequest, masterAddr) {
		t.Fatalf("registration reply: %v, want the list request at once", kinds(out))
	}
	list := ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 9, IP: masterAddr.IP}, {ID: 7, IP: other.IP, Port: other.Port, Mode: 0x6a}})
	out = p.Packet(0, masterAddr, list, nil)
	if len(out) > p.MaxOutputs() || !sentTo(out, ipsc.OpPeerRegisterRequest, other) {
		t.Fatalf("peer list: %v, want the peer registration at once", kinds(out))
	}
	out = p.Packet(0, masterAddr, ctl(ipsc.OpDeregisterRequest, 1), nil)
	if len(out) > p.MaxOutputs() || !sentTo(out, ipsc.OpRegisterRequest, masterAddr) {
		t.Fatalf("de-registration: %v, want the registration at once", kinds(out))
	}
}
