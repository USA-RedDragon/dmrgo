//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

type fuzzOp struct {
	sym  int8
	key  bool
	room int
}

func fuzzOps(data []byte, most int) []fuzzOp {
	ops := make([]fuzzOp, 0, len(data)/2)
	for i := 0; i+1 < len(data) && len(ops) < most; i += 2 {
		ops = append(ops, fuzzOp{sym: int8(data[i]), key: data[i+1]&1 == 1, room: int(data[i+1] >> 1)})
	}
	return ops
}

func checkPhase(t *testing.T, x []complex128, a, rate, hz, tol float64) {
	t.Helper()
	for i := 1; i < len(x); i++ {
		r := min(cmplx.Abs(x[i]), cmplx.Abs(x[i-1])) / a
		if r < 0.25 {
			continue
		}
		step := 2*math.Pi*hz/rate + 3/(a*r) + tol
		if d := math.Abs(cmplx.Phase(x[i] * cmplx.Conj(x[i-1]))); d > step {
			t.Fatalf("phase step %.4f rad at sample %d, most %.4f", d, i, step)
		}
	}
}

func FuzzAudioConfig(f *testing.F) {
	f.Add(48000, 15000, 0, int8(1))
	f.Add(24000, 1100, 2048, int8(0))
	f.Add(65536, 16000, 1490, int8(2))
	f.Add(4915200, 1, -32760, int8(3))
	f.Add(8191, 15000, 0, int8(2))
	f.Fuzz(func(t *testing.T, hz, level, offset int, tier int8) {
		cfg := AudioConfig{Hz: hz, Level: level, Offset: offset, Tier: Tier(tier)}
		a, err := NewAudio(cfg)
		if err != nil {
			if a != nil {
				t.Fatalf("%+v: a modulator and %v", cfg, err)
			}
			return
		}
		u := txUnit{mcu: false, tier: cfg.Tier, format: fmtAudio, rate: hz, level: level, offset: offset}
		if u.tier == TierDefault {
			u.tier = Tier1
		}
		if u.rate == 0 {
			u.rate = 48000
		}
		if u.level == 0 {
			u.level = tableUnits
		}
		checkAudioRange(t, u, patWorst, runMod(audioMod{a}, u.pattern(patWorst, 100), nil))
	})
}

func FuzzIQConfig(f *testing.F) {
	f.Add(2400000, 1200.0, 1e-3, int8(1))
	f.Add(48000, 11999.0, 0.0, int8(0))
	f.Add(192000, -47999.0, 1e-2, int8(0))
	f.Add(20000000, 4999999.0, 5e-8, int8(2))
	f.Add(96000, math.NaN(), 1e-3, int8(1))
	f.Fuzz(func(t *testing.T, rate int, off, ramp float64, tier int8) {
		cfg := IQConfig{Rate: rate, OffsetHz: off, RampSeconds: ramp, Tier: Tier(tier)}
		q, err := NewIQ(cfg)
		if err != nil {
			if q != nil {
				t.Fatalf("%+v: a modulator and %v", cfg, err)
			}
			return
		}
		if !(4*math.Abs(off) < float64(max(rate, 1))) && rate != 0 {
			t.Fatalf("%+v: took an offset outside a quarter of the rate", cfg)
		}
		if rate == 0 {
			rate = 2400000
		}
		u := txUnit{tier: cfg.Tier, format: fmtS16, rate: rate, offHz: int(math.Round(off))}
		if u.tier == TierDefault {
			u.tier = Tier1
		}
		syms := goldenSymbols(40)
		raw := runMod(&iqMod{q: q}, syms, []keying{{at: 2, on: true}, {at: 30, on: false}})
		if want := len(syms) * rate / SymbolRate; len(raw)/2 < want-1 || len(raw)/2 > want+1 {
			t.Fatalf("%+v: %d samples for %d symbols", cfg, len(raw)/2, len(syms))
		}
		a := u.amplitude()
		for i, v := range raw {
			if math.Abs(float64(v)) > a {
				t.Fatalf("%+v: value %d at %d", cfg, v, i)
			}
		}
		checkPhase(t, iqComplex(raw), a, float64(rate), math.Abs(off)+u.peakHz()*1.001+1, u.phaseStep())
	})
}

