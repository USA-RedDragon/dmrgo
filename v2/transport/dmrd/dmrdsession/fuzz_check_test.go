package dmrdsession_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

const (
	maxOps   = 128
	maxRaw   = 128
	maxSteps = 512

	repeatOctet = 4
)

type fuzzInput struct {
	ops, raw       []byte
	from, to, reps uint8
}

type fuzzPacket struct {
	pk  []byte
	dt  time.Duration
	raw bool
}

func fuzzPool() [][]byte {
	ctl := func(c dmrd.Command, id uint32) []byte {
		b, _ := (&dmrd.Control{Command: c, ID: id}).AppendBinary(nil, dmrd.DialectMMDVM)
		return b
	}
	tok, _ := dmrdauth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	rptk, _ := (&dmrd.Control{Command: dmrd.CommandRPTK, ID: 1234, Token: tok}).AppendBinary(nil, dmrd.DialectMMDVM)
	data := func(h string) []byte {
		b, _ := hex.DecodeString(h)
		d, _ := dmrd.DecodeData(b)
		d.Repeater = 1234
		b, _ = d.AppendBinary(nil)
		return b
	}
	return [][]byte{
		ctl(dmrd.CommandRPTL, 1234),
		rptk,
		mmdvmConfig(1234),
		ctl(dmrd.CommandRPTPING, 1234),
		ctl(dmrd.CommandRPTACK, 7),
		ctl(dmrd.CommandRPTACK, 1234),
		ctl(dmrd.CommandMSTPONG, 1234),
		ctl(dmrd.CommandMSTNAK, 1234),
		[]byte("MSTACK000004D20A7ED498"),
		[]byte("MSTACK000004D2"),
		[]byte("RPTPONG000004D2"),
		[]byte("MSTCL000004D2"),
		[]byte("RPTL000004D2"),
		[]byte("MSTPING000004D2"),
		hbrpConfig(1234),
		[]byte("RPTCL000004D2"),
		ctl(dmrd.CommandRPTO, 1234),
		ctl(dmrd.CommandRPTCL, 1234),
		ctl(dmrd.CommandMSTCL, 1234),
		[]byte("BCKA"),
		data("444d52440030b43c00005b003010afa1506290d020580b4015ec0f6871000d005c4d5d7f77fd757361a416203d52352421818502770000"),
		data("444d52440130b43c00005b003010af90506290d0298d0a33fd2bbc43cc28124ac5a7f7d5dd57dfd138d2514ab1ba41155039c905790000"),
	}
}

func fuzzSeeds() []fuzzInput {
	voice := fuzzPool()[21]
	return []fuzzInput{
		{ops: []byte{0, 3, 1, 3, 2, 3, 3, 3}},
		{ops: []byte{4, 3, 5, 3, 5, 3, 6, 3, 7, 3}},
		{ops: []byte{8, 3, 9, 3, 9, 3, 10, 3, 11, 3}},
		{ops: []byte{12, 3, 13, 3, 14, 3, 15, 3}},
		{ops: []byte{16, 3, 17, 3, 18, 3}},
		{},
		{ops: []byte{0x84, 200}, raw: []byte("BCKA")},
		{ops: []byte{0, 3, 1, 3, 2, 3, 3, 10, 6, 10, 17, 3}, from: 3, to: 5, reps: 40},
		{ops: []byte{0, 3, 1, 3, 2, 3, 4, 3, 5, 3, 5, 3, 5, 3, 20, 0, 21, 0, 17, 3}, from: 8, to: 9, reps: 20},
		{ops: []byte{0, 3, 1, 3, 2, 3, 4, 3, 5, 3, 5, 3, 5, 3, 20, 0, 0x80 | byte(len(voice)), 0, 17, 3}, raw: voice, from: 8, to: 9, reps: 30},
	}
}

func scriptLoop(n int, from, to, reps uint8) (lo, hi, total int) {
	lo = min(int(from), n)
	hi = max(min(int(to), n), lo)
	return lo, hi, min(n+int(reps)*(hi-lo), maxSteps)
}

func scriptIndex(s, lo, hi int, reps uint8) int {
	if s < hi {
		return s
	}
	s -= hi
	w := int(reps) * (hi - lo)
	if s < w {
		return lo + s%(hi-lo)
	}
	return hi + s - w
}

func scriptRepeat(s, lo, hi int, reps uint8) int {
	if s < hi || hi == lo || s-hi >= int(reps)*(hi-lo) {
		return 0
	}
	return (s-hi)/(hi-lo) + 1
}

func fuzzPackets(pool [][]byte, in fuzzInput, f func(p []byte, dt time.Duration)) {
	ops := in.ops[:min(len(in.ops), maxOps)]
	raw := in.raw[:min(len(in.raw), maxRaw)]
	list := make([]fuzzPacket, 0, len(ops)/2)
	for i := 0; i+1 < len(ops); i += 2 {
		sel := ops[i]
		pk, isRaw := pool[int(sel)%len(pool)], sel&0x80 != 0
		if isRaw {
			n := min(int(sel&0x7F), len(raw))
			pk, raw = raw[:n], raw[n:]
		}
		list = append(list, fuzzPacket{pk: pk, dt: time.Duration(ops[i+1]) * 500 * time.Millisecond, raw: isRaw})
	}
	lo, hi, total := scriptLoop(len(list), in.from, in.to, in.reps)
	for s := range total {
		p := list[scriptIndex(s, lo, hi, in.reps)]
		pk := append([]byte{}, p.pk...)
		if p.raw && len(pk) > repeatOctet {
			pk[repeatOctet] ^= byte(scriptRepeat(s, lo, hi, in.reps))
		}
		f(pk, p.dt)
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
			if _, err := dmrd.MMDVMConfigOf(o.Packet); err != nil {
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

func checkSession(t *testing.T, pool [][]byte, in fuzzInput) {
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
		fuzzPackets(pool, in, func(pk []byte, dt time.Duration) {
			now += dt
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Packet(now, pk, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output {
				o = m.Packet(now, pk, o)
				if m.State() == dmrdsession.StateLogin {
					o = m.Admit(now, 7, []byte("pw"), o)
				}
				if m.State() == dmrdsession.StateConfig && len(pk)%2 == 0 {
					o = m.Accept(now, o)
				}
				return o
			})
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return b.Tick(now, o) })
			step(func(o []dmrdsession.Output) []dmrdsession.Output { return b.Packet(now, pk, o) })
		})
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Resume(now, 1234, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Accept(now, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return p.Close(now, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Close(now, o) })
		step(func(o []dmrdsession.Output) []dmrdsession.Output { return m.Refuse(now, o) })
	}
}
