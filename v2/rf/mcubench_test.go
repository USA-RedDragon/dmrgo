//go:build !tinygo

package rf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

type mcuHash struct {
	h  uint32
	nb uint32
}

func (m *mcuHash) mix(b byte) { m.h = (m.h ^ uint32(b)) * 16777619 }
func (m *mcuHash) mix64(v uint64) {
	for range 8 {
		m.mix(byte(v))
		v >>= 8
	}
}
func (m *mcuHash) on(bs []Burst) {
	for i := range bs {
		b := &bs[i]
		m.nb++
		for _, c := range b.Data {
			m.mix(c)
		}
		for _, c := range b.Fixed {
			m.mix(c)
		}
		for _, c := range b.CACH {
			m.mix(c)
		}
		m.mix64(math.Float64bits(b.Seconds))
		m.mix64(uint64(b.Distance))
		m.mix64(uint64(b.Stream))
		m.mix64(uint64(b.Slot))
		m.mix(byte(b.Sync))
		m.mix(byte(b.Voice))
		var f byte
		if b.HasCACH {
			f |= 1
		}
		if b.BS {
			f |= 2
		}
		if b.HasFixed {
			f |= 4
		}
		m.mix(f)
	}
}

func mcuNoisy(fm, env []float64, cn float64, seed uint64, sig bool) []complex64 {
	pad := int(0.3 * basebandRate)
	x := make([]complex64, len(fm)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - pad; sig && k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + 1400) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	return x
}

func TestMCUBenchSignals(t *testing.T) {
	t.Parallel()
	out := os.Getenv("DMRGO_MCUBENCH_OUT")
	if out == "" {
		t.Skip("cmd/mcubench sets DMRGO_MCUBENCH_OUT to write the signals of the Cortex-M4F bench")
	}
	slots, _ := benchStream(t, 6)
	fm, env := rftx.ModulateBS(slots)
	mf, me, _ := msStream(t, 2, 0.37)
	conds := map[string]func() []complex64{
		"30":    func() []complex64 { return mcuNoisy(fm, env, 30, 30, true) },
		"10":    func() []complex64 { return mcuNoisy(fm, env, 10, 10, true) },
		"6":     func() []complex64 { return mcuNoisy(fm, env, 6, 6, true) },
		"2":     func() []complex64 { return mcuNoisy(fm, env, 2, 2, true) },
		"-1":    func() []complex64 { return mcuNoisy(fm, env, -1, 1, true) },
		"noise": func() []complex64 { return mcuNoisy(fm, env, 0, 5, false) },
		"ms10":  func() []complex64 { return mcuNoisy(mf, me, 10, 11, true) },
		"ms2":   func() []complex64 { return mcuNoisy(mf, me, 2, 12, true) },
	}
	ref := make([]string, 0, 2*len(conds))
	for _, pe := range []bool{false, true} {
		key, prefix := "DMRGO_MCUBENCH_AUDIO", "a"
		if pe {
			key, prefix = "DMRGO_MCUBENCH_AUDIOPE", "p"
		}
		for _, r := range strings.Fields(os.Getenv(key)) {
			rate, _ := strconv.Atoi(r)
			for c, f := range conds {
				a := radioAudio(f(), rate, pe)
				b := make([]byte, 2*len(a))
				for i, v := range a {
					binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
				}
				name := fmt.Sprintf("%s-%s-%d.bin", prefix, c, rate)
				if err := os.WriteFile(filepath.Join(out, name), b, 0o600); err != nil {
					t.Fatal(err)
				}
				var m mcuHash
				m.h = 2166136261
				var r AudioReceiver
				cfg := AudioRxConfig{Rate: rate, OnBursts: m.on}
				if pe {
					cfg.PreEmphasis = NewRxPreEmphasis()
				}
				if err := r.Init(cfg); err != nil {
					t.Fatal(err)
				}
				for k := 0; k < len(a); k += 256 {
					_, _ = r.Write(a[k:min(k+256, len(a))])
				}
				_ = r.Flush()
				ref = append(ref, fmt.Sprintf("%s %d %d %x %d", name, len(a), m.nb, m.h, rate))
			}
		}
	}
	for _, r := range strings.Fields(os.Getenv("DMRGO_MCUBENCH_IQ")) {
		rate, _ := strconv.Atoi(r)
		for c, f := range conds {
			y := lowIQ(f(), rate)
			b := make([]byte, 4*len(y))
			for i, v := range y {
				binary.LittleEndian.PutUint16(b[4*i:], uint16(q15(real(v))))
				binary.LittleEndian.PutUint16(b[4*i+2:], uint16(q15(imag(v))))
			}
			name := fmt.Sprintf("q-%s-%d.bin", c, rate)
			if err := os.WriteFile(filepath.Join(out, name), b, 0o600); err != nil {
				t.Fatal(err)
			}
			var m mcuHash
			m.h = 2166136261
			var r Receiver
			if err := r.Init(RxConfig{Rate: rate, OnBursts: m.on}); err != nil {
				t.Fatal(err)
			}
			for k := 0; k < len(b); k += 512 {
				_, _ = r.Write(b[k:min(k+512, len(b))])
			}
			_ = r.Flush()
			ref = append(ref, fmt.Sprintf("%s %d %d %x %d", name, len(y), m.nb, m.h, rate))
		}
	}
	if err := os.WriteFile(filepath.Join(out, "ref.txt"), []byte(strings.Join(ref, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
