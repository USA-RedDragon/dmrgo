//go:build !tinygo

package fsktx

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"testing"
)

var updateTxPairs = flag.Bool("update-txpairs", false, "measure the transmitter pairs into testdata/txpairs.json and rewrite their block of docs/tx.md")

const (
	txPairsFile    = "testdata/txpairs.json"
	txNotesFile    = "../../../docs/tx-notes.md"
	txPairsStreams = 64
	txPairsBegin   = "<!-- txpairs:begin -->"
	txPairsEnd     = "<!-- txpairs:end -->"
)

type txPairCells struct {
	ACPdBc   float64 `json:"acp_dbc"`
	DevPct   float64 `json:"dev_pct"`
	Eye96    float64 `json:"eye_96k"`
	EyeAC    float64 `json:"eye_ac10"`
	EyeACRaw float64 `json:"eye_ac10_raw"`
}

type txPairs struct {
	Ours, RRC txPairCells
	Streams   int         `json:"streams"`
	OursMean  float64     `json:"ours_mean"`
	RRCMean   float64     `json:"rrc_mean"`
	DiffMean  float64     `json:"diff_mean"`
	DiffSE    float64     `json:"diff_se"`
	OursWins  int         `json:"ours_wins"`
	RRCWins   int         `json:"rrc_wins"`
	ALT       []txPairALT `json:"alt"`
}

type txPairALT struct {
	Name     string  `json:"name"`
	Hz       int     `json:"hz"`
	HoldDBc  float64 `json:"hold_dbc"`
	IdealDBc float64 `json:"ideal_dbc"`
}

func measureTxPairALT(t testing.TB) []txPairALT {
	syms := newTestStream(t, 6, 40).syms
	cases := []struct {
		name string
		hz   int
		rrc  bool
	}{{"Tier 1", 24000, false}, {"Root raised cosine", 24000, true}, {"Root raised cosine", 48000, true}, {"Root raised cosine", 96000, true}}
	out := make([]txPairALT, 0, len(cases))
	for _, c := range cases {
		cfg, rrc := pairConfigs(c.hz)
		if c.rrc {
			cfg = rrc
		}
		r := txPairALT{Name: c.name, Hz: c.hz}
		for _, x := range []struct {
			dst *float64
			r   radio
		}{{&r.HoldDBc, radio{}}, {&r.IdealDBc, radio{ideal: true}}} {
			fm := pairSignal(t, cfg, syms, x.r)
			*x.dst = newSpectrum(fm[len(fm)/8:len(fm)*7/8], dacRate).acp(25000)
		}
		out = append(out, r)
	}
	return out
}

func pairConfigs(hz int) (ours, rrc AudioConfig) {
	return AudioConfig{Hz: hz, Level: tableUnits, Tier: Tier1}, AudioConfig{Hz: hz, Level: tableUnits, Tier: Tier2, Pulse: PulseConfig{Gain: 1, Span: 16}}
}

func pairSignal(t testing.TB, cfg AudioConfig, syms []int8, r radio) []complex128 {
	t.Helper()
	a, err := NewAudio(cfg)
	if err != nil {
		t.Fatal(err)
	}
	u := txUnit{format: fmtAudio, rate: cfg.Hz, tier: cfg.Tier, level: cfg.Level}
	return u.audioFM(runMod(audioMod{a}, syms, nil), r)
}

func pairEye(t testing.TB, cfg AudioConfig, syms []int8, r radio) float64 {
	y := resample(pairSignal(t, cfg, syms, r), dacRate, rxRate)
	return eyeError(y, syms, fsk(y, syms))
}

