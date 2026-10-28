package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func txPool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	var p []layer2.RawBurst
	var rd packet.ResponseData
	rd.Retry[0], rd.Retry[3] = true, true
	for _, r := range []packet.Response{packet.ResponseACK, packet.ResponseSACK, packet.ResponseNACKPacketCRC,
		packet.ResponseNACKOutOfSequence, packet.ResponseNACKMemoryFull} {
		for ns := range uint8(8) {
			ch := confirmedHeader(t, ns)
			h := confirmedOf(t, &ch)
			h.BlocksToFollow = 7
			resp, err := packet.NewResponse(&h, r, &rd)
			if err != nil {
				t.Fatal(err)
			}
			b, err := packet.Bursts(&resp, 0, layer2.SyncSourceBS)
			if err != nil {
				t.Fatal(err)
			}
			p = append(p, b...)
		}
	}
	return p
}

func txMessages(t testing.TB) []*data.Message {
	t.Helper()
	u := &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		LLIDDestination: dstID, LLIDSource: srcID}), Rate: enums.DataTypeRate1, Payload: payload(30)}
	dd := &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
		LLIDDestination: dstID, LLIDSource: srcID, SARQ: true}), Rate: enums.DataTypeRate12, Payload: payload(20)}
	return []*data.Message{message(t, 60), message(t, 10), u, dd, message(t, 500)}
}

const maxTxOps = 128

type txInput struct {
	c0, c1, c2, step uint8
	ops              []byte
}

type txOp struct {
	kind, step, arg uint8
}

type txDecoder struct {
	ops  []byte
	step uint8
	rep  int
	op   txOp
}

func newTxDecoder(in *txInput) txDecoder {
	return txDecoder{ops: in.ops[:min(len(in.ops), maxTxOps)], step: in.step}
}

func (d *txDecoder) next() (txOp, bool) {
	if d.rep > 0 {
		d.rep--
		return d.op, true
	}
	if len(d.ops) == 0 {
		return txOp{}, false
	}
	c := d.ops[0]
	n := 1 + int(c>>3&1) + int(c>>4&1)
	if n > len(d.ops) {
		d.ops = nil
		return txOp{}, false
	}
	d.op = txOp{kind: c & 7 % 6, step: d.step}
	i := 1
	if c&0x08 != 0 {
		d.op.step = d.ops[i]
		i++
	}
	if c&0x10 != 0 {
		d.op.arg = d.ops[i]
	}
	d.ops = d.ops[n:]
	d.rep = int(c >> 5)
	return d.op, true
}

func checkTxScript(t *testing.T, in *txInput, p []layer2.RawBurst, msgs []*data.Message) {
	t.Helper()
	c0, c1, c2 := in.c0, in.c1, in.c2
	s, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1,
		Window: int(c2 % 10),
		Access: enums.Access(c0 & 3), Preambles: int(c0 >> 2 & 7), Retries: int(c1 & 7),
		Resync: c0&0x20 != 0, Seed: uint64(c1), Holdoff: time.Duration(c1>>3) * 50 * time.Millisecond,
		IdleSearch: time.Duration(int8(c0&0xC0)) * time.Millisecond, MaxBlocks: int(c1 >> 6 * 16),
	})
	if err != nil {
		return
	}
	out := make([]data.Output, 0, data.MaxOutputs)
	var now time.Duration
	active := 0
	d := newTxDecoder(in)
	for i := 0; ; i++ {
		op, ok := d.next()
		if !ok {
			break
		}
		k, arg := op.kind, op.arg
		now += time.Duration(op.step) * time.Millisecond
		out = out[:0]
		switch k {
		case 0:
			if s.Send(now, msgs[int(arg)%len(msgs)]) == nil {
				active++
			}
		case 1:
			if at, ok := s.Deadline(); ok && at > now {
				now = at
			}
			out = s.Tick(now, out)
		case 2:
			s.Channel(now, arg&1 != 0)
		case 3:
			raw := p[int(arg)%len(p)]
			if arg&0x80 != 0 {
				raw[int(arg)%33] ^= 0x10
			}
			out = s.Burst(now, &raw, layer2.Hint{}, out)
		case 4:
			out = s.Cancel(now, out)
		case 5:
			s.Reset()
			active = 0
		}
		if len(out) > data.MaxOutputs {
			t.Fatalf("op %d: %d outputs", i, len(out))
		}
		for _, o := range out {
			if o.Kind != data.OutputDone {
				continue
			}
			if active == 0 {
				t.Fatalf("op %d: a message ended twice", i)
			}
			active--
		}
		if (active == 0) != (s.State() == data.SenderIdle) {
			t.Fatalf("op %d: state %v with %d messages", i, s.State(), active)
		}
	}
}

func txSeeds() []txInput {
	seeds := make([]txInput, 0, 3)
	for _, cfg := range [][3]byte{{0, 0, 0}, {0x2A, 0x13, 3}, {0xC2, 0xFF, 8}} {
		ops := make([]byte, 0, 64)
		for i := range 40 {
			switch {
			case i%10 == 0:
				ops = append(ops, 0x18, 1, byte(i/10))
			case i%7 == 3:
				ops = append(ops, 0x1B, 20, byte(i))
			case i%11 == 5:
				ops = append(ops, 0x1A, 5, byte(i))
			default:
				ops = append(ops, 1)
			}
		}
		seeds = append(seeds, txInput{c0: cfg[0], c1: cfg[1], c2: cfg[2], step: 60, ops: ops})
	}
	return seeds
}

func FuzzSender(f *testing.F) {
	p, msgs := txPool(f), txMessages(f)
	for _, s := range txSeeds() {
		f.Add(s.c0, s.c1, s.c2, s.step, s.ops)
	}
	f.Fuzz(func(t *testing.T, c0, c1, c2, step uint8, ops []byte) {
		checkTxScript(t, &txInput{c0: c0, c1: c1, c2: c2, step: step, ops: ops}, p, msgs)
	})
}
