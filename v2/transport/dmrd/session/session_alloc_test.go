package session_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func TestAllocsSession(t *testing.T) {
	p, _ := session.NewPeer(&session.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1234, Password: []byte("pw"), Config: mmdvmConfig(1234), Options: []byte("X=1")})
	m, _ := session.NewMaster(&session.MasterConfig{Dialect: dmrd.DialectMMDVM})
	b, _ := session.NewBridge(&session.BridgeConfig{Key: []byte("pw"), Enhanced: true})
	pass := []byte("pw")
	po := make([]session.Output, 0, session.MaxOutputs)
	mo := make([]session.Output, 0, session.MaxOutputs)
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
				if o.Kind == session.OutputSend {
					mo = m.Packet(now, o.Packet, mo)
				}
			}
			if m.State() == session.StateLogin {
				mo = m.Admit(now, 7, pass, mo[:0])
			}
			po = po[:0]
			for _, o := range mo {
				if o.Kind == session.OutputSend {
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
		po = p.Close(now, po[:0])
	}
	run()
	if p.State() != session.StateIdle || m.State() != session.StateRunning || b.State() != session.StateRunning {
		t.Fatalf("states %v %v %v", p.State(), m.State(), b.State())
	}
	if n := testing.AllocsPerRun(100, run); n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
