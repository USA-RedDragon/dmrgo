//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

func keyUnits(full bool) []txUnit {
	var out []txUnit
	for _, k := range iqKinds() {
		for _, r := range iqRates(k.mcu, k.tier, full) {
			if r > 2400000 && !full {
				continue
			}
			maxRamp := int(math.Round(0.01 * float64(r)))
			if k.mcu {
				maxRamp = 1 << 15
			}
			for _, ramp := range []int{1, max(r/SymbolRate/2, 1), int(math.Round(1e-3 * float64(r))), maxRamp} {
				u := k
				u.rate, u.offHz, u.ramp = r, -1700, ramp
				out = append(out, u)
			}
		}
	}
	return out
}

func (u txUnit) envelope(counts []int, keys []keying, n int) []float64 {
	e := make([]float64, n)
	env, on, k := 0, false, 0
	for i := range n {
		for k < len(keys) && counts[keys[k].at] == i {
			on = keys[k].on
			k++
		}
		switch {
		case on && env < u.ramp:
			env++
		case !on && env > 0:
			env--
		}
		e[i] = 0.5 - 0.5*math.Cos(math.Pi*float64(env)/float64(u.ramp))
	}
	return e
}

func TestTxMatrixKeying(t *testing.T) {
	t.Parallel()
	full := txFull()
	for _, u := range raceUnits(keyUnits(full)) {
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewPCG(uint64(u.rate), uint64(u.ramp)))
			sps := (u.rate + SymbolRate - 1) / SymbolRate
			rampSyms := (u.ramp+sps-1)/sps + 2
			type seq struct {
				name string
				keys []keying
				n    int
			}
			var seqs []seq
			phases := 12
			if _, m := ratio(u.rate); m > 1 && full {
				phases = min(m, 400)
			}
			for p := range phases {
				on := 3 + p
				seqs = append(seqs, seq{"burst", []keying{{on, true}, {on + rampSyms + 132, false}}, on + 2*rampSyms + 140})
			}
			seqs = append(seqs,
				seq{"one symbol", []keying{{5, true}, {6, false}}, 8 + rampSyms},
				seq{"back to back", []keying{{3, true}, {3 + rampSyms + 20, false}, {4 + rampSyms + 20, true}, {6 + 2*rampSyms + 40, false}}, 10 + 3*rampSyms + 40},
				seq{"mid ramp", []keying{{3, true}, {3 + rampSyms/2, false}, {3 + rampSyms, true}, {4 + rampSyms, false}}, 8 + 3*rampSyms},
				seq{"repeat", []keying{{2, true}, {2, false}, {2, true}, {9, true}, {9 + rampSyms, false}, {9 + rampSyms, false}}, 12 + 2*rampSyms},
			)
			for _, s := range seqs {
				syms := make([]int8, s.n)
				for i := range syms {
					syms[i] = int8(2*rng.IntN(4) - 3)
				}
				m := u.mustMod(t)
				var raw []int16
				counts := make([]int, len(syms)+1)
				k := 0
				for i, v := range syms {
					for k < len(s.keys) && s.keys[k].at == i {
						m.key(s.keys[k].on)
						k++
					}
					raw = m.emit(raw, v)
					counts[i+1] = len(raw) / 2
				}
				checkKeyed(t, u, s.name, iqComplex(raw), u.envelope(counts, s.keys, len(raw)/2))
			}
		})
	}
}

func checkKeyed(t *testing.T, u txUnit, name string, x []complex128, e []float64) {
	t.Helper()
	a := u.amplitude()
	q := 1.5 / a
	if u.mcu || u.tier == Tier0 {
		q = 3/a + math.Pi/2/2048 + 1.0/32768
	}
	for i, v := range x {
		if math.Abs(cmplx.Abs(v)/a-e[i]) > q+1e-9 {
			t.Errorf("%s: amplitude %.5f at sample %d, ramp %.5f", name, cmplx.Abs(v)/a, i, e[i])
			return
		}
		if e[i] == 0 && v != 0 {
			t.Errorf("%s: output %v keyed down at sample %d", name, v, i)
			return
		}
	}
	peak := u.peakHz()
	for i := 1; i < len(x); i++ {
		r := min(cmplx.Abs(x[i]), cmplx.Abs(x[i-1])) / a
		if r < 0.25 {
			continue
		}
		step := 2*math.Pi*(math.Abs(float64(u.offHz))+peak*1.001+1)/float64(u.rate) + 2*q/r + u.phaseStep()
		if d := math.Abs(cmplx.Phase(x[i] * cmplx.Conj(x[i-1]))); d > step {
			t.Errorf("%s: phase step %.4f rad at sample %d, most %.4f", name, d, i, step)
			return
		}
	}
}