func FuzzMCUAudio(f *testing.F) {
	f.Add(false, 15000, 0, []byte{3, 20, 0xfd, 20, 1, 20, 0xff, 20, 0x7f, 20})
	f.Add(true, 1080, 2048, []byte{3, 0, 3, 2, 3, 30, 0xfd, 30})
	f.Add(true, 16000, -1500, []byte{0x80, 30, 3, 30})
	f.Fuzz(func(t *testing.T, at48 bool, level, offset int, data []byte) {
		tab := Table24k()
		if at48 {
			tab = Table48k()
		}
		var m, ref MCUAudio
		err := m.Init(tab, level, offset)
		if err2 := ref.Init(tab, level, offset); (err == nil) != (err2 == nil) {
			t.Fatal("Init is not deterministic")
		}
		if err != nil {
			return
		}
		u := txUnit{mcu: true, format: fmtAudio, rate: tab.Hz(), level: level, offset: offset}
		lo, hi := u.audioBounds()
		ops := fuzzOps(data, 4096)
		var buf, rbuf [64]int16
		got := make([]int16, 0, len(ops)*len(buf))
		want := make([]int16, 0, len(ops)*len(buf))
		allocs := testing.AllocsPerRun(4, func() {
			got, want = got[:0], want[:0]
			_ = m.Init(tab, level, offset)
			_ = ref.Init(tab, level, offset)
			for _, op := range ops {
				n := m.Symbol(buf[:min(op.room, len(buf))], op.sym)
				if min(op.room, len(buf)) < m.Samples() {
					if n != 0 {
						t.Fatalf("wrote %d to a dst of %d", n, op.room)
					}
					continue
				}
				got = append(got, buf[:n]...)
				want = append(want, rbuf[:ref.Symbol(rbuf[:], cleanSymbol(op.sym))]...)
			}
		})
		if allocs > 0 {
			t.Fatalf("%v allocations", allocs)
		}
		if !slices.Equal(got, want) {
			t.Fatal("the output differs from that of the valid symbols")
		}
		for i, v := range got {
			if float64(v) < lo || float64(v) > hi {
				t.Fatalf("sample %d is %d, bounds %v to %v", i, v, lo, hi)
			}
		}
	})
}

func FuzzMCUIQ(f *testing.F) {
	f.Add(uint8(0), 1200, 48, []byte{3, 81, 0xfd, 81, 1, 80, 0xff, 80, 0x7f, 81})
	f.Add(uint8(1), -23999, 1, []byte{3, 1, 3, 3, 3, 21, 0xfd, 41})
	f.Add(uint8(2), 47999, 32768, []byte{0x80, 81, 3, 81})
	f.Fuzz(func(t *testing.T, rsel uint8, off, ramp int, data []byte) {
		rate := 48000 << (rsel % 3)
		var m, ref MCUIQ
		if err := m.Init(rate, off, ramp); err != nil {
			return
		}
		ops := fuzzOps(data, 2048)
		var buf, rbuf [2 * maxSamples << 3]int16
		got := make([]int16, 0, len(ops)*len(buf))
		want := make([]int16, 0, len(ops)*len(buf))
		allocs := testing.AllocsPerRun(4, func() {
			got, want = got[:0], want[:0]
			_ = m.Init(rate, off, ramp)
			_ = ref.Init(rate, off, ramp)
			for _, op := range ops {
				m.Key(op.key)
				n := m.Symbol(buf[:min(op.room, len(buf))], op.sym)
				if min(op.room, len(buf)) < 2*m.Samples() {
					if n != 0 {
						t.Fatalf("wrote %d to a dst of %d", n, op.room)
					}
					continue
				}
				ref.Key(op.key)
				got = append(got, buf[:n]...)
				want = append(want, rbuf[:ref.Symbol(rbuf[:], cleanSymbol(op.sym))]...)
			}
		})
		if allocs > 0 {
			t.Fatalf("%v allocations", allocs)
		}
		if !slices.Equal(got, want) {
			t.Fatal("the output differs from that of the valid symbols")
		}
		for i, v := range got {
			if math.Abs(float64(v)) > iqAmplitude {
				t.Fatalf("value %d at %d", v, i)
			}
		}
		u := txUnit{mcu: true, format: fmtS16, rate: rate, offHz: off}
		checkPhase(t, iqComplex(got), iqAmplitude, float64(rate), math.Abs(float64(off))+u.peakHz()*1.001+1, u.phaseStep())
	})
}

