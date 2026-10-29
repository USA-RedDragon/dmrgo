//go:build !tinygo

package fsktx

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

func TestTxMatrixLimitsAudio(t *testing.T) {
	t.Parallel()
	type audioCase struct {
		cfg AudioConfig
		ok  bool
	}
	audio := make([]audioCase, 0, 1024)
	for _, tier := range []Tier{TierDefault, Tier0, Tier1, Tier2} {
		rates := []int{0, 24000, 48000}
		bad := []int{-48000, 1, 44100, 96000}
		if tier != Tier0 {
			rates = []int{0, 8000, 11025, 44100, 65536, 4915200}
			bad = []int{-8000, 1, 7999, 8191, 4920000, math.MaxInt, math.MinInt}
		}
		for _, hz := range rates {
			audio = append(audio, audioCase{AudioConfig{Hz: hz, Tier: tier}, true})
			for _, l := range []int{0, 1, 15000, 16000} {
				u := txUnit{mcu: false, tier: tier, format: fmtAudio, rate: max(hz, 0), level: l}
				if u.rate == 0 {
					u.rate = 48000
				}
				if l == 0 {
					u.level = tableUnits
				}
				if tier == TierDefault {
					u.tier = Tier1
				}
				lo, hi := u.offsetRange()
				if lo > hi {
					audio = append(audio, audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l}, l == 0})
					continue
				}
				audio = append(audio,
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: lo}, true},
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: hi}, true},
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: lo - 1}, false},
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: hi + 1}, false},
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: math.MaxInt}, false},
					audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l, Offset: math.MinInt}, false})
			}
			for _, l := range []int{-1, 16001, math.MaxInt, math.MinInt} {
				audio = append(audio, audioCase{AudioConfig{Hz: hz, Tier: tier, Level: l}, false})
			}
		}
		for _, hz := range bad {
			audio = append(audio, audioCase{AudioConfig{Hz: hz, Tier: tier}, false})
		}
	}
	audio = append(audio, audioCase{AudioConfig{Tier: Tier2 + 1}, false}, audioCase{AudioConfig{Tier: -1}, false})
	const chunk = 16
	for i := 0; i < len(audio); i += chunk {
		cases := audio[i:min(i+chunk, len(audio))]
		t.Run(fmt.Sprint(i/chunk), func(t *testing.T) {
			t.Parallel()
			for _, c := range cases {
				a, err := NewAudio(c.cfg)
				if (err == nil) != c.ok || (a == nil) == c.ok {
					t.Errorf("NewAudio(%+v): %v, want ok %v", c.cfg, err, c.ok)
				}
			}
		})
	}
}

func TestTxMatrixLimitsIQ(t *testing.T) {
	t.Parallel()
	type iqCase struct {
		cfg IQConfig
		ok  bool
	}
	var iq []iqCase
	for _, tier := range []Tier{TierDefault, Tier0, Tier1, Tier2} {
		rates := []int{48000, 96000, 192000}
		bad := []int{0, -48000, 24000, 47999, 2400000, 20000000}
		if tier != Tier0 {
			rates = []int{0, 48000, 48001, 2400000, 20000000}
			bad = []int{-48000, 0x7fffffff, 47999, 20000001, math.MaxInt, math.MinInt}
		}
		for _, r := range rates {
			rate := float64(r)
			if r == 0 {
				rate = 2400000
			}
			edge := math.Nextafter(rate/4, 0)
			if tier == Tier0 {
				edge = rate/4 - 0.51
			}
			for _, off := range []float64{0, 1200, -2500, edge, -edge} {
				for _, ramp := range []float64{0, 1 / rate, 1e-3, 0.01} {
					iq = append(iq, iqCase{IQConfig{Rate: r, Tier: tier, OffsetHz: off, RampSeconds: ramp}, true})
				}
			}
			for _, off := range []float64{rate / 4, -rate / 4, math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64, -math.MaxFloat64} {
				iq = append(iq, iqCase{IQConfig{Rate: r, Tier: tier, OffsetHz: off}, false})
			}
			if tier == Tier0 {
				iq = append(iq, iqCase{IQConfig{Rate: r, Tier: tier, OffsetHz: rate/4 - 0.49}, false})
			}
			for _, ramp := range []float64{-1e-3, 0.4 / rate, math.Nextafter(0.01, 1), math.NaN(), math.Inf(1), math.Inf(-1), math.SmallestNonzeroFloat64} {
				iq = append(iq, iqCase{IQConfig{Rate: r, Tier: tier, RampSeconds: ramp}, false})
			}
		}
		for _, r := range bad {
			iq = append(iq, iqCase{IQConfig{Rate: r, Tier: tier}, false})
		}
	}
	iq = append(iq, iqCase{IQConfig{Tier: Tier2 + 1}, false}, iqCase{IQConfig{Tier: -1}, false})
	for _, c := range iq {
		q, err := NewIQ(c.cfg)
		if (err == nil) != c.ok || (q == nil) == c.ok {
			t.Errorf("NewIQ(%+v): %v, want ok %v", c.cfg, err, c.ok)
		}
	}
}

