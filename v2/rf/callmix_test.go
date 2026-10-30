//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type callTalker struct {
	src          uint32
	slot, lead   int
	superframes  int
	header, term bool
	levelDB      float64
}

type callSignal struct {
	x     []complex64
	truth []layer2.RawBurst
	who   []int
}

func callBursts(t testing.TB, k callTalker) []layer2.RawBurst {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: k.src, Dest: 9, Group: true, Wakeups: 1, Superframes: k.superframes, Message: "DMRGO CALL"}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out []layer2.RawBurst
	for _, r := range b.Bursts {
		x, err := layer2.NewBurstFromBytes(r)
		if err == nil && x.Kind == layer2.KindData {
			dt := x.SlotType.DataType
			if dt != enums.DataTypeTerminatorWithLC && !k.header || dt == enums.DataTypeTerminatorWithLC && !k.term {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func callMix(t testing.TB, cn float64, seed uint64, tail float64, talkers ...callTalker) callSignal {
	t.Helper()
	pad := int(benchPad * basebandRate)
	var s callSignal
	fs, es := make([][]float64, len(talkers)), make([][]float64, len(talkers))
	n, next := 0, 0
	for i, k := range talkers {
		bs := callBursts(t, k)
		if k.lead < 0 {
			k.lead = next
		}
		next = k.lead + len(bs)
		f, e := rftx.Modulate(bs, rftx.Schedule{Slot: k.slot})
		lead := 2 * k.lead * int(slotSeconds*basebandRate)
		fs[i], es[i] = make([]float64, lead+len(f)), make([]float64, lead+len(f))
		copy(fs[i][lead:], f)
		copy(es[i][lead:], e)
		n = max(n, len(fs[i]))
		for m, b := range bs {
			j := 2*(k.lead+m) + k.slot
			for len(s.truth) <= j {
				s.truth, s.who = append(s.truth, layer2.RawBurst{}), append(s.who, -1)
			}
			s.truth[j], s.who[j] = b, i
		}
	}
	s.x = make([]complex64, n+2*pad+int(tail*basebandRate))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	ph := make([]float64, len(talkers))
	for i := range s.x {
		re, im := rng.NormFloat64()*sigma, rng.NormFloat64()*sigma
		if k := i - pad; k >= 0 {
			for m, tk := range talkers {
				if k >= len(fs[m]) {
					continue
				}
				ph[m] += 2 * math.Pi * (fs[m][k] + 1400) / basebandRate
				a := es[m][k] * math.Pow(10, tk.levelDB/20)
				re += a * math.Cos(ph[m])
				im += a * math.Sin(ph[m])
			}
		}
		s.x[i] = complex(float32(re), float32(im))
	}
	return s
}

func receiveCore3(t testing.TB, x []complex64) []Burst {
	t.Helper()
	r, err := newReceiver(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8}, ptr(coreIQ3Options()))
	if err != nil {
		t.Fatal(err)
	}
	var out []Burst
	for k := 0; k < len(x); k += 4096 {
		out = append(out, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
	}
	return append(out, r.flushAll()...)
}