func pairCells(t testing.TB, c48, c96 AudioConfig, syms []int8) txPairCells {
	var c txPairCells
	fm := pairSignal(t, c48, syms, radio{})
	c.ACPdBc = newSpectrum(fm[len(fm)/8:len(fm)*7/8], dacRate).acp(12500)
	y := resample(fm, dacRate, rxRate)
	c.DevPct = math.Abs(fsk(y, syms).DevError)
	c.Eye96 = pairEye(t, c96, syms, radio{})
	ya := resample(pairSignal(t, c48, syms, radio{acHz: 10}), dacRate, rxRate)
	f := fsk(ya, syms)
	c.EyeAC = eyeError(ya, syms, f)
	z := fskAt(demod(ya), f.Delay, 0, len(syms))
	for k := 40; k < len(syms)-40; k++ {
		c.EyeACRaw = max(c.EyeACRaw, math.Abs(z[k]-float64(syms[k])*DeviationHz)/(2*DeviationHz))
	}
	return c
}

func measureTxPairs(t *testing.T) txPairs {
	o48, r48 := pairConfigs(48000)
	o96, r96 := pairConfigs(96000)
	base := newTestStream(t, 2, 40).syms
	p := txPairs{Ours: pairCells(t, o48, o96, base), RRC: pairCells(t, r48, r96, base), Streams: txPairsStreams}
	d := make([][2]float64, txPairsStreams)
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	for i := range d {
		rng := rand.New(rand.NewPCG(uint64(i), 0x5ac))
		syms := make([]int8, len(base))
		for k := range syms {
			syms[k] = [4]int8{-3, -1, 1, 3}[rng.IntN(4)]
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			d[i] = [2]float64{pairEye(t, o48, syms, radio{acHz: 10}), pairEye(t, r48, syms, radio{acHz: 10})}
		}()
	}
	wg.Wait()
	var s, s2 float64
	n := float64(txPairsStreams)
	for _, v := range d {
		p.OursMean += v[0] / n
		p.RRCMean += v[1] / n
		x := v[0] - v[1]
		s += x
		s2 += x * x
		switch {
		case x < 0:
			p.OursWins++
		case x > 0:
			p.RRCWins++
		}
	}
	p.ALT = measureTxPairALT(t)
	p.DiffMean = s / n
	p.DiffSE = math.Sqrt((s2/n - p.DiffMean*p.DiffMean) / (n - 1))
	return p
}