func TestTxMatrixLimitsMCU(t *testing.T) {
	t.Parallel()
	for _, tab := range []AudioTable{Table24k(), Table48k()} {
		for _, l := range []int{1, 100, 1080, 15000, 16000} {
			u := txUnit{mcu: true, format: fmtAudio, rate: tab.Hz(), level: l}
			lo, hi := u.offsetRange()
			for _, c := range []struct {
				level, offset int
				ok            bool
			}{
				{l, 0, true}, {l, lo, true}, {l, hi, true}, {l, lo - 1, false}, {l, hi + 1, false},
				{l, math.MaxInt, false}, {l, math.MinInt, false}, {l, math.MaxInt32, false}, {l, math.MinInt32, false},
				{0, 0, false}, {-1, 0, false}, {16001, 0, false}, {math.MaxInt, 0, false}, {math.MinInt, 0, false},
			} {
				checkMCUAudioInit(t, tab, c.level, c.offset, c.ok)
			}
		}
	}
	checkMCUAudioInit(t, AudioTable{}, tableUnits, 0, false)
	for _, r := range []int{48000, 96000, 192000} {
		for _, c := range []struct {
			off, ramp int
			ok        bool
		}{
			{0, 1, true}, {0, 1 << 15, true}, {r/4 - 1, 48, true}, {-(r/4 - 1), 48, true},
			{0, 0, false}, {0, -1, false}, {0, 1<<15 + 1, false}, {r / 4, 48, false}, {-r / 4, 48, false},
			{math.MaxInt, 48, false}, {math.MinInt, 48, false}, {math.MaxInt / 2, 48, false}, {math.MinInt / 4, 48, false},
			{1 << 29, 48, false}, {-1 << 29, 48, false}, {1 << 30, 48, false}, {0, math.MaxInt, false}, {0, math.MinInt, false},
		} {
			checkMCUIQInit(t, r, c.off, c.ramp, c.ok)
		}
	}
	for _, r := range []int{0, 24000, 47999, 48001, 384000, -48000, math.MaxInt, math.MinInt} {
		checkMCUIQInit(t, r, 0, 48, false)
	}
}

func checkMCUAudioInit(t *testing.T, tab AudioTable, level, offset int, ok bool) {
	t.Helper()
	syms := goldenSymbols(200)
	var a, b MCUAudio
	if err := a.Init(Table24k(), 1000, 100); err != nil {
		t.Fatal(err)
	}
	b = a
	var buf [maxSamples]int16
	var x, y []int16
	for i, s := range syms {
		if i == 100 {
			err := b.Init(tab, level, offset)
			if (err == nil) != ok {
				t.Errorf("MCUAudio.Init(%d Hz, %d, %d): %v, want ok %v", tab.Hz(), level, offset, err, ok)
			}
			if ok {
				return
			}
		}
		x = append(x, buf[:a.Symbol(buf[:], s)]...)
		y = append(y, buf[:b.Symbol(buf[:], s)]...)
	}
	if !slices.Equal(x, y) {
		t.Errorf("MCUAudio.Init(%d Hz, %d, %d) failed and changed the state", tab.Hz(), level, offset)
	}
}

func checkMCUIQInit(t *testing.T, rate, off, ramp int, ok bool) {
	t.Helper()
	syms := goldenSymbols(200)
	var a, b MCUIQ
	if err := a.Init(96000, 700, 96); err != nil {
		t.Fatal(err)
	}
	a.Key(true)
	b = a
	var buf [2 * maxSamples << 3]int16
	var x, y []int16
	for i, s := range syms {
		if i == 100 {
			err := b.Init(rate, off, ramp)
			if (err == nil) != ok {
				t.Errorf("MCUIQ.Init(%d, %d, %d): %v, want ok %v", rate, off, ramp, err, ok)
			}
			if ok {
				return
			}
		}
		x = append(x, buf[:a.Symbol(buf[:], s)]...)
		y = append(y, buf[:b.Symbol(buf[:], s)]...)
	}
	if !slices.Equal(x, y) {
		t.Errorf("MCUIQ.Init(%d, %d, %d) failed and changed the state", rate, off, ramp)
	}
}

