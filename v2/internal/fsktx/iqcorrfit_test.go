//go:build !tinygo

package fsktx

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIQCorrDump(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRGO_IQCORR_DUMP")
	if dir == "" {
		t.Skip("set DMRGO_IQCORR_DUMP")
	}
	acp, alt := optACP, optALT
	for _, e := range []struct {
		name string
		v    *float64
	}{{"DMRGO_IQCORR_ACP", &acp}, {"DMRGO_IQCORR_ALT", &alt}} {
		if v := os.Getenv(e.name); v != "" {
			var err error
			if *e.v, err = strconv.ParseFloat(v, 64); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := IQConfig{Rate: 96000, Tier: Tier2, Exact: true, Pulse: PulseConfig{Window: 3, Beta: 5}}
	q0, err := NewIQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Shape = 15
	q1, err := NewIQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	q1.opt = newOptimizerACP(cfg.Shape, acp, alt)
	const n = 200000
	rng := rand.New(rand.NewPCG(3, 4))
	sy := make([]byte, n)
	buf := make([]float64, len(q0.buf))
	y0, y1 := make([]float64, 0, n*len(buf)), make([]float64, 0, n*len(buf))
	for i := range sy {
		s := int8(rng.IntN(4)*2 - 3)
		sy[i] = byte(s + 3)
		k := q0.k.symbol(buf, s)
		y0 = append(y0, buf[:k]...)
		k = q1.k.symbol(buf, s)
		q1.opt.process(buf[:k])
		y1 = append(y1, buf[:k]...)
	}
	b := make([]byte, 0, 4*len(y0))
	for i := range len(y0) - optDelay {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(float32(y1[i+optDelay]-y0[i])))
	}
	b0 := make([]byte, 0, 4*len(y0))
	for _, v := range y0[:len(y0)-optDelay] {
		b0 = binary.LittleEndian.AppendUint32(b0, math.Float32bits(float32(v)))
	}
	if err := os.WriteFile(filepath.Join(dir, "iq_base.f32"), b0, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iq_syms.bin"), sy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iq_corr.f32"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}