func renderTxPairs(p txPairs) string {
	var b strings.Builder
	b.WriteString(txPairsBegin + "\n\n")
	b.WriteString("Two pairs of cells of the comparison of the transmit tiers with external transmitters (October 2026) took each half of the bar from a different transmitter. The external captures of that comparison are gone, so `TestTxPairs` compares Tier 1 audio with a stand-in for the GNU Radio transmitter: a plain root raised cosine of 16 symbols each side, through the same measurements, with 16-bit audio and a hold converter.\n\n")
	b.WriteString("| Cell | Tier 1 | Root raised cosine |\n| --- | ---: | ---: |\n")
	fmt.Fprintf(&b, "| Adjacent channel power at 48 kHz, dBc | %.2f | %.2f |\n", p.Ours.ACPdBc, p.RRC.ACPdBc)
	fmt.Fprintf(&b, "| Deviation error at 48 kHz, %% | %.4f | %.4f |\n", p.Ours.DevPct, p.RRC.DevPct)
	fmt.Fprintf(&b, "| Eye error at 96 kHz | %.5f | %.5f |\n", p.Ours.Eye96, p.RRC.Eye96)
	fmt.Fprintf(&b, "| Eye error at 48 kHz, 10 Hz AC-coupled radio | %.5f | %.5f |\n", p.Ours.EyeAC, p.RRC.EyeAC)
	fmt.Fprintf(&b, "| The same, without the fit of gain and offset | %.5f | %.5f |\n", p.Ours.EyeACRaw, p.RRC.EyeACRaw)
	fmt.Fprintf(&b, "| Mean eye error, 10 Hz AC-coupled radio, %d random streams | %.5f | %.5f |\n\n", p.Streams, p.OursMean, p.RRCMean)
	both := "Tier 1 is lower on both cells at once."
	if p.Ours.ACPdBc >= p.RRC.ACPdBc || p.Ours.DevPct >= p.RRC.DevPct {
		both = "Tier 1 is not lower on both cells at once."
	}
	b.WriteString("- Adjacent channel power against deviation error: " + both + "\n")
	eye96 := "Tier 1 has the lower 96 kHz eye."
	if p.Ours.Eye96 >= p.RRC.Eye96 {
		eye96 = "The root raised cosine has the lower 96 kHz eye."
	}
	verdict := "a tie within two standard errors, so the one-stream cell is an artifact of one symbol pattern"
	switch {
	case p.DiffMean > 2*p.DiffSE:
		verdict = "the root raised cosine is lower by more than two standard errors"
	case p.DiffMean < -2*p.DiffSE:
		verdict = "Tier 1 is lower by more than two standard errors"
	}
	fmt.Fprintf(&b, "- AC-coupled eye against the 96 kHz eye: %s On the test stream, without the fit of gain and offset, the AC-coupled eye error is the wander of the data through the 10 Hz high pass and differs by %.5f between the two; with the fit, the rest is the intersymbol error of the one symbol where the wander peaks. Over %d random streams of the same length, each sent by both, the paired difference of Tier 1 less the root raised cosine is %+.5f with a standard error of %.5f, and they win %d and %d streams: %s.\n\n", eye96, math.Abs(p.Ours.EyeACRaw-p.RRC.EyeACRaw), p.Streams, p.DiffMean, p.DiffSE, p.OursWins, p.RRCWins, verdict)
	if len(p.ALT) > 0 {
		b.WriteString("The alternate channel power of the comparison with the MMDVM firmware ties for every transmitter at 24 kHz. The same signals through the hold converter of that comparison and through an ideal converter, which interpolates the codes with no images:\n\n| Signal | Rate | Hold converter | Ideal converter |\n| --- | ---: | ---: | ---: |\n")
		for _, a := range p.ALT {
			fmt.Fprintf(&b, "| %s | %s kHz | %.2f dBc | %.2f dBc |\n", a.Name, khz(a.Hz), a.HoldDBc, a.IdealDBc)
		}
		b.WriteString("\n- Alternate channel power at 24 kHz: through the hold converter every waveform reads the same, and through an ideal converter or at a higher rate it is far lower. The tie is the image of the converter at 24 kHz, which no waveform at that rate changes.\n\n")
	}
	b.WriteString("`go test ./internal/fsktx -run TestTxPairs -update-txpairs` in v2 measures them into `v2/internal/fsktx/testdata/txpairs.json` and writes this block, and `DMRGO_LONG=1` measures them again.\n\n")
	b.WriteString(txPairsEnd)
	return b.String()
}

func TestTxPairs(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(txNotesFile)
	if errors.Is(err, fs.ErrNotExist) && !*updateTxPairs {
		t.Skip("docs/tx-notes.md is not in this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	var p txPairs
	if *updateTxPairs || longRun() {
		p = measureTxPairs(t)
	}
	if *updateTxPairs {
		js, err := json.MarshalIndent(p, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(txPairsFile, append(js, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		var want txPairs
		js, err := os.ReadFile(txPairsFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(js, &want); err != nil {
			t.Fatal(err)
		}
		if longRun() && (math.Abs(p.DiffMean-want.DiffMean) > 1e-6 || p.OursWins != want.OursWins || math.Abs(p.Ours.EyeAC-want.Ours.EyeAC) > 1e-6) {
			t.Errorf("measured %+v, %s has %+v; run go test ./internal/fsktx -run TestTxPairs -update-txpairs", p, txPairsFile, want)
		}
		p = want
	}
	i, j := bytes.Index(doc, []byte(txPairsBegin)), bytes.Index(doc, []byte(txPairsEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", txNotesFile, txPairsBegin, txPairsEnd)
	}
	want := renderTxPairs(p)
	if *updateTxPairs {
		out := append(append(append([]byte{}, doc[:i]...), want...), doc[j+len(txPairsEnd):]...)
		if err := os.WriteFile(txNotesFile, out, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := string(doc[i : j+len(txPairsEnd)]); got != want {
		t.Errorf("the transmitter pairs of %s differ from %s; run go test ./internal/fsktx -run TestTxPairs -update-txpairs in v2", txNotesFile, txPairsFile)
	}
}