func TestTxMatrixChunking(t *testing.T) {
	t.Parallel()
	full := txFull()
	units := nominal(full)
	for _, u := range raceUnits(offsetUnits(full)) {
		if u.ramp == 1 && u.offHz == -2500 {
			u.ramp = max(u.rate/1000, 1)
			units = append(units, u)
		}
	}
	for _, u := range units {
		if u.rate > 2400000 && !full {
			continue
		}
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			syms := goldenSymbols(600)
			for i := range syms {
				if i%97 == 13 {
					syms[i] = int8(i) | 4
				}
			}
			keys := []keying{{at: 10, on: true}, {at: 300, on: false}, {at: 340, on: true}}
			want := u.run(t, syms, keys)
			clean := make([]int8, len(syms))
			for i, s := range syms {
				clean[i] = cleanSymbol(s)
			}
			if got := u.run(t, clean, keys); !slices.Equal(got, want) {
				t.Errorf("an invalid symbol is not 0")
			}
			rng := rand.New(rand.NewPCG(3, 4))
			m := u.mustMod(t)
			var got []int16
			k := 0
			for i, s := range syms {
				for k < len(keys) && keys[k].at == i {
					m.key(keys[k].on)
					k++
				}
				var dst []int16
				switch i % 4 {
				case 1:
					dst = make([]int16, 0, rng.IntN(64))
				case 2:
					dst = make([]int16, rng.IntN(9), 4096)
				case 3:
					dst = make([]int16, rng.IntN(5))
				}
				pre := slices.Clone(dst)
				out := m.emit(dst, s)
				if !slices.Equal(out[:len(pre)], pre) {
					t.Fatalf("symbol %d: the prefix of dst changed", i)
				}
				got = append(got, out[len(pre):]...)
			}
			if !slices.Equal(got, want) {
				t.Errorf("the output depends on dst")
			}
			a, b := u.mustMod(t), u.mustMod(t)
			var xa, xb []int16
			other := goldenSymbols(700)[100:]
			k = 0
			for i := range syms {
				for k < len(keys) && keys[k].at == i {
					a.key(keys[k].on)
					k++
				}
				b.key(i%50 < 25)
				xa = a.emit(xa, syms[i])
				xb = b.emit(xb, other[i])
			}
			if !slices.Equal(xa, want) {
				t.Errorf("a second modulator changed the output of the first")
			}
			switch mm := m.(type) {
			case *mcuAudioMod:
				t2 := Table24k()
				if u.rate == 48000 {
					t2 = Table48k()
				}
				if err := mm.m.Init(t2, u.level, u.offset); err != nil {
					t.Fatal(err)
				}
			case *mcuIQMod:
				if err := mm.m.Init(u.rate, u.offHz, u.ramp); err != nil {
					t.Fatal(err)
				}
			default:
				return
			}
			if again := runMod(m, syms, keys); !slices.Equal(again, want) {
				t.Errorf("Init after use does not start over")
			}
		})
	}
}

func TestTxMatrixLongRun(t *testing.T) {
	t.Parallel()
	full := txFull()
	for _, u := range raceUnits(nominal(full)) {
		minutes := 2.0
		switch {
		case full && u.rate > 192000:
			minutes = 60
		case full:
			minutes = 180
		case u.rate > 192000:
			minutes = 1.0 / 3
		}
		if testmode.Race {
			minutes = 1.0 / 12
		}
		if u.rate > 2400000 && !full {
			continue
		}
		u.offHz = -1300
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			checkLongRun(t, u, int(minutes*60*SymbolRate))
		})
	}
}

func checkLongRun(t *testing.T, u txUnit, n int) {
	t.Helper()
	const period = SymbolRate
	rng := rand.New(rand.NewPCG(9, 9))
	syms := make([]int8, period)
	for i := range syms {
		syms[i] = int8(2*rng.IntN(4) - 3)
	}
	m := u.mustMod(t)
	m.key(true)
	var first, last, buf []int16
	reps := max(n/period, 3)
	for r := range reps {
		buf = buf[:0]
		for _, s := range syms {
			buf = m.emit(buf, s)
		}
		switch r {
		case 1:
			first = slices.Clone(buf)
		case reps - 1:
			last = buf
		}
	}
	if len(first) != len(last) {
		t.Fatalf("%d samples in the first period and %d in the last", len(first), len(last))
	}
	if u.audio() {
		if !slices.Equal(first, last) {
			t.Errorf("the last period of %d differs from the first", reps)
		}
		return
	}
	x, y := iqComplex(first), iqComplex(last)
	var rot complex128
	for i := range x {
		rot += y[i] * cmplx.Conj(x[i])
	}
	rot /= complex(cmplx.Abs(rot), 0)
	a := u.amplitude()
	tol := 3.0 + a*u.phaseStep()
	for i := range x {
		if d := cmplx.Abs(y[i] - x[i]*rot); d > tol {
			t.Errorf("sample %d of the last period of %d is %v off the first, rotated", i, reps, d)
			break
		}
	}
}
