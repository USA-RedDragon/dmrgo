package rxtiers

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

const audioRate = 9600

type digest struct {
	sent     map[layer2.RawBurst]bool
	h, n, ok uint32
}

func (d *digest) mix(b byte) { d.h = (d.h ^ uint32(b)) * 16777619 }

func (d *digest) on(bs []rf.Burst) {
	for i := range bs {
		b := &bs[i]
		d.n++
		if d.sent[b.Data] || d.sent[b.Fixed] {
			d.ok++
		}
		for _, c := range b.Data {
			d.mix(c)
		}
		for _, c := range b.CACH {
			d.mix(c)
		}
		v := uint64(int64(math.Round(b.Seconds * 1e6)))
		for range 8 {
			d.mix(byte(v))
			v >>= 8
		}
		d.mix(byte(b.Slot))
	}
}

func stream(t *testing.T) ([]int16, map[layer2.RawBurst]bool) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 1234567, Dest: 9, BS: 765432, Group: true, Wakeups: 1, Message: "DMRGO TIERS", Superframes: 3, BSSourced: true, Slot: 1, Preambles: 2}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	sent := map[layer2.RawBurst]bool{}
	for _, s := range b.Slots {
		sent[s.Burst] = true
	}
	fm, _ := rftx.ModulateBS(b.Slots)
	step := rftx.BasebandRate / audioRate
	out := make([]int16, len(fm)/step)
	seed := uint32(1)
	for i := range out {
		var s float64
		for _, v := range fm[i*step : (i+1)*step] {
			s += v
		}
		seed = seed*1664525 + 1013904223
		noise := float64(int32(seed)>>16) / 32768 * 300
		out[i] = int16(math.Round((s/float64(step) + noise) * 4))
	}
	return out, sent
}

func TestTiersDecode(t *testing.T) {
	t.Parallel()

	a, sent := stream(t)
	for _, c := range []struct {
		tier     rf.Tier
		n, ok, h uint32
	}{
		{rf.Tier1, 48, 48, 0xf98ad06b},
		{rf.Tier2, 48, 47, 0xa10f4ebe},
	} {
		d := digest{sent: sent, h: 2166136261}
		r, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: audioRate, Tier: c.tier, OnBursts: d.on})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(a); k += 256 {
			if _, err := r.Write(a[k:min(k+256, len(a))]); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.Flush(); err != nil {
			t.Fatal(err)
		}
		if d.n != c.n || d.ok != c.ok || d.h != c.h {
			t.Errorf("tier %d: %d bursts, %d exact, digest %#x, want %d, %d and %#x", c.tier-rf.Tier0, d.n, d.ok, d.h, c.n, c.ok, c.h)
		}
	}
}

func iqStream(t *testing.T) ([]byte, map[layer2.RawBurst]bool) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 1234567, Dest: 9, BS: 765432, Group: true, Wakeups: 1, Message: "DMRGO TIERS", Superframes: 3, BSSourced: true, Slot: 1, Preambles: 2}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	sent := map[layer2.RawBurst]bool{}
	for _, s := range b.Slots {
		sent[s.Burst] = true
	}
	fm, env := rftx.ModulateBS(b.Slots)
	pad := rftx.BasebandRate / 5
	out := make([]byte, 4*(len(fm)+2*pad))
	seed := uint32(7)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(int32(seed)>>16) / 32768 * 0.6
	}
	var ph float64
	for i := 0; i < len(fm)+2*pad; i++ {
		var re, im float64
		if k := i - pad; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + 2500) / rftx.BasebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		re, im = re+noise(), im+noise()
		binary.LittleEndian.PutUint16(out[4*i:], uint16(int16(math.Round(re*8000))))
		binary.LittleEndian.PutUint16(out[4*i+2:], uint16(int16(math.Round(im*8000))))
	}
	return out, sent
}

func TestIQTiersDecode(t *testing.T) {
	t.Parallel()

	x, sent := iqStream(t)
	for _, c := range []struct {
		tier     rf.Tier
		n, ok, h uint32
	}{
		{rf.Tier1, 48, 48, 0x4c76a7f4},
		{rf.Tier2, 48, 48, 0x9026a1c3},
	} {
		d := digest{sent: sent, h: 2166136261}
		r, err := rf.NewReceiver(&rf.RxConfig{Rate: rftx.BasebandRate, Format: rf.IQFormatCS16, Tier: c.tier, OnBursts: d.on})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(x); k += 1024 {
			if _, err := r.Write(x[k:min(k+1024, len(x))]); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.Flush(); err != nil {
			t.Fatal(err)
		}
		if d.n != c.n || d.ok != c.ok || d.h != c.h {
			t.Errorf("I/Q tier %d: %d bursts, %d exact, digest %#x, want %d, %d and %#x", c.tier-rf.Tier0, d.n, d.ok, d.h, c.n, c.ok, c.h)
		}
	}
}
