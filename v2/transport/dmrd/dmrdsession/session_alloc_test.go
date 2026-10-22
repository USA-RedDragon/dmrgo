package dmrdsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestAllocsSession(t *testing.T) {
	p, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1234, Password: []byte("pw"), Config: mmdvmConfig(1234), Options: []byte("X=1")})
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})
	b, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("pw"), Enhanced: true})
	pass := []byte("pw")
	po := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs)
	mo := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs)
	var d dmrd.Data
	d.Repeater, d.Slot = 1234, 1
	data, _ := d.AppendBinary(nil)
	now := time.Duration(0)
	run := func() {
		p.Reset()
		m.Reset()
		po = p.Tick(now, po[:0])
		for range 8 {
			mo = mo[:0]
			for _, o := range po {
				if o.Kind == dmrdsession.OutputSend {
					mo = m.Packet(now, o.Packet, mo)
				}
			}
			if m.State() == dmrdsession.StateLogin {
				mo = m.Admit(now, 7, pass, mo[:0])
			}
			mo = m.Accept(now, mo)
			po = po[:0]
			for _, o := range mo {
				if o.Kind == dmrdsession.OutputSend {
					po = p.Packet(now, o.Packet, po)
				}
			}
		}
		now += 10 * time.Second
		po = p.Tick(now, po[:0])
		mo = m.Packet(now, po[0].Packet, mo[:0])
		po = p.Packet(now, mo[0].Packet, po[:0])
		po = p.Packet(now, data, po[:0])
		mo = m.Packet(now, data, mo[:0])
		bo := b.Tick(now, po[:0])
		po = b.Packet(now, bo[0].Packet, po[:0])
		mo = m.Resume(now, 99, mo[:0])
		mo = m.Resume(now, 1234, mo[:0])
		po = p.Close(now, po[:0])
	}
	run()
	if p.State() != dmrdsession.StateIdle || m.State() != dmrdsession.StateRunning || b.State() != dmrdsession.StateRunning {
		t.Fatalf("states %v %v %v", p.State(), m.State(), b.State())
	}
	if n := testing.AllocsPerRun(100, run); n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
