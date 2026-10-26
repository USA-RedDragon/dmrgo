package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func newPool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	p := make([]layer2.RawBurst, 0, 64)
	for _, in := range call(t, 1, 9, 3191868, 2, enums.Slot1, bs.Source{}, 0) {
		p = append(p, in.raw)
	}
	p = append(p, wakeup(t, 1), preCSBK(t, 3120005), preCSBK(t, 3120001), header(t, 9, 3120001))
	for _, name := range []string{"d-sms.bin", "m-sms.bin"} {
		p = append(p, loadBin(t, name)...)
	}
	return p
}

type script struct {
	cfg bs.Config
	ops []op
	seq [4]uint16
}

type op struct {
	kind uint8
	at   time.Duration
	slot enums.Slot
	from bs.Source
	raw  layer2.RawBurst
	h    layer2.Hint
}

const (
	maxScript = 128
	maxOps    = 512
)

type fuzzInput struct {
	data             []byte
	loop, reps, step uint8
}

func parseScript(in fuzzInput, p []layer2.RawBurst) script {
	var s script
	data := in.data[:min(len(in.data), maxScript)]
	if len(data) < 2 {
		return s
	}
	s.cfg = bs.Config{Mode: bs.Mode(data[0] & 1), CACH: data[0]&2 != 0, Activation: bs.Activation(data[0] >> 2 & 1),
		ColorCode: 1, CallHang: time.Duration(data[0]>>3) * 100 * time.Millisecond, ChannelAuth: bs.ChannelAuth(data[1] >> 4 & 3 % 3)}
	if data[1]&0x80 != 0 {
		s.cfg.ColorCode = enums.ColorCode(data[1] & 0xF)
	}
	if data[1]&0x40 != 0 {
		s.cfg.Jitter = -1
	}
	ops := data[2 : 2+(len(data)-2)/3*3]
	loop := ops[len(ops)-3*min(int(in.loop), len(ops)/3):]
	s.ops = make([]op, 0, min(len(ops)/3+int(in.reps)*len(loop)/3, maxOps))
	var now time.Duration
	for i := 0; i < len(ops); i += 3 {
		s.add(&now, p, ops[i], ops[i+1], ops[i+2])
	}
	for r := range int(in.reps) {
		k := byte(r+1) * in.step
		for i := 0; i < len(loop) && len(s.ops) < maxOps; i += 3 {
			s.add(&now, p, loop[i], loop[i+1]+k, loop[i+2])
		}
	}
	return s
}

func (s *script) add(now *time.Duration, p []layer2.RawBurst, a, b, c byte) {
	*now += time.Duration(c) * 5 * time.Millisecond
	o := op{kind: a % 6, at: *now, slot: enums.Slot(a>>3&1 + 1), raw: p[int(b)%len(p)]}
	if src := a >> 4 & 3; src == 0 {
		o.from.RF = true
	} else {
		o.from.Peer = uint32(src)
	}
	if a&0x40 != 0 {
		o.raw[b%33] ^= 1 << (c % 8)
	}
	o.h = layer2.Hint{Slot: o.slot, SyncLost: a&0x80 != 0 && b&1 != 0, End: layer2.EndHint(b >> 6 % 3), At: *now}
	if src := a >> 4 & 3; src != 0 && c&0x20 == 0 {
		s.seq[src]++
		o.h.Seq, o.h.HasSeq = s.seq[src]-uint16(c>>6), true
		if c&0x10 != 0 {
			o.h.SeqBits, o.h.Seq = 8, o.h.Seq&0xFF
		}
	}
	s.ops = append(s.ops, o)
}

func decoded(b *layer2.Burst, raw *layer2.RawBurst) *layer2.Burst {
	if b.DecodeFromBytes(*raw) != nil {
		return nil
	}
	return b
}

