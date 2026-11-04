//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func TestIQRegenAudioAligned(t *testing.T) {
	t.Parallel()
	x := make([]complex64, basebandRate)
	for i := range x {
		p := 2*math.Pi*1500*float64(i)/basebandRate - 900/700.0*math.Cos(2*math.Pi*700*float64(i)/basebandRate)
		x[i] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier1, Format: IQFormatCF32})
	q := r.host().iqd
	for _, n := range []int{4000, 2400 + 333, 2400 + 1001} {
		q.writeBaseband(x[:n])
		for _, v := range x[n : n+700] {
			q.sample(v)
		}
		x = x[n+700:]
		if q.d.locked {
			t.Fatal("locked on a tone")
		}
		live := append([]int16(nil), q.audio...)
		got := append([]int16(nil), live...)
		if n, ok := q.regenAudio(got, q.conv.taps, 0); len(live) == 0 || !ok || n != len(live) {
			t.Fatalf("no audio to regenerate (%d pending)", len(live))
		}
		worst := 0
		for i := range live {
			worst = max(worst, abs(int(got[i])-int(live[i])))
		}
		if worst > 2 {
			t.Errorf("%d pending samples: the regenerated audio differs from the converter by up to %d units", len(live), worst)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestIQNarrowReplayAligned(t *testing.T) {
	t.Parallel()
	x := make([]complex64, basebandRate/2)
	for i := range x {
		p := -900 / 700.0 * math.Cos(2*math.Pi*700*float64(i)/basebandRate)
		x[i] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier1, Format: IQFormatCF32})
	q := r.host().iqd
	f := q.refill
	if f.narrow == nil {
		t.Fatal("Tier 1 has no narrow filter")
	}
	ref := func(taps []float32) []int16 {
		d := iqDisc{decim: q.conv.decim, step: q.conv.step, taps: taps, hist: make([]complex64, 2*len(taps)), ncoRot: 1, ncoStep: 1}
		var out []int16
		for _, v := range x {
			if a, _, ok := d.push(v); ok {
				out = append(out, int16(max(min(roundf(a*iqUnitsPerHz), math.MaxInt16), math.MinInt16)))
			}
		}
		return out
	}
	narrow, wide := ref(f.narrow), ref(f.wideTaps)
	n := 4003
	for _, v := range x[:n] {
		q.sample(v)
	}
	rest := append([]int16(nil), q.audio...)
	k0 := int(q.outN) - len(rest)
	m, ok := q.regenAudio(rest, f.narrow, f.dd)
	if !ok {
		t.Fatal("no audio to regenerate")
	}
	got := append(make([]int16, k0), rest[:m]...)
	q.toNarrow()
	q.afcN, q.audio = 0, make([]int16, 0, 4096)
	for _, v := range x[n : n+2000] {
		q.sample(v)
	}
	got = append(got, q.audio...)
	sw := len(got)
	q.afcN, q.audio = 0, q.audio[:0]
	q.toWide()
	for _, v := range x[n+2000 : n+3000] {
		q.sample(v)
	}
	got = append(got, q.audio...)
	for i := k0; i < len(got); i++ {
		want := wide[i]
		if i < sw {
			want = narrow[i+int(f.dd)]
		}
		if d := abs(int(got[i]) - int(want)); d > 12 || i != sw && d > 2 {
			t.Fatalf("output %d (narrow up to %d): %d, want %d", i, sw, got[i], want)
		}
	}
}