func fuzzAudioUnits() []txUnit {
	return []txUnit{
		{tier: Tier0, format: fmtAudio, rate: 24000, level: tableUnits},
		{tier: Tier0, format: fmtAudio, rate: 48000, level: 1080, offset: 2048},
		{tier: Tier1, format: fmtAudio, rate: 44100, level: tableUnits},
		{tier: Tier1, format: fmtAudio, rate: 8000, level: 16000, offset: -1000},
		{tier: Tier1, format: fmtAudio, rate: 65536, level: 100},
		{tier: Tier2, format: fmtAudio, rate: 48000, level: tableUnits},
		{tier: Tier2, format: fmtAudio, rate: 11025, level: tableUnits, offset: 3000},
	}
}

func fuzzIQUnits() []txUnit {
	return []txUnit{
		{tier: Tier0, format: fmtS16, rate: 48000, offHz: 1200, ramp: 48},
		{tier: Tier0, format: fmtS16, rate: 192000, offHz: -47999, ramp: 1},
		{tier: Tier1, format: fmtS16, rate: 48001, offHz: 0, ramp: 48},
		{tier: Tier1, format: fmtS16, rate: 1000000, offHz: 249999, ramp: 1000},
		{tier: Tier1, format: fmtS16, rate: 2400000, offHz: -2500, ramp: 1},
		{tier: Tier2, format: fmtS16, rate: 96000, offHz: 23999, ramp: 960},
		{tier: Tier2, format: fmtS16, rate: 250000, offHz: 0, ramp: 250},
	}
}

func FuzzAudio(f *testing.F) {
	f.Add(uint8(0), []byte{3, 0, 0xfd, 7, 1, 255, 0xff, 3, 0x7f, 1})
	f.Add(uint8(4), []byte{3, 1, 3, 2, 3, 3, 0xfd, 4})
	f.Fuzz(func(t *testing.T, sel uint8, data []byte) {
		us := fuzzAudioUnits()
		u := us[int(sel)%len(us)]
		a := u.mustMod(t)
		ref := u.mustMod(t)
		ops := fuzzOps(data, 1024)
		var got, want []int16
		for _, op := range ops {
			pre := make([]int16, op.room%5, op.room%5+op.room)
			for i := range pre {
				pre[i] = int16(i*7919 + op.room)
			}
			out := a.emit(pre, op.sym)
			if !slices.Equal(out[:len(pre)], pre[:len(pre):len(pre)]) {
				t.Fatal("emit changed the prefix of dst")
			}
			got = append(got, out[len(pre):]...)
			want = ref.emit(want, cleanSymbol(op.sym))
		}
		if !slices.Equal(got, want) {
			t.Fatal("the output depends on dst or on invalid symbols")
		}
		lo, hi := u.audioBounds()
		for i, v := range got {
			if float64(v) < lo-0.5 || float64(v) > hi+0.5 {
				t.Fatalf("sample %d is %d, bounds %v to %v", i, v, lo, hi)
			}
		}
	})
}

