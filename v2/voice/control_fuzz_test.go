package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func controlPool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	var p []layer2.RawBurst
	for _, src := range []uint32{3120005, 3120006} {
		lc, err := pdu.NewVoiceLC(true, 9, src, layer3.ServiceOptions{})
		if err != nil {
			t.Fatal(err)
		}
		vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
		if err != nil {
			t.Fatal(err)
		}
		p = append(p, vt.Header(), vt.Terminator())
		if b, err := vt.AddBurst([layer2.VoiceSlotsPerBurst][9]byte{}); err == nil {
			p = append(p, b)
		}
	}
	idle, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	p = append(p, idle)
	for _, cmd := range []enums.RCCommand{enums.RCCeaseTransmissionCommand, enums.RCCeaseTransmissionRequest} {
		in, err := voice.NewInterrupter(&voice.InterrupterConfig{ColorCode: 1})
		if err != nil {
			t.Fatal(err)
		}
		_ = in.Interrupt(0, cmd)
		p = append(p, in.Tick(0, nil)[0].Burst)
	}
	return p
}

const (
	maxControl    = 128
	maxControlOps = 512
)

type controlInput struct {
	c0, c1         uint8
	ops            []byte
	from, to, reps uint8
}

func controlScript(t testing.TB, p []layer2.RawBurst, in *controlInput) {
	t.Helper()
	c0, c1 := in.c0, in.c1
	cfg := voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Access: enums.Access(c0 % 3), IdleSearch: time.Duration(int8(c0)) * 10 * time.Millisecond,
		Interruptible: c0&0x08 != 0, ChanAuth: time.Duration(c1&0x0F) * 60 * time.Millisecond, HoldoffMax: time.Duration(int8(c1)>>4) * 100 * time.Millisecond, Seed: uint64(c1)}
	tx, err := voice.NewTransmitter(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	irq, err := voice.NewInterrupter(&voice.InterrupterConfig{ColorCode: 1, Retries: int(c0 >> 6), Repeats: int(c1>>6) + 1, Wait: time.Duration(int8(c1<<2)) * 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]voice.Output, 0, voice.MaxOutputs)
	var b layer2.Burst
	var now time.Duration
	check := func(op byte) {
		if len(out) > voice.MaxOutputs {
			t.Fatalf("op %d: %d outputs", op, len(out))
		}
		for _, o := range out {
			if o.Kind == voice.OutputBurst && b.DecodeFromBytes(o.Burst) != nil && o.Hint.VoiceBurst == enums.VoiceBurstUnknown {
				t.Fatalf("op %d: bad burst", op)
			}
		}
	}
	ops := in.ops[:min(len(in.ops), maxControl)]
	lo, hi, total := fuzztest.Loop(len(ops)/2, in.from, in.to, in.reps, maxControlOps)
	for s := range total {
		i := 2 * fuzztest.Index(s, lo, hi, in.reps)
		op, arg := ops[i], ops[i+1]
		now += time.Duration(arg&0x0F) * 15 * time.Millisecond
		raw := p[int(arg>>4)%len(p)]
		if op&0x80 != 0 {
			raw[int(arg)%len(raw)] ^= 1 << (op >> 4 & 7)
		}
		out = out[:0]
		switch op & 0x0F {
		case 0:
			_ = tx.Start(now, voice.Call{Group: true, Target: 9, Source: 3120005}, &voice.CallOptions{InCall: arg&0x10 != 0, HangTime: arg&0x20 != 0})
		case 1:
			_ = tx.VocoderSlot(vocoder.AMBESilenceSlot())
		case 2:
			tx.Channel(now, arg&0x10 != 0)
		case 3:
			out = tx.Burst(now, &raw, layer2.Hint{}, out)
		case 4:
			_ = tx.ReverseChannel(b.ReverseChannel)
			if b.DecodeFromBytes(raw) == nil {
				_ = tx.ReverseChannel(b.ReverseChannel)
			}
		case 5:
			tx.Stop()
		case 6:
			_ = irq.Interrupt(now, enums.RCCommand(arg&0x0F))
		case 7:
			out = irq.Burst(now, &raw, layer2.Hint{}, out)
		case 8:
			out = irq.Cancel(now, out)
		case 9:
			tx.Reset()
			irq.Reset()
		default:
			for range 32 {
				at, ok := tx.Deadline()
				if !ok || at > now {
					break
				}
				out = tx.Tick(now, out[:0])
				check(op)
			}
			for range 32 {
				at, ok := irq.Deadline()
				if !ok || at > now {
					break
				}
				out = irq.Tick(now, out[:0])
				check(op)
			}
			out = out[:0]
		}
		check(op)
	}
}

func controlSeeds() []controlInput {
	return []controlInput{
		{c0: 0x00, c1: 0x03, ops: []byte{2, 0x10, 0, 0x20, 1, 0, 15, 0x01, 3, 0x02, 15, 0x0F, 5, 0, 15, 0x0F}},
		{c0: 0x08, c1: 0x00, ops: []byte{0, 0, 1, 0, 15, 0x02, 4, 0x80, 15, 0x0F, 15, 0x0F}},
		{c0: 0x02, c1: 0x40, ops: []byte{6, 0x04, 15, 0x01, 7, 0x10, 15, 0x0F, 6, 0x05, 15, 0x0F, 8, 0}},
		{c0: 0x01, c1: 0x23, ops: []byte{2, 0x10, 0, 0x30, 15, 0x01, 3, 0x61, 15, 0x0F, 2, 0x00, 15, 0x0F, 9, 0}},
		{c0: 0x00, c1: 0x03, ops: []byte{2, 0x10, 0, 0x20, 1, 0, 1, 0, 15, 0x04, 3, 0x02, 15, 0x0F, 5, 0, 15, 0x0F}, from: 2, to: 5, reps: 40},
	}
}

func TestControlFuzzSeeds(t *testing.T) {
	t.Parallel()
	p := controlPool(t)
	for _, in := range controlSeeds() {
		controlScript(t, p, &in)
	}
}

func FuzzCallControl(f *testing.F) {
	p := controlPool(f)
	for _, in := range controlSeeds() {
		f.Add(in.c0, in.c1, in.ops, in.from, in.to, in.reps)
	}
	f.Fuzz(func(t *testing.T, c0, c1 uint8, ops []byte, from, to, reps uint8) {
		controlScript(t, p, &controlInput{c0: c0, c1: c1, ops: ops, from: from, to: to, reps: reps})
	})
}
