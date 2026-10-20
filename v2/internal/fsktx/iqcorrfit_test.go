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

// TestIQCorrDump writes the data that the MCUIQ correction tables
// (testdata/mcorriq-*.json) are fitted to, to the dir of DMRGO_IQCORR_DUMP:
// 200000 random symbols (PCG 3, 4) as s+3 bytes to iq_syms.bin, and the
// deviation of Tier2 I/Q at 96 kHz (Exact, Pulse Window 3 Beta 5) with
// Shape 15 less that with Shape 0, at the shaper rate and aligned by
// optDelay, as float32 to iq_corr.f32. DMRGO_IQCORR_ACP sets the optACP of
// the optimizer (default optACP). The fit is
// candtx/scratch/txmcorr/fitiq.py and pocs.py.
func TestIQCorrDump(t *testing.T) {
	dir := os.Getenv("DMRGO_IQCORR_DUMP")
	if dir == "" {
		t.Skip("set DMRGO_IQCORR_DUMP")
	}
	acp := optACP
	if v := os.Getenv("DMRGO_IQCORR_ACP"); v != "" {
		var err error
		if acp, err = strconv.ParseFloat(v, 64); err != nil {
			t.Fatal(err)
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
	q1.opt = newOptimizerACP(cfg.Shape, acp)
	const n = 200000
	rng := rand.New(rand.NewPCG(3, 4))
	sy := make([]byte, n)
	var y0, y1 []float64
	buf := make([]float64, len(q0.buf))
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
	if err := os.WriteFile(filepath.Join(dir, "iq_syms.bin"), sy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iq_corr.f32"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}
