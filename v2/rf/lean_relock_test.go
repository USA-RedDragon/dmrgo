//go:build !tinygo

package rf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type relockPart struct {
	bursts []layer2.RawBurst
	t0, hz float64
}

func relockSynth(parts []relockPart, fade [][2]float64, end float64) []complex64 {
	n := int(end * basebandRate)
	x := make([]complex64, n)
	for _, p := range parts {
		f, e := rftx.Modulate(p.bursts, rftx.Schedule{Slot: 0})
		k0 := int(math.Round(p.t0 * basebandRate))
		var ph float64
		for i := range f {
			ph += 2 * math.Pi * (f[i] + p.hz) / basebandRate
			if k := k0 + i; k >= 0 && k < n {
				x[k] += complex64(complex(e[i]*math.Cos(ph), e[i]*math.Sin(ph)))
			}
		}
	}
	for k := range x {
		t := float64(k) / basebandRate
		for _, f := range fade {
			if t >= f[0] && t < f[1] {
				x[k] = 0
			}
		}
	}
	return x
}

func relockStarts(t0 float64, n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = t0 + float64(2*i)*slotSeconds + (slotSeconds-burstSeconds)/2
	}
	return w
}

func relockCheck(t *testing.T, label string, x []complex64, want []float64) {
	t.Helper()
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, audio := range []bool{false, true} {
			var got []Burst
			delay := 0.0
			if audio {
				r := mustAudio(AudioRxConfig{Rate: 48000, Tier: tier})
				got = r.AppendFlush(r.AppendBursts(nil, discAudio(x, 48000)))
				delay = secondsAudioLag / basebandRate
			} else {
				r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
				got = r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16)))
			}
			found := 0
			for _, w := range want {
				for _, b := range got {
					if math.Abs(b.Seconds+delay-w) < 20e-6 {
						found++
						break
					}
				}
			}
			if found != len(want) {
				t.Errorf("%s t%d audio %v: %d of %d bursts", label, tier-Tier0, audio, found, len(want))
			}
		}
	}
}

func TestLeanPayloadSync(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 310123, Superframes: 1}
	v, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	var p [8]byte
	binary.BigEndian.PutUint64(p[:], uint64(enums.SyncMSSourcedData)<<16)
	for lost := 1; lost <= 2; lost++ {
		vb := slices.Clone(v.Bursts)
		copy(vb[2+lost][0:6], p[:6])
		var fade [][2]float64
		var want []float64
		for i, w := range relockStarts(lead, len(vb)) {
			if i >= 2 && i < 2+lost {
				s := lead + float64(2*i)*slotSeconds
				fade = append(fade, [2]float64{s, s + slotSeconds})
				continue
			}
			want = append(want, w)
		}
		x := relockSynth([]relockPart{{vb, lead, 0}}, fade, lead+float64(2*len(vb))*slotSeconds+0.1)
		rng := rand.New(rand.NewPCG(uint64(lost), 9))
		for i := range x {
			x[i] += complex(float32(rng.NormFloat64()*0.01), float32(rng.NormFloat64()*0.01))
		}
		relockCheck(t, fmt.Sprintf("%d lost", lost), x, want)
	}
}

func TestLeanTalkerAfterDrop(t *testing.T) {
	t.Parallel()
	c1 := rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 310123, Superframes: 1}
	c2 := rftx.SeqConfig{ColorCode: 1, Source: 1234568, Dest: 9990, BS: 310123, Message: "TALKER TWO DATA"}
	v, err := rftx.Sequence("voice", c1)
	if err != nil {
		t.Fatal(err)
	}
	d, err := rftx.Sequence("data12", c2)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	vb := v.Bursts[:len(v.Bursts)-1]
	for _, gap := range []float64{0.0555, 0.0592, 0.1147, 0.1295, 0.21} {
		t2 := lead + float64(2*len(vb))*slotSeconds + gap + 7.3/symbolRate
		x := relockSynth([]relockPart{{vb, lead, 0}, {d.Bursts, t2, 150}}, nil, t2+float64(2*len(d.Bursts))*slotSeconds+0.1)
		want := append(relockStarts(lead, len(vb)), relockStarts(t2, len(d.Bursts))...)
		relockCheck(t, fmt.Sprintf("gap %.4f s", gap), x, want)
	}
}

func TestLeanLoneRCProof(t *testing.T) {
	t.Parallel()
	b, err := rftx.Sequence("rc", rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 310123})
	if err != nil {
		t.Fatal(err)
	}
	rc := b.Bursts[len(b.Bursts)-1]
	flip := func(r layer2.RawBurst, from, to int, bad func(layer2.RawBurst) bool) layer2.RawBurst {
		for k := from; k < to && !bad(r); k++ {
			r[k/8] ^= 0x80 >> (k % 8)
		}
		if !bad(r) {
			t.Fatal("corruption left the field valid")
		}
		return r
	}
	badEMB := flip(rc, 84, 92, func(r layer2.RawBurst) bool { return r.StandaloneRCEMB().FEC.Uncorrectable })
	oneEMB := flip(rc, 84, 85, func(r layer2.RawBurst) bool { return r.StandaloneRCEMB().FEC.ErrorsCorrected == 1 })
	const lead = 0.05
	for _, c := range []struct {
		name  string
		burst layer2.RawBurst
		want  int
	}{{"valid", rc, 1}, {"bad EMB", badEMB, 0}, {"EMB with one bit error", oneEMB, 0}} {
		x := relockSynth([]relockPart{{[]layer2.RawBurst{c.burst}, lead, 0}}, nil, lead+slotSeconds+0.1)
		for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
			n := 0
			for _, g := range r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16))) {
				if g.Sync == enums.SyncMSSourcedRC {
					n++
				}
			}
			if n != c.want {
				t.Errorf("%s t%d: %d RC bursts, want %d", c.name, tier-Tier0, n, c.want)
			}
		}
	}
}
