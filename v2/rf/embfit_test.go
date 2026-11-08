package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestEMBFitKeptSamples checks that the EMB timing fits of an MS call at
// Tier 1 read the samples of their own burst once the hunt for the clock rate
// has ended. The core keeps its acquisition samples only while it searches or
// hunts, so a fit that read that buffer after the hunt took old samples, its
// correlation fell to about 0.05, and the timing line of the talker's slot
// took no more fits. At +400 ppm and 30 dB the span of the line covers 9
// slots of the talker, of which at most 2 hold a voice burst A, which gets no
// fit after the hunt; it fails when the line holds fewer than the other 7.
func TestEMBFitKeptSamples(t *testing.T) {
	t.Parallel()
	embFitKept(t)
}

// TestEMBFitLockZeroPast pins a payload whose lock SYNC fit put its MM zero
// past the newest sample. The zero, extrapolated there, read the lock 0.05
// symbol late, the line through the first three SYNC fits took a slope 130
// ppm off, and every later EMB fit fell outside the gate of that line.
func TestEMBFitLockZeroPast(t *testing.T) {
	t.Parallel()
	embFitKept(pinnedTB{t, runMix(14818072848625347973, nameHash("TestEMBFitKeptSamples"))})
}

func embFitKept(t testing.TB) {
	t.Helper()
	cfg := testConfig(t)
	cfg.Superframes = 3
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var f, e [2][]float64
	f[0], e[0] = rftx.Modulate(b.Bursts, rftx.Schedule{})
	f[0], e[0] = resampleFM(f[0], e[0], 400)
	x := twoTalkerIQ(t, f, e, 1400, 30, 2)
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier1, Format: IQFormatCU8})
	for k := 0; k < len(x); k += 2048 {
		must(r.writeBB(x[k:min(k+2048, len(x))]))
	}
	xe, ok := r.host().iqd.d.ext.(*softExt)
	if !ok {
		t.Fatal("no stage extension at Tier 1")
	}
	n := max(xe.rate[0].n, xe.rate[1].n)
	if want := float32(extAnchorSpan/(2*slotSymbols) + 1 - 2); n < want {
		t.Errorf("the timing line holds %.0f fits, want at least %.0f", n, want)
	}
}
