package rf

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func codec2Call(t *testing.T, n int, late bool, cn float64) ([]complex64, [][9]byte, float64) {
	t.Helper()
	var lc pdu.FullLinkControl
	if err := lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 9, SourceAddress: 3191868}); err != nil {
		t.Fatal(err)
	}
	lc.FID = 0xC2
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 7, Source: layer2.SyncSourceMS})
	if err != nil {
		t.Fatal(err)
	}
	var bursts []layer2.RawBurst
	if !late {
		bursts = tx.Headers()
	}
	first := len(bursts)
	rng := rand.New(rand.NewPCG(7, 8))
	slots := make([][9]byte, n*layer2.VoiceBurstsPerSuperframe*layer2.VoiceSlotsPerBurst)
	for i := range slots {
		for k := range 6 {
			slots[i][k] = byte(rng.Uint32())
		}
		b, err := tx.AddSlots(slots[i])
		if err != nil {
			t.Fatal(err)
		}
		bursts = append(bursts, b...)
	}
	end, err := tx.Terminate()
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(append(bursts, end...), rftx.Schedule{Slot: 1})
	pad := int(0.3 * basebandRate)
	x := make([]complex64, len(fm)+2*pad)
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - pad; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + 900) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	ramp := (slotSeconds - burstSeconds) / 2
	return x, slots, 0.3 + float64(2*first+1)*slotSeconds + ramp + float64(cachSymbols)/symbolRate
}

func TestReceiverCodec2Call(t *testing.T) {
	t.Parallel()
	type run struct {
		tier Tier
		half bool
	}
	runs := []run{{Tier0, false}, {Tier1, false}, {Tier2, false}, {Tier3, false}, {Tier0, true}, {Tier1, true}}
	for _, late := range []bool{false, true} {
		x, want, start := codec2Call(t, 4, late, 30)
		for _, rn := range runs {
			name := strconv.Itoa(int(rn.tier - Tier0))
			if rn.half {
				name += "half"
			}
			if late {
				name += "late"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var got []Burst
				if rn.half {
					h := make([]complex64, len(x)/2)
					for i := range h {
						h[i] = (x[2*i] + x[2*i+1]) / 2
					}
					r := mustNew(RxConfig{Tier: rn.tier, Rate: discRate, Format: IQFormatCU8})
					for k := 0; k < len(h); k += 2048 {
						got = append(got, must(r.writeBB(h[k:min(k+2048, len(h))]))...)
					}
					got = append(got, r.flushAll()...)
				} else {
					got = receiveBaseband(x, rn.tier)
				}
				bad := codec2Bad(got, want, start, late)
				if bad != 0 {
					t.Errorf("%d of %d Codec2 frames differ", bad, len(want))
				}
			})
		}
	}
}

func codec2Bad(got []Burst, want [][9]byte, start float64, late bool) int {
	slots := make([][9]byte, len(want))
	seen := make([]bool, len(want)/layer2.VoiceSlotsPerBurst)
	for i := range got {
		b := &got[i]
		k := int(math.Round((b.Seconds - start) / (2 * slotSeconds)))
		if k < 0 || k >= len(seen) {
			continue
		}
		seen[k] = true
		data, _ := b.Decoded()
		if late && k == 0 {
			data = b.Data
		}
		s := data.VoiceSlots()
		copy(slots[layer2.VoiceSlotsPerBurst*k:], s[:])
	}
	bad := 0
	for i := range want {
		if !seen[i/layer2.VoiceSlotsPerBurst] || slots[i] != want[i] {
			bad++
		}
	}
	return bad
}