func runScript(t *testing.T, p []layer2.RawBurst, in fuzzInput) {
	t.Helper()
	s := parseScript(in, p)
	r := newRepeater(t, &s.cfg)
	out := make([]bs.Output, 0, bs.MaxOutputs)
	b := layer2.NewBurst()
	dec := layer2.NewBurst()
	for i := range s.ops {
		o := &s.ops[i]
		var err error
		switch o.kind {
		case 0, 1:
			out = r.Inbound(o.at, &o.raw, o.h, o.from, out[:0])
		case 2:
			out = r.Decoded(o.at, decoded(dec, &o.raw), o.h, o.from, out[:0])
		case 3:
			out = r.Tick(o.at, out[:0])
		case 4:
			out = r.Lost(o.at, o.slot, o.from, out[:0])
		case 5:
			err = r.Reject(o.slot)
			out = out[:0]
		}
		if err != nil {
			t.Fatalf("op %d: %v", i, err)
		}
		if len(out) > bs.MaxOutputs {
			t.Fatalf("op %d: %d outputs", i, len(out))
		}
		for j := range out {
			checkOutput(t, b, &s.cfg, &out[j])
		}
	}
}

func checkOutput(t *testing.T, b *layer2.Burst, cfg *bs.Config, o *bs.Output) {
	t.Helper()
	if o.Kind != bs.OutputBurst {
		return
	}
	if o.Slot != enums.Slot1 && o.Slot != enums.Slot2 || o.To == 0 {
		t.Fatalf("burst on %v to %v", o.Slot, o.To)
	}
	if o.HasCACH != (cfg.Mode == bs.ModeRF && cfg.CACH && o.To&bs.ToRF != 0) {
		t.Fatalf("HasCACH %v in mode %v", o.HasCACH, cfg.Mode)
	}
	cc := cfg.ColorCode
	err := b.DecodeFromBytesWith(o.Burst, &layer2.DecodeOptions{ColorCode: &cc})
	if b.Kind == layer2.KindVoice && !b.HasEmbeddedSignalling() {
		return
	}
	if err != nil || b.Kind == layer2.KindData && b.SlotType.FEC.Uncorrectable {
		t.Fatalf("output burst: %v, %v", err, b.SlotType)
	}
}

func seeds() []fuzzInput {
	return []fuzzInput{
		{data: []byte{0x02, 0x01, 0, 0, 12, 3, 0, 6}, loop: 2, reps: 39, step: 1},
		{data: []byte{0x01, 0x01, 0x10, 0, 6, 0x28, 0, 2}, loop: 2, reps: 39, step: 1},
		{data: []byte{0x06, 0x01, 0, 32, 1, 3, 0, 255, 3, 0, 255}},
		{data: []byte{0x02, 0x01, 0, 33, 12, 0, 34, 12, 0, 35, 12, 0, 36, 12, 0, 37, 12, 0, 38, 12, 3, 0, 60}},
		{data: []byte{0x03, 0x8F, 0x10, 0, 1, 5, 0, 1, 0x50, 1, 12, 4, 0, 1}},
		{data: []byte{0x02, 0x11, 0, 0, 12, 3, 0, 60, 0, 0, 12, 3, 0, 60, 0, 50, 30, 3, 0, 60, 0, 51, 2, 3, 0, 255}},
		{data: []byte{0x02, 0x21, 0, 0, 12, 3, 0, 60, 0, 0, 12, 0x10, 53, 6, 3, 0, 60, 3, 0, 255}},
	}
}

func FuzzRepeater(f *testing.F) {
	p := newPool(f)
	for _, s := range seeds() {
		f.Add(s.data, s.loop, s.reps, s.step)
	}
	f.Fuzz(func(t *testing.T, data []byte, loop, reps, step uint8) {
		runScript(t, p, fuzzInput{data: data, loop: loop, reps: reps, step: step})
	})
}

func corpusFiles(t *testing.T, name string) []fuzzInput {
	t.Helper()
	var out []fuzzInput
	var in fuzzInput
	fuzztest.Corpus(t, name, func() { out = append(out, in) }, &in.data, &in.loop, &in.reps, &in.step)
	return out
}
