package txtiers

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

type digest struct {
	h, n uint32
}

func (d *digest) mix(v int16) {
	d.h = (d.h ^ uint32(uint16(v))) * 16777619
	d.n++
}

func bursts() []rf.Burst {
	bs := make([]rf.Burst, 12)
	x := byte(1)
	for i := range bs {
		for j := range bs[i].Data {
			x = x*37 + 11
			bs[i].Data[j] = x
		}
		bs[i].Stream, bs[i].Slot = 1, int64(2*i)
	}
	return bs
}

func TestIQTiers(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		tier rf.Tier
		n, h uint32
	}{
		{rf.Tier1, 133360, 0x896028bd},
		{rf.Tier2, 133360, 0x787cee5},
	} {
		d := digest{h: 2166136261}
		tx, err := rf.NewTransmitter(&rf.TxConfig{Tier: c.tier, Rate: 96000, Format: rf.IQFormatCS16, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30,
			OnSamples: func(b []byte) {
				for i := 0; i+1 < len(b); i += 2 {
					d.mix(int16(uint16(b[i]) | uint16(b[i+1])<<8))
				}
			}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Write(bursts()); err != nil {
			t.Fatal(err)
		}
		if err := tx.Flush(); err != nil {
			t.Fatal(err)
		}
		if d.n != c.n || d.h != c.h {
			t.Errorf("tier %d: %d values, digest %#x, want %d and %#x", c.tier-rf.Tier0, d.n, d.h, c.n, c.h)
		}
	}
}

func TestAudioTiers(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		tier rf.Tier
		n, h uint32
	}{
		{rf.Tier1, 33340, 0x605b1a63},
		{rf.Tier2, 33340, 0x187373ef},
	} {
		d := digest{h: 2166136261}
		tx, err := rf.NewAudioTransmitter(&rf.AudioTxConfig{Tier: c.tier, Rate: 48000, Bits: 12, Unsigned: true, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30,
			OnSamples: func(x []int16) {
				for _, v := range x {
					d.mix(v)
				}
			}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Write(bursts()); err != nil {
			t.Fatal(err)
		}
		if err := tx.Flush(); err != nil {
			t.Fatal(err)
		}
		if d.n != c.n || d.h != c.h {
			t.Errorf("tier %d: %d samples, digest %#x, want %d and %#x", c.tier-rf.Tier0, d.n, d.h, c.n, c.h)
		}
	}
}
