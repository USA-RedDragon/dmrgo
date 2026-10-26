package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const maxTimingScript = 14 * 9

func timingScript(t testing.TB, cfg uint8, in []byte, from, to, reps uint8) {
	t.Helper()
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: uint32(cfg)<<4 | 1, DynamicID: cfg >> 6, Slot: enums.Slot(cfg>>5&1 + 1), Seed: uint64(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Join(0); err != nil {
		t.Fatal(err)
	}
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	var b layer2.Burst
	var now time.Duration
	check := func(op byte) {
		if len(out) > csbk.MaxOutputs {
			t.Fatalf("op %d: %d outputs", op, len(out))
		}
		for _, o := range out {
			if o.Kind == csbk.OutputBurst {
				if o.Hint.Kind != layer2.KindData || o.Hint.DataType != enums.DataTypeCSBK || o.Hint.Slot == enums.SlotUnknown {
					t.Fatalf("op %d: Hint %+v", op, o.Hint)
				}
				if err := b.DecodeFromBytes(o.Burst); err != nil || b.SlotType.ColorCode != csbk.AllSiteColorCode {
					t.Fatalf("op %d: burst %v", op, err)
				}
				if c, ok := b.Data.(*pdu.CSBK); !ok {
					t.Fatalf("op %d: not a CT_CSBK", op)
				} else if _, ok := c.AsChannelTiming(); !ok {
					t.Fatalf("op %d: not a CT_CSBK", op)
				}
			}
		}
		if _, _, ok := m.Leader(); ok != (m.State() >= csbk.TimingLeaderAndTimingKnown) {
			t.Fatalf("op %d: leader %v in %v", op, ok, m.State())
		}
	}
	in = in[:min(len(in), maxTimingScript)]
	lo, hi, total := fuzztest.Loop(len(in)/9, from, to, reps, maxFuzzOps)
	for s := range total {
		i := 9 * fuzztest.Index(s, lo, hi, reps)
		op, f := in[i], in[i+1:i+9]
		switch op & 7 {
		case 0, 1, 2:
			p := pdu.ChannelTimingPDU{SyncAge: uint16(f[0])<<3 | uint16(f[1]>>5), Generation: f[1] & 0x1F, LeaderIdentifier: uint32(f[2])<<12 | uint32(f[3])<<4 | uint32(f[4]>>4),
				NewLeader: f[4]&8 != 0, LeaderDynamicIdentifier: f[4] >> 1 & 3, ChannelTimingOp1: f[4]&1 != 0, SourceIdentifier: uint32(f[5])<<12 | uint32(f[6])<<4 | uint32(f[7]>>4),
				SourceDynamicIdentifier: f[7] >> 1 & 3, ChannelTimingOp0: f[7]&1 != 0}
			c := pdu.CSBK{LastBlock: true}
			if err := c.SetChannelTiming(p); err != nil {
				t.Fatal(err)
			}
			raw, err := layer2.BuildBurst(&c, csbk.AllSiteColorCode, layer2.SyncSourceDirectTS1)
			if err != nil {
				t.Fatal(err)
			}
			if op&0x80 != 0 {
				raw[int(f[0])%len(raw)] ^= 1 << (op >> 4 & 7)
			}
			now += time.Duration(f[2]) * 10 * time.Millisecond
			out = m.Burst(now, &raw, layer2.Hint{}, out[:0])
		case 3:
			m.Channel(now, op&0x10 != 0)
		case 4:
			out = m.Terminator(now, out[:0])
		case 5:
			now += time.Duration(f[0]) * 5 * time.Second
			for range 64 {
				at, ok := m.Deadline()
				if !ok || at > now {
					break
				}
				out = m.Tick(now, out[:0])
				check(op)
			}
		case 6:
			now += time.Duration(f[0]) * time.Millisecond
			out = timingQuiet(m, now, op, out[:0])
		default:
			if op&0x10 != 0 {
				m.Reset()
			}
			if err := m.Join(now); err != nil {
				t.Fatal(err)
			}
		}
		check(op)
	}
}

func timingQuiet(m *csbk.Timing, now time.Duration, op byte, out []csbk.Output) []csbk.Output {
	switch op >> 4 & 3 {
	case 1:
		return m.Burst(now, nil, layer2.Hint{SyncLost: op&0x40 != 0}, out)
	case 2:
		return m.Lost(now, out)
	}
	return m.Tick(now, out)
}

type timingSeed struct {
	cfg uint8
	in  []byte
}

func timingSeeds() []timingSeed {
	return []timingSeed{
		{0x95, []byte{5, 54, 0, 0, 0, 0, 0, 0, 0, 5, 54, 0, 0, 0, 0, 0, 0, 0}},
		{0x81, []byte{0, 0, 0, 0, 0x70, 0x07, 0, 0, 0x76, 5, 1, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0}},
		{0x42, []byte{0, 0, 0x28, 0, 0, 0x6A, 0, 0, 0x34, 5, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x30, 1, 0, 0, 0, 0, 0, 0, 0}},
		{0xC3, []byte{0x13, 0, 0, 0, 0, 0, 0, 0, 0, 5, 120, 0, 0, 0, 0, 0, 0, 0, 0x17, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
}

func TestTimingFuzzSeeds(t *testing.T) {
	t.Parallel()
	for _, s := range timingSeeds() {
		timingScript(t, s.cfg, s.in, 0, 0, 0)
	}
}

func FuzzTiming(f *testing.F) {
	for _, s := range timingSeeds() {
		f.Add(s.cfg, s.in, uint8(0), uint8(0), uint8(0))
	}
	f.Add(uint8(0x95), timingSeeds()[0].in, uint8(0), uint8(1), uint8(40))
	f.Fuzz(func(t *testing.T, cfg uint8, in []byte, from, to, reps uint8) {
		timingScript(t, cfg, in, from, to, reps)
	})
}
