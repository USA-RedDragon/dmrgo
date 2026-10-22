package dmrdsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func fuzzSeeds() [][]byte {
	ctl := func(c dmrd.Command, id uint32) []byte {
		b, _ := (&dmrd.Control{Command: c, ID: id}).AppendBinary(nil, dmrd.DialectMMDVM)
		return b
	}
	tok, _ := auth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	rptk, _ := (&dmrd.Control{Command: dmrd.CommandRPTK, ID: 1234, Token: tok}).AppendBinary(nil, dmrd.DialectMMDVM)
	join := func(ps ...[]byte) []byte {
		var out []byte
		for _, p := range ps {
			out = append(out, byte(len(p)>>8), byte(len(p)), 3)
			out = append(out, p...)
		}
		return out
	}
	return [][]byte{
		join(ctl(dmrd.CommandRPTL, 1234), rptk, mmdvmConfig(1234), ctl(dmrd.CommandRPTPING, 1234)),
		join(ctl(dmrd.CommandRPTACK, 7), ctl(dmrd.CommandRPTACK, 1234), ctl(dmrd.CommandRPTACK, 1234), ctl(dmrd.CommandMSTPONG, 1234), ctl(dmrd.CommandMSTNAK, 1234)),
		join([]byte("MSTACK000004D20A7ED498"), []byte("MSTACK000004D2"), []byte("MSTACK000004D2"), []byte("RPTPONG000004D2"), []byte("MSTCL000004D2")),
		join([]byte("RPTL000004D2"), []byte("MSTPING000004D2"), hbrpConfig(1234), []byte("RPTCL000004D2")),
		join(ctl(dmrd.CommandRPTO, 1234), ctl(dmrd.CommandRPTCL, 1234), ctl(dmrd.CommandMSTCL, 1234)),
		{},
		{0, 4, 200, 'B', 'C', 'K', 'A'},
	}
}

func fuzzPackets(data []byte, f func(p []byte, dt time.Duration)) {
	for len(data) >= 3 {
		n := int(data[0])<<8 | int(data[1])
		dt := time.Duration(data[2]) * 500 * time.Millisecond
		data = data[3:]
		n = min(n, len(data))
		f(data[:n], dt)
		data = data[n:]
	}
}

func checkOutputs(t *testing.T, before, after []dmrdsession.Output, d dmrd.Dialect) {
	t.Helper()
	if len(after)-len(before) > dmrdsession.MaxOutputs {
		t.Fatalf("%d outputs", len(after)-len(before))
	}
	for _, o := range after[len(before):] {
		if o.Kind < dmrdsession.OutputSend || o.Kind > dmrdsession.OutputPong {
			t.Fatalf("kind %v", o.Kind)
		}
		if tooLong(o.Packet) {
			t.Fatalf("%d octets", len(o.Packet))
		}
		if o.Kind != dmrdsession.OutputSend {
			continue
		}
		c := dmrd.Classify(o.Packet, d)
		switch c {
		case dmrd.CommandRPTC:
			if _, err := dmrd.ConfigOf(o.Packet); err != nil {
				if _, err := dmrd.HBRPConfigOf(o.Packet); err != nil {
					t.Fatalf("sent %q", o.Packet)
				}
			}
		case dmrd.CommandBCKA:
		default:
			if _, err := dmrd.DecodeControl(o.Packet, d); err != nil {
				t.Fatalf("sent %q: %v", o.Packet, err)
			}
		}
	}
}

func checkSession(t *testing.T, data []byte) {
	for _, d := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP} {
		cfg := &dmrdsession.PeerConfig{Dialect: d, ID: 1234, Password: []byte("pw"), Config: mmdvmConfig(1234), Options: []byte("X=1")}
		if d == dmrd.DialectHBRP {
			cfg.Config, cfg.Options = hbrpConfig(1234), nil
		}
		p, err := dmrdsession.NewPeer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d})
		b, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("pw"), Enhanced: true})
		out := make([]dmrdsession.Output, 0, 64)
		now := time.Duration(0)
		step := func(f func([]dmrdsession.Output) []dmrdsession.Output) {
			n := len(out)
			out = f(out)
			checkOutputs(t, out[:n], out, d)
			if len(out) > 48 {
				out = out[:0]
			}
		}
		fuzzPackets(data, func(pk []byte, dt time.Duration) {
			now += dt
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Packet(now, pk, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output {
				o = m.Packet(now, pk, o)
				if m.State() == dmrdsession.StateLogin {
					o = m.Admit(0, 7, []byte("pw"), o)
				}
				return o
			})
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return b.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return b.Packet(now, pk, o) })
		})
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Resume(now, 1234, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Close(0, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Close(0, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Refuse(0, o) })
	}
}
