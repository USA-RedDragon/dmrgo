package data_test

import (
	"math/bits"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func pool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	var p []layer2.RawBurst
	for _, name := range []string{"d-sms", "h-sms", "m-sms"} {
		b, err := os.ReadFile(filepath.Join("..", "layer2", "testdata", name+".bin"))
		if err != nil {
			t.Fatal(err)
		}
		for off := 0; off+33 <= len(b); off += 33 {
			p = append(p, layer2.RawBurst(b[off:off+33]))
		}
	}
	xpr, err := parseTimed("testdata/anytone_sms_xpr.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range xpr[:20] {
		p = append(p, b.raw)
	}
	p = append(p, build(t, confirmedHeader(t, 1), payload(50), enums.DataTypeRate1)...)
	p = append(p, build(t, confirmedHeader(t, 2), payload(30), enums.DataTypeRate12)...)
	for ns := range uint8(3) {
		h := confirmedHeader(t, ns)
		editConfirmed(t, &h, func(c *pdu.ConfirmedDataHeader) { c.ResponseRequested = ns == 2 })
		p = append(p, build(t, h, payload(20), enums.DataTypeRate12)...)
	}
	for _, h := range []pdu.DataHeader{
		sapHeader(t, enums.SAPShortData, 0, false),
		sapHeader(t, enums.SAPShortData, 1, false),
		sapHeader(t, enums.SAPShortData, 4, false),
		sapHeader(t, enums.SAPShortData, 4, true),
		sapHeader(t, enums.SAPUDPIPHeaderCompression, 0, false),
	} {
		p = append(p, build(t, h, payload(20), enums.DataTypeRate34)...)
	}
	udt := setHeader(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{
		LLIDDestination: dstID, LLIDSource: srcID, UAB: 0, Opcode: pdu.UDTOpcodeInboundHeader})
	blocks := [][12]byte{{1, 2, 3}}
	pdu.SetMBCLastBlockCRC(blocks)
	for _, d := range []layer2.Data{&udt, &pdu.Rate12Data{Data: blocks[0]}} {
		b, err := layer2.BuildBurst(d, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		p = append(p, b)
	}
	voice, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{}, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	idle, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return append(p, voice, idle)
}

type rxOp struct {
	kind uint8
	at   time.Duration
	raw  layer2.RawBurst
	hint layer2.Hint
}

type rxScript struct {
	cfg data.ReceiverConfig
	cc  enums.ColorCode
	ops []rxOp
}

const maxRxOps = 128

type rxInput struct {
	c0, c1, step uint8
	ops          []byte
}

func parseRxScript(in rxInput, p []layer2.RawBurst) *rxScript {
	s := &rxScript{}
	c0, c1 := in.c0, in.c1
	s.cfg.Respond = c0&1 != 0
	if c0&2 != 0 {
		s.cfg.Address = dstID
	}
	if c0&4 != 0 {
		s.cc = enums.ColorCode(c1 & 0x0F)
		s.cfg.ColorCode = &s.cc
	}
	s.cfg.AcceptBadCRC = c0&8 != 0
	s.cfg.Source = layer2.SyncSource(c0 >> 4 & 1)
	s.cfg.MaxBlocks = int(c1 >> 4 * 8)
	s.cfg.Lost = time.Duration(int8(c0&0xE0)) * time.Millisecond
	s.cfg.Hang = time.Duration(int8(c1)) * 3 * time.Millisecond
	ops := in.ops[:min(len(in.ops), maxRxOps)]
	var at time.Duration
	var seq uint16
	idx := -1
	for i := 0; i < len(ops); {
		c := ops[i]
		i++
		if i+bits.OnesCount8(c&0xF0) > len(ops) {
			break
		}
		idx++
		if c&0x10 != 0 {
			idx = int(ops[i])
			i++
		}
		step := in.step
		if c&0x20 != 0 {
			step = ops[i]
			i++
		}
		var flip, ext byte
		if c&0x40 != 0 {
			flip = 0x80 | ops[i]
			i++
		}
		if c&0x80 != 0 {
			ext = ops[i]
			i++
		}
		at += time.Duration(step) * time.Millisecond
		op := rxOp{kind: c & 3, at: at, raw: p[idx%len(p)]}
		if flip&0x80 != 0 {
			bit := int(flip&0x7F) * 2
			op.raw[bit/8] ^= 1 << (bit % 8)
		}
		seq += uint16(ext>>2&3) + 1
		op.hint = layer2.Hint{
			SyncLost: c&0x04 != 0,
			End:      layer2.EndHint(ext & 3 % 3),
			HasSeq:   c&0x08 != 0,
			Seq:      seq,
		}
		s.ops = append(s.ops, op)
	}
	return s
}

func runRxOp(r *data.Receiver, op *rxOp, dst []data.Event) []data.Event {
	switch op.kind {
	case 1:
		if at, ok := r.Deadline(); ok {
			return r.Tick(max(at, op.at), dst)
		}
		return r.Tick(op.at, dst)
	case 2:
		return r.Lost(op.at, dst)
	case 3:
		if op.hint.SyncLost && op.hint.HasSeq {
			r.Reset()
			return dst
		}
	}
	return r.Burst(op.at, &op.raw, op.hint, dst)
}

func checkRxScript(t *testing.T, in rxInput, p []layer2.RawBurst) {
	t.Helper()
	s := parseRxScript(in, p)
	r, err := data.NewReceiver(&s.cfg)
	if err != nil {
		return
	}
	ev := make([]data.Event, 0, data.MaxEvents)
	for i := range s.ops {
		ev = runRxOp(r, &s.ops[i], ev[:0])
		if len(ev) > data.MaxEvents {
			t.Fatalf("op %d: %d events, more than MaxEvents", i, len(ev))
		}
		for _, e := range ev {
			if e.Kind < data.EventCallStart || e.Kind > data.EventReassembled {
				t.Fatalf("op %d: event kind %v", i, e.Kind)
			}
			if e.Kind == data.EventCallEnd && e.Reason == 0 {
				t.Fatalf("op %d: call end without a reason", i)
			}
			if st := e.Stats; e.Kind == data.EventCallEnd && st.Delivered+st.Duplicates+st.OutOfSequence > st.Packets {
				t.Fatalf("op %d: stats %+v deliver or refuse more packets than complete", i, st)
			}
		}
		if at, ok := r.Deadline(); ok && r.State() == data.StateIdle {
			t.Fatalf("op %d: deadline %v in state Idle", i, at)
		}
	}
}

func rxSeeds() []rxInput {
	seeds := make([]rxInput, 0, 4)
	for _, cfg := range [][2]byte{{0, 0}, {3, 0}, {0x0F, 0x17}, {0x23, 0x80}} {
		ops := make([]byte, 0, 72)
		for i := range 64 {
			switch {
			case i%17 == 5:
				ops = append(ops, 1)
			case i%23 == 7:
				ops = append(ops, 0x40, byte(i))
			case i%29 == 3:
				ops = append(ops, 0x08)
			default:
				ops = append(ops, 0)
			}
		}
		seeds = append(seeds, rxInput{c0: cfg[0], c1: cfg[1], step: 60, ops: ops})
	}
	return seeds
}

func FuzzReceiver(f *testing.F) {
	p := pool(f)
	for _, s := range rxSeeds() {
		f.Add(s.c0, s.c1, s.step, s.ops)
	}
	f.Fuzz(func(t *testing.T, c0, c1, step uint8, ops []byte) {
		checkRxScript(t, rxInput{c0: c0, c1: c1, step: step, ops: ops}, p)
	})
}
