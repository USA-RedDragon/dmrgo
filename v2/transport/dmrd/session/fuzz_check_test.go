package session_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func fuzzSeeds() [][]byte {
	ctl := func(c dmrd.Command, id uint32) []byte {
		b, _ := dmrd.Control{Command: c, ID: id}.AppendBinary(nil, dmrd.DialectMMDVM)
		return b
	}
	tok, _ := auth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	rptk, _ := dmrd.Control{Command: dmrd.CommandRPTK, ID: 1234, Token: tok}.AppendBinary(nil, dmrd.DialectMMDVM)
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

func checkOutputs(t *testing.T, before, after []session.Output, d dmrd.Dialect) {
	t.Helper()
	if len(after)-len(before) > session.MaxOutputs {
		t.Fatalf("%d outputs", len(after)-len(before))
	}
	for _, o := range after[len(before):] {
		if o.Kind < session.OutputSend || o.Kind > session.OutputMessage {
			t.Fatalf("kind %v", o.Kind)
		}
		if tooLong(o.Packet) {
			t.Fatalf("%d octets", len(o.Packet))
		}
		if o.Kind != session.OutputSend {
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
		cfg := &session.PeerConfig{Dialect: d, ID: 1234, Password: []byte("pw"), Config: mmdvmConfig(1234), Options: []byte("X=1")}
		if d == dmrd.DialectHBRP {
			cfg.Config, cfg.Options = hbrpConfig(1234), nil
		}
		p, err := session.NewPeer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		m, _ := session.NewMaster(&session.MasterConfig{Dialect: d})
		b, _ := session.NewBridge(&session.BridgeConfig{Key: []byte("pw"), Enhanced: true})
		out := make([]session.Output, 0, 64)
		now := time.Duration(0)
		step := func(f func([]session.Output) []session.Output) {
			n := len(out)
			out = f(out)
			checkOutputs(t, out[:n], out, d)
			if len(out) > 48 {
				out = out[:0]
			}
		}
		fuzzPackets(data, func(pk []byte, dt time.Duration) {
			now += dt
			step(func(o []session.Output) []session.Output { return p.Tick(now, o) })
			step(func(o []session.Output) []session.Output { return p.Packet(now, pk, o) })
			step(func(o []session.Output) []session.Output { return m.Tick(now, o) })
			step(func(o []session.Output) []session.Output {
				o = m.Packet(now, pk, o)
				if m.State() == session.StateLogin {
					o = m.Admit(now, 7, []byte("pw"), o)
				}
				return o
			})
			step(func(o []session.Output) []session.Output { return b.Tick(now, o) })
			step(func(o []session.Output) []session.Output { return b.Packet(now, pk, o) })
		})
		step(func(o []session.Output) []session.Output { return m.Resume(now, 1234, o) })
		step(func(o []session.Output) []session.Output { return p.Close(now, o) })
		step(func(o []session.Output) []session.Output { return m.Close(now, o) })
		step(func(o []session.Output) []session.Output { return m.Refuse(now, o) })
	}
}
