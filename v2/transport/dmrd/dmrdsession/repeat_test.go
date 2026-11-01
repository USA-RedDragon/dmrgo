package dmrdsession_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestRepeatedRPTKLogin(t *testing.T) {
	t.Parallel()

	ms := time.Millisecond
	const id = 3129100
	for _, c := range []struct {
		d       dmrd.Dialect
		options string
	}{{dmrd.DialectMMDVM, "TS2=91;"}, {dmrd.DialectMMDVM, ""}, {dmrd.DialectHBRP, ""}} {
		cfg := &dmrdsession.PeerConfig{Dialect: c.d, ID: id, Password: []byte("pw"), Config: mmdvmConfig(id), Options: []byte(c.options), Retry: 200 * ms}
		if c.d == dmrd.DialectHBRP {
			cfg.Config = hbrpConfig(id)
		}
		p, _ := dmrdsession.NewPeer(cfg)
		m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: c.d})
		var events []dmrdsession.Output
		toMaster := func(now time.Duration, out []dmrdsession.Output) [][]byte {
			r := make([][]byte, 0, len(out))
			for _, b := range sends(out) {
				o := m.Packet(now, b, nil)
				if f, ok := find(o, dmrdsession.OutputLogin); ok {
					o = append(o[:0], f)
					o = m.Admit(now, 7, []byte("pw"), o)
				}
				r = append(r, sends(o)...)
				for _, e := range o {
					if e.Kind != dmrdsession.OutputSend {
						events = append(events, e)
					}
				}
			}
			return r
		}
		toPeer := func(now time.Duration, acks [][]byte) []dmrdsession.Output {
			out := make([]dmrdsession.Output, 0, dmrdsession.MaxOutputs*len(acks))
			for _, a := range acks {
				out = append(out, p.Packet(now, a, nil)...)
			}
			if _, ok := find(out, dmrdsession.OutputDown); ok {
				t.Fatalf("%v %q: repeater down %+v", c.d, c.options, out)
			}
			return out
		}

		salt := toMaster(0, p.Tick(0, nil))
		k1 := toPeer(0, salt)
		a1 := toMaster(10*ms, k1)
		at, _ := p.Deadline()
		k2 := p.Tick(at, nil)
		if len(sends(k2)) != 1 || !bytes.Equal(sends(k2)[0], sends(k1)[0]) {
			t.Fatalf("%v %q: RPTK again %v", c.d, c.options, kinds(k2))
		}
		a2 := toMaster(at, k2)
		late := at + 10*ms
		steps := toPeer(late, append(a1, a2...))
		loginAt, _ := m.Deadline()
		early := toMaster(late, steps)
		if len(early) != 0 || m.State() != dmrdsession.StateConfig {
			t.Fatalf("%v %q: early steps got %d answers, state %v", c.d, c.options, len(early), m.State())
		}
		if _, ok := find(events, dmrdsession.OutputConfig); !ok {
			t.Fatalf("%v %q: no OutputConfig %v", c.d, c.options, kinds(events))
		}
		now := late
		for range 3 {
			at, _ := p.Deadline()
			now = at
			if got := toMaster(now, p.Tick(now, nil)); len(got) != 0 {
				t.Fatalf("%v %q: answer before Accept", c.d, c.options)
			}
		}
		var data dmrd.Data
		data.Repeater, data.Slot = id, 1
		dp, _ := data.AppendBinary(nil)
		if out := m.Packet(now, dp, nil); len(out) != 0 || m.State() != dmrdsession.StateConfig {
			t.Fatalf("%v %q: DMRD before Accept %v %v", c.d, c.options, kinds(out), m.State())
		}
		if at, _ := m.Deadline(); at != loginAt {
			t.Fatalf("%v %q: deadline %v, want %v", c.d, c.options, at, loginAt)
		}
		acc := m.Accept(now, nil)
		want := []dmrdsession.OutputKind{dmrdsession.OutputSend, dmrdsession.OutputUp}
		if c.options != "" {
			want = append(want, dmrdsession.OutputOptions)
		}
		sameKinds(t, kinds(acc), want...)
		if o, ok := find(acc, dmrdsession.OutputOptions); ok && (string(o.Packet) != c.options || o.ID != id) {
			t.Fatalf("%v: options %q %v", c.d, o.Packet, o.ID)
		}
		toPeer(now, sends(acc))
		if p.State() != dmrdsession.StateRunning || m.State() != dmrdsession.StateRunning {
			t.Fatalf("%v %q: states %v %v", c.d, c.options, p.State(), m.State())
		}
		at, _ = p.Deadline()
		pong := toMaster(at, p.Tick(at, nil))
		if len(pong) != 1 || len(toPeer(at, pong)) != 0 {
			t.Fatalf("%v %q: ping after Accept", c.d, c.options)
		}
		if _, ok := find(events, dmrdsession.OutputPong); !ok {
			t.Fatalf("%v %q: no OutputPong %v", c.d, c.options, kinds(events))
		}
		if out := m.Packet(at, dp, nil); count(out, dmrdsession.OutputData) != 1 {
			t.Fatalf("%v %q: DMRD after Accept %v", c.d, c.options, kinds(out))
		}
	}
}

func TestMasterEarlyStepsAfterRepeatedRPTK(t *testing.T) {
	t.Parallel()

	d := dmrd.DialectMMDVM
	tok, _ := dmrdauth.Token(d, 7, []byte("pw"))
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d})
	nak := func(out []dmrdsession.Output) bool {
		return len(out) == 1 && command(t, out[0].Packet, d) == dmrd.CommandMSTNAK
	}
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	if !nak(m.Packet(0, ctl(dmrd.CommandRPTPING, 5), nil)) {
		t.Fatal("ping before RPTK")
	}
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	m.Packet(0, rptk(5, tok), nil)
	sameKinds(t, kinds(m.Packet(0, mmdvmConfig(5), nil)), dmrdsession.OutputConfig)
	opts := make([]byte, 0, 32)
	opts = append(opts, ctl(dmrd.CommandRPTO, 5)...)
	opts = append(opts, "TS1=1;"...)
	for _, b := range [][]byte{ctl(dmrd.CommandRPTPING, 5), opts, ctl(dmrd.CommandRPTPING, 5)} {
		if out := m.Packet(0, b, nil); len(out) != 0 {
			t.Fatalf("%q in StateConfig: %v", b, kinds(out))
		}
	}
	opts[len(opts)-2] = '2'
	m.Packet(0, opts, nil)
	opts[len(opts)-2] = '9'
	if !nak(m.Packet(0, ctl(dmrd.CommandRPTPING, 6), nil)) || !nak(m.Packet(0, ctl(dmrd.CommandRPTO, 6), nil)) {
		t.Fatal("another ID in StateConfig")
	}
	out := m.Accept(0, nil)
	if o, ok := find(out, dmrdsession.OutputOptions); !ok || string(o.Packet) != "TS1=2;" {
		t.Fatalf("held options %v %q", kinds(out), o.Packet)
	}
	if out := m.Accept(0, nil); len(out) != 0 {
		t.Fatalf("second Accept %v", kinds(out))
	}
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	m.Packet(0, mmdvmConfig(5), nil)
	sameKinds(t, kinds(m.Accept(0, nil)), dmrdsession.OutputSend, dmrdsession.OutputUp)
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	m.Packet(0, opts, nil)
	m.Reset()
	m.Resume(0, 5, nil)
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	m.Packet(0, mmdvmConfig(5), nil)
	sameKinds(t, kinds(m.Accept(0, nil)), dmrdsession.OutputSend, dmrdsession.OutputUp)
}
