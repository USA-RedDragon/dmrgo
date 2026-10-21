package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func timingScript(t testing.TB, in []byte) {
	t.Helper()
	if len(in) < 1 {
		return
	}
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: uint32(in[0])<<4 | 1, DynamicID: in[0] >> 6, Slot: enums.Slot(in[0]>>5&1 + 1), Seed: uint64(in[0])})
	if err != nil {
		t.Fatal(err)
	}
	m.Join(0)
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	var b layer2.Burst
	var now time.Duration
	check := func(op byte) {
		if len(out) > csbk.MaxOutputs {
			t.Fatalf("op %d: %d outputs", op, len(out))
		}
		for _, o := range out {
			if o.Kind == csbk.OutputBurst {
				if err := b.DecodeFromBytes(o.Burst); err != nil || b.SlotType.ColorCode != csbk.AllSiteColorCode {
					t.Fatalf("op %d: burst %v", op, err)
				}
				if c, ok := b.Data.(*pdu.CSBK); !ok || c.ChannelTimingPDU == nil {
					t.Fatalf("op %d: not a CT_CSBK", op)
				}
			}
		}
		if _, _, ok := m.Leader(); ok != (m.State() >= csbk.TimingLeaderAndTimingKnown) {
			t.Fatalf("op %d: leader %v in %v", op, ok, m.State())
		}
	}
	in = in[1:]
	for i := 0; i+9 <= len(in) && i < 9*maxFuzzOps; i += 9 {
		op, f := in[i], in[i+1:i+9]
		switch op & 7 {
		case 0, 1, 2:
			p := pdu.ChannelTimingPDU{SyncAge: uint16(f[0])<<3 | uint16(f[1]>>5), Generation: f[1] & 0x1F, LeaderIdentifier: uint32(f[2])<<12 | uint32(f[3])<<4 | uint32(f[4]>>4),
				NewLeader: f[4]&8 != 0, LeaderDynamicIdentifier: f[4] >> 1 & 3, ChannelTimingOp1: f[4]&1 != 0, SourceIdentifier: uint32(f[5])<<12 | uint32(f[6])<<4 | uint32(f[7]>>4),
				SourceDynamicIdentifier: f[7] >> 1 & 3, ChannelTimingOp0: f[7]&1 != 0}
			raw, err := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKChannelTiming, ChannelTimingPDU: &p}, csbk.AllSiteColorCode, layer2.SyncSourceDirectTS1)
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
			out = m.Tick(now, out[:0])
		default:
			if op&0x10 != 0 {
				m.Reset()
			}
			m.Join(now)
		}
		check(op)
	}
}

func timingSeeds() [][]byte {
	return [][]byte{
		{0x95, 5, 54, 0, 0, 0, 0, 0, 0, 0, 5, 54, 0, 0, 0, 0, 0, 0, 0},
		{0x81, 0, 0, 0, 0, 0x70, 0x07, 0, 0, 0x76, 5, 1, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0},
		{0x42, 0, 0, 0x28, 0, 0, 0x6A, 0, 0, 0x34, 5, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x30, 1, 0, 0, 0, 0, 0, 0, 0},
		{0xC3, 0x13, 0, 0, 0, 0, 0, 0, 0, 0, 5, 120, 0, 0, 0, 0, 0, 0, 0, 0x17, 0, 0, 0, 0, 0, 0, 0, 0},
	}
}

func TestTimingFuzzSeeds(t *testing.T) {
	t.Parallel()
	for _, in := range timingSeeds() {
		timingScript(t, in)
	}
}

func FuzzTiming(f *testing.F) {
	for _, in := range timingSeeds() {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		timingScript(t, in)
	})
}