func TestTxMatrixZero(t *testing.T) {
	t.Parallel()
	var a Audio
	var q IQ
	var m MCUAudio
	var mq MCUIQ
	var buf [160]int16
	for _, s := range []int8{3, -3, 0, 1, 127, -128} {
		if out := a.AppendSymbol(nil, s); len(out) != 0 {
			t.Errorf("zero Audio wrote %d samples", len(out))
		}
		if out := q.AppendS16(nil, s); len(out) != 0 {
			t.Errorf("zero IQ wrote %d values", len(out))
		}
		if out := q.AppendS8(nil, s); len(out) != 0 {
			t.Errorf("zero IQ wrote %d bytes", len(out))
		}
		q.Key(true)
		if n := m.Symbol(buf[:], s); n != 0 {
			t.Errorf("zero MCUAudio wrote %d samples", n)
		}
		mq.Key(s > 0)
		if n := mq.Symbol(buf[:], s); n != 0 {
			t.Errorf("zero MCUIQ wrote %d values", n)
		}
	}
	syms := goldenSymbols(300)
	for _, c := range []struct {
		name string
		init func() (func([]int16, int8) int, int, error)
	}{
		{"MCUAudio 24k", func() (func([]int16, int8) int, int, error) {
			var m MCUAudio
			return m.Symbol, 5, m.Init(Table24k(), 15000, 0)
		}},
		{"MCUAudio 48k", func() (func([]int16, int8) int, int, error) {
			var m MCUAudio
			return m.Symbol, 10, m.Init(Table48k(), 15000, 0)
		}},
		{"MCUIQ 192k", func() (func([]int16, int8) int, int, error) {
			var m MCUIQ
			err := m.Init(192000, 1000, 96)
			m.Key(true)
			return m.Symbol, 80, err
		}},
	} {
		sym, n, err := c.init()
		if err != nil {
			t.Fatal(err)
		}
		ref, _, _ := c.init()
		x := make([]int16, 0, len(syms)*n)
		y := make([]int16, 0, len(syms)*n)
		for i, s := range syms {
			if i%7 == 3 {
				if k := sym(make([]int16, n-1, 4*n), s); k != 0 {
					t.Errorf("%s: wrote %d to a dst of %d", c.name, k, n-1)
				}
			}
			x = append(x, buf[:sym(buf[:], s)]...)
			y = append(y, buf[:ref(buf[:], s)]...)
		}
		if !slices.Equal(x, y) {
			t.Errorf("%s: a short dst changed the output", c.name)
		}
	}
}

func TestTxMatrixErrors(t *testing.T) {
	t.Parallel()
	var m MCUAudio
	var q MCUIQ
	_, audioHz := NewAudio(AudioConfig{Hz: 7999})
	_, audioLevel := NewAudio(AudioConfig{Level: 16001})
	_, audioOffset := NewAudio(AudioConfig{Offset: 5000})
	_, audioTier := NewAudio(AudioConfig{Tier: Tier2 + 1})
	_, iqHz := NewIQ(IQConfig{Rate: 47999})
	_, iqCarrier := NewIQ(IQConfig{Rate: 48000, OffsetHz: 12000})
	_, iqRound := NewIQ(IQConfig{Rate: 48000, OffsetHz: 11999.6, Tier: Tier0})
	_, iqRamp := NewIQ(IQConfig{RampSeconds: 0.02})
	_, iqRate := NewIQ(IQConfig{Rate: 2400000, Tier: Tier0})
	for _, c := range []struct {
		name      string
		got, want error
	}{
		{"NewAudio Hz", audioHz, errHz},
		{"NewAudio Level", audioLevel, errLevel},
		{"NewAudio Offset", audioOffset, errOffset},
		{"NewAudio Tier", audioTier, errTier},
		{"NewIQ Rate", iqHz, errHz},
		{"NewIQ OffsetHz", iqCarrier, errCarrier},
		{"NewIQ OffsetHz of Tier0", iqRound, errCarrier},
		{"NewIQ RampSeconds", iqRamp, errRamp},
		{"NewIQ Rate of Tier0", iqRate, errRate},
		{"MCUAudio.Init table", m.Init(AudioTable{}, tableUnits, 0), errTable},
		{"MCUAudio.Init level", m.Init(Table24k(), 0, 0), errLevel},
		{"MCUAudio.Init offset", m.Init(Table24k(), tableUnits, 5000), errOffset},
		{"MCUIQ.Init rate", q.Init(24000, 0, 48), errRate},
		{"MCUIQ.Init offset", q.Init(48000, 12000, 48), errCarrier},
		{"MCUIQ.Init ramp", q.Init(48000, 0, 0), errRamp},
	} {
		if !errors.Is(c.got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
	}
}
