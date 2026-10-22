package bs_test

import (
	"os"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type input struct {
	at   time.Duration
	slot enums.Slot
	from bs.Source
	raw  layer2.RawBurst
}

func call(t testing.TB, cc enums.ColorCode, target, source uint32, superframes int, s enums.Slot, from bs.Source, start time.Duration) []input {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, target, source, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: cc, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	out := tx.AppendHeaders(nil)
	for i := range superframes * layer2.VoiceBurstsPerSuperframe * layer2.VoiceSlotsPerBurst {
		var f vocoder.AMBEFrame
		f[i%49/8] = 0x80 >> (i % 49 % 8)
		v, err := vocoder.EncodeAMBE72(f)
		if err != nil {
			t.Fatal(err)
		}
		if out, err = tx.AppendSlots(out, v); err != nil {
			t.Fatal(err)
		}
	}
	if out, err = tx.AppendTerminate(out); err != nil {
		t.Fatal(err)
	}
	return timed(out, s, from, start)
}

func timed(bursts []layer2.RawBurst, s enums.Slot, from bs.Source, start time.Duration) []input {
	in := make([]input, len(bursts))
	for i := range bursts {
		in[i] = input{at: start + time.Duration(i)*60*time.Millisecond, slot: s, from: from, raw: bursts[i]}
	}
	return in
}

func merge(a, b []input) []input {
	out := make([]input, 0, len(a)+len(b))
	for len(a) > 0 || len(b) > 0 {
		if len(b) == 0 || len(a) > 0 && a[0].at <= b[0].at {
			out, a = append(out, a[0]), a[1:]
		} else {
			out, b = append(out, b[0]), b[1:]
		}
	}
	return out
}

func run(t *testing.T, r *bs.Repeater, in []input, end time.Duration) []bs.Output {
	t.Helper()
	return runWith(t, r, in, end, nil)
}

func runWith(t *testing.T, r *bs.Repeater, in []input, end time.Duration, scratch *layer2.Burst) []bs.Output {
	t.Helper()
	out := make([]bs.Output, 0, bs.MaxOutputs)
	var got []bs.Output
	keep := func() {
		if len(out) > bs.MaxOutputs {
			t.Fatalf("%d outputs in one call", len(out))
		}
		for i := range out {
			checkHint(t, &out[i])
		}
		got = append(got, out...)
	}
	now := time.Duration(0)
	for now <= end {
		for len(in) > 0 && in[0].at <= now {
			h := layer2.Hint{Slot: in[0].slot}
			if err := bs.CheckSlot(h.Slot); err != nil {
				t.Fatal(err)
			}
			if scratch != nil {
				var b *layer2.Burst
				if scratch.DecodeFromBytes(in[0].raw) == nil {
					b = scratch
				}
				out = r.Decoded(in[0].at, b, h, in[0].from, out[:0])
			} else {
				out = r.Inbound(in[0].at, &in[0].raw, h, in[0].from, out[:0])
			}
			keep()
			in = in[1:]
		}
		for {
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			out = r.Tick(now, out[:0])
			keep()
			if len(out) == 0 {
				break
			}
		}
		now += 10 * time.Millisecond
	}
	return got
}

func checkHint(t *testing.T, o *bs.Output) {
	t.Helper()
	if o.Kind != bs.OutputBurst {
		return
	}
	h := &o.Hint
	b := layer2.NewBurst()
	_ = b.DecodeFromBytes(o.Burst)
	_, placed := h.VoiceBurst.Index()
	switch {
	case h.Slot != o.Slot:
		t.Fatalf("hint slot %v, output slot %v", h.Slot, o.Slot)
	case h.Kind == layer2.KindData && (!h.HasDataType || b.Kind != layer2.KindData || b.SlotType.DataType != h.DataType):
		t.Fatalf("hint %+v for %v %v", *h, b.Kind, b.SlotType.DataType)
	case h.Kind == layer2.KindVoice && (!placed || (h.VoiceBurst == enums.VoiceBurstA) != (b.VoiceBurst == enums.VoiceBurstA)):
		t.Fatalf("hint %+v for voice burst %v", *h, b.VoiceBurst)
	case h.Kind != layer2.KindData && h.Kind != layer2.KindVoice:
		t.Fatalf("hint %+v", *h)
	}
}

func filter(out []bs.Output, k bs.OutputKind) []bs.Output {
	var got []bs.Output
	for _, o := range out {
		if o.Kind == k {
			got = append(got, o)
		}
	}
	return got
}

func wakeup(t testing.TB, cc enums.ColorCode) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKBSOutboundActivation, RawPayload: [8]byte{0, 0, 0, 0, 1, 0x30, 0xB4, 0x3C}}
	b, err := layer2.BuildBurst(&c, cc, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadBin(t testing.TB, name string) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile("../layer2/testdata/" + name)
	if err != nil || len(data)%33 != 0 {
		t.Fatalf("read %s: %v", name, err)
	}
	out := make([]layer2.RawBurst, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		out = append(out, layer2.RawBurst(data[i:i+33]))
	}
	return out
}

func newRepeater(t *testing.T, cfg *bs.Config) *bs.Repeater {
	t.Helper()
	r, err := bs.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