func FuzzIQ(f *testing.F) {
	f.Add(uint8(0), []byte{3, 1, 0xfd, 7, 1, 255, 0xff, 2, 0x7f, 1})
	f.Add(uint8(3), []byte{3, 1, 3, 3, 3, 3, 0xfd, 4})
	f.Fuzz(func(t *testing.T, sel uint8, data []byte) {
		us := fuzzIQUnits()
		u := us[int(sel)%len(us)]
		q16, err := NewIQ(IQConfig{Rate: u.rate, OffsetHz: float64(u.offHz), RampSeconds: float64(u.ramp) / float64(u.rate), Tier: u.tier})
		if err != nil {
			t.Fatal(err)
		}
		q8, _ := NewIQ(IQConfig{Rate: u.rate, OffsetHz: float64(u.offHz), RampSeconds: float64(u.ramp) / float64(u.rate), Tier: u.tier})
		ref, _ := NewIQ(IQConfig{Rate: u.rate, OffsetHz: float64(u.offHz), RampSeconds: float64(u.ramp) / float64(u.rate), Tier: u.tier})
		ops := fuzzOps(data, 256)
		var got, want []int16
		var got8 []byte
		for _, op := range ops {
			q16.Key(op.key)
			q8.Key(op.key)
			ref.Key(op.key)
			pre := make([]int16, op.room%5, op.room%5+op.room)
			out := q16.AppendS16(pre, op.sym)
			got = append(got, out[len(pre):]...)
			pre8 := make([]byte, op.room%3, op.room%3+op.room)
			out8 := q8.AppendS8(pre8, op.sym)
			got8 = append(got8, out8[len(pre8):]...)
			want = ref.AppendS16(want, cleanSymbol(op.sym))
		}
		if !slices.Equal(got, want) {
			t.Fatal("the output depends on dst or on invalid symbols")
		}
		if len(got8) != len(got) {
			t.Fatalf("%d bytes of 8-bit I/Q for %d values", len(got8), len(got))
		}
		for i, v := range got {
			if int8(got8[i]) != int8((int32(v)+1<<7)>>8) {
				t.Fatalf("8-bit value %d at %d for 16-bit %d", int8(got8[i]), i, v)
			}
		}
		a := u.amplitude()
		for i, v := range got {
			if math.Abs(float64(v)) > a {
				t.Fatalf("value %d at %d", v, i)
			}
		}
		checkPhase(t, iqComplex(got), a, float64(u.rate), math.Abs(float64(u.offHz))+u.peakHz()*1.001+1, u.phaseStep())
	})
}

func FuzzRoundTrip(f *testing.F) {
	f.Add(uint8(7), uint32(3191868), uint32(9), true, uint8(0), int16(1200), uint16(100))
	f.Add(uint8(15), uint32(1), uint32(16777215), false, uint8(9), int16(-3000), uint16(0))
	f.Add(uint8(15), uint32(1), uint32(16777215), false, uint8(86), int16(-3000), uint16(81))
	f.Fuzz(func(t *testing.T, cc uint8, src, dst uint32, group bool, sel uint8, off int16, lead uint16) {
		src, dst = max(src&0xffffff, 1), max(dst&0xffffff, 1)
		cfg := rftx.SeqConfig{ColorCode: enums.ColorCode(cc % 16), Source: src, Dest: dst, BS: 314926, Group: group, Wakeups: 1, Superframes: 1, BSSourced: true, Slot: 1}
		base := newStreamOf(t, cfg, 40)
		ts := base.withLead(int(lead % 600))
		us := append(fuzzAudioUnits(), fuzzIQUnits()...)
		u := us[int(sel)%len(us)]
		if !u.audio() {
			u.offHz = int(off) % 3000
			if u.rate == 48000 || u.rate == 48001 {
				u.offHz %= 1500
			}
		}
		ref := newRefSig(ts)
		x := u.signal48(u.run(t, ts.syms, keyedOn()), radio{ideal: true})
		d := fsk(x, ts.syms).Delay
		sh := ref.shift(d, rxRate)
		p := rxPath{"rx1", rxTier1, false, 6}
		var notes []string
		defer func() { skipRF(t, notes) }()
		notes = append(notes, checkDetect(t, "rx1", func(k int, isRef bool) rawScore {
			if isRef {
				return ts.rawScore(p.run(noisy(ref.x48, sh+timing(k, rxRate), ref.n, 30, uint64(lead))), rxPad)
			}
			return ts.rawScore(p.run(noisy(x, timing(k, rxRate), ref.n, 30, uint64(lead))), rxPad)
		}))
		if u.audio() {
			return
		}
		lt := base.withLead(1200 + int(lead%600))
		refLead := newRefSig(lt)
		b := u.s8Bytes(u.run(t, lt.syms, keyedOn()))
		var rb []byte
		notes = append(notes, checkDetect(t, "native", func(k int, isRef bool) rawScore {
			if !isRef {
				return lt.rawScore(nativeBursts(moved(b, timing(k, u.rate)), u.rate), 0)
			}
			if rb == nil {
				rb = refLead.native(u, d+(refLead.delay-ref.delay))
			}
			return lt.rawScore(nativeBursts(moved(rb, timing(k, u.rate)), u.rate), 0)
		}))
	})
}
