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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	txCmpFile  = "testdata/txcmp.json"
	txCmpBegin = "<!-- txcmp:begin -->"
	txCmpEnd   = "<!-- txcmp:end -->"

	txCmpMethodBegin = "<!-- txcmp:methods:begin -->"
	txCmpMethodEnd   = "<!-- txcmp:methods:end -->"
	txCmpDir         = "../../../docs/tx"
	txCmpLevel       = 960
	txCmpHz          = 24000
)

var updateTxCmp = flag.Bool("update-txcmp", false, "measure the comparison with the MMDVM firmware modulator (DMRGO_MMDVMTX) into testdata/txcmp.json and rewrite its block and graphs in docs/tx")

type txCmpRow struct {
	Name          string    `json:"name"`
	Level         int       `json:"level"`
	ACP           float64   `json:"acp_dbc"`
	ALT           float64   `json:"alt_dbc"`
	Mask          float64   `json:"mask_margin_db"`
	FSKError      float64   `json:"fsk_error_pct"`
	DevError      float64   `json:"dev_error_pct"`
	ETSIDev       float64   `json:"etsi_dev_pct"`
	VoiceCN90     float64   `json:"voice_cn90"`
	DataCN90      float64   `json:"data_cn90"`
	DiscVoiceCN90 float64   `json:"disc_voice_cn90"`
	DiscDataCN90  float64   `json:"disc_data_cn90"`
	Paired        []float64 `json:"paired_db,omitempty"`
	PairedSE      []float64 `json:"paired_se_db,omitempty"`
	Spectrum      []float64 `json:"spectrum_db"`
	Eye           [][]float64
}

type txCmpData struct {
	MMDVM  string     `json:"mmdvm"`
	Seeds  int        `json:"seeds"`
	BinHz  float64    `json:"bin_hz"`
	Rows   []txCmpRow `json:"rows"`
	EyeSPS int        `json:"eye_sps"`
}

func mmdvmDibit(s int8) byte {
	switch s {
	case 3:
		return 1
	case 1:
		return 0
	case -1:
		return 2
	}
	return 3
}

func mmdvmTX(t testing.TB, bin string, syms []int8) []int16 {
	t.Helper()
	in := make([]byte, (len(syms)+3)/4)
	for i, s := range syms {
		in[i/4] |= mmdvmDibit(s) << (6 - 2*(i%4))
	}
	cmd := exec.CommandContext(t.Context(), bin)
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	a := make([]int16, len(out)/2)
	for i := range a {
		a[i] = int16(uint16(out[2*i]) | uint16(out[2*i+1])<<8)
	}
	return a[:min(len(a), len(syms)*txCmpHz/SymbolRate)]
}

func mmdvmLevel(t testing.TB, bin string) int {
	t.Helper()
	syms := make([]int8, 400)
	for i := range syms {
		syms[i] = 3
	}
	a := mmdvmTX(t, bin, syms)
	return int(a[len(a)-50]) - 2048
}

func txCmpEye(x48 []complex128, delay float64) [][]float64 {
	f := demod(x48)
	sps := rxRate / SymbolRate
	lp := butter2(2400, rxRate)
	g := make([]float64, len(f))
	for i, v := range f {
		g[i] = lp.step(v)
	}
	var out [][]float64
	start := int(math.Round(delay)) + 200*sps - sps/2
	for k := 0; k < 120; k++ {
		i := start + 2*k*sps
		if i+2*sps+1 > len(g) {
			break
		}
		tr := make([]float64, 2*sps+1)
		for j := range tr {
			tr[j] = math.Round(g[i+j])
		}
		out = append(out, tr)
	}
	return out
}

const txCmpSeeds = 64

type txCmpCurve struct {
	cn []float64
	sc [][]score
}

func (c txCmpCurve) points(skip int) []point {
	pts := make([]point, len(c.cn))
	for k, cn := range c.cn {
		p := point{CN: cn}
		for i, s := range c.sc[k] {
			if i == skip {
				continue
			}
			p.Voice += s.voice
			p.VoiceOK += s.voiceOK
			p.Data += s.data
			p.DataOK += s.dataOK
		}
		pts[k] = p
	}
	return pts
}

func (c txCmpCurve) cn90(skip int) [2]float64 {
	v, d := cn90(c.points(skip))
	return [2]float64{v, d}
}

func txCmpPaired(a, b [2]txCmpCurve) (diff, se []float64) {
	for r := range a {
		for m := range 2 {
			d := a[r].cn90(-1)[m] - b[r].cn90(-1)[m]
			jk := make([]float64, txCmpSeeds)
			var mean float64
			for i := range jk {
				jk[i] = a[r].cn90(i)[m] - b[r].cn90(i)[m]
				mean += jk[i] / txCmpSeeds
			}
			var ss float64
			for _, v := range jk {
				ss += (v - mean) * (v - mean)
			}
			diff = append(diff, math.Round(d*1000)/1000)
			se = append(se, math.Round(math.Sqrt(ss*(txCmpSeeds-1)/txCmpSeeds)*1000)/1000)
		}
	}
	return diff, se
}

func txCmpScores(x48 []complex128, shift int, ts *testStream, disc bool) txCmpCurve {
	n := int((2*rxPad + float64(len(ts.syms))/SymbolRate) * rxRate)
	var c txCmpCurve
	for cn := -1.0; cn <= 16; cn += 0.5 {
		sc := make([]score, txCmpSeeds)
		var wg sync.WaitGroup
		sem := make(chan struct{}, max(1, testParallel()))
		for i := range sc {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				y := noisy(x48, shift, n, cn, uint64(300+i))
				if disc {
					sc[i] = ts.score(receiveDisc(y), rxPad)
				} else {
					sc[i] = ts.score(receive(y, rxTier2), rxPad)
				}
			}()
		}
		wg.Wait()
		c.cn, c.sc = append(c.cn, cn), append(c.sc, sc)
		var miss int
		for _, s := range sc {
			miss += s.voice - s.voiceOK + s.data - s.dataOK
		}
		if miss == 0 {
			break
		}
	}
	return c
}

func txCmpMeasure(t *testing.T, name string, level int, x []complex128, rate float64, etsi []complex128, ts *testStream) (txCmpRow, [2]txCmpCurve) {
	r := txCmpRow{Name: name, Level: level}
	sp := newSpectrum(x[len(x)/8:len(x)*7/8], rate)
	r.ACP, r.ALT, r.Mask = sp.acp(12500), sp.acp(25000), sp.maskMargin()
	for f := -25000.0; f <= 25000; f += 250 {
		p := sp.band(f-125, f+125)
		r.Spectrum = append(r.Spectrum, math.Round(10*math.Log10(max(p, 1e-15))*10)/10)
	}
	x48 := resample(x, int(rate), rxRate)
	fr := fsk(x48, ts.syms)
	r.FSKError, r.DevError = fr.FSKError, fr.DevError
	r.ETSIDev = etsiDeviation(etsi)
	r.Eye = txCmpEye(x48, fr.Delay)
	shift := int(math.Round(fr.Delay))
	cs := [2]txCmpCurve{txCmpScores(x48, shift, ts, false), txCmpScores(x48, shift, ts, true)}
	iq, disc := cs[0].cn90(-1), cs[1].cn90(-1)
	r.VoiceCN90, r.DataCN90, r.DiscVoiceCN90, r.DiscDataCN90 = iq[0], iq[1], disc[0], disc[1]
	t.Logf("%s: %+v", name, r)
	return r, cs
}

func measureTxCmp(t *testing.T, bin string) txCmpData {
	ts := newTestStream(t, 6, 40)
	d := txCmpData{MMDVM: os.Getenv("DMRGO_MMDVMTX_VERSION"), Seeds: txCmpSeeds, BinHz: 250, EyeSPS: rxRate / SymbolRate}
	curves := make([][2]txCmpCurve, 0, 3)
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		u := txUnit{tier: tier, format: fmtAudio, rate: txCmpHz, level: txCmpLevel}
		x := u.audioFM(u.run(t, ts.syms, nil), radio{})
		etsi := resample(u.audioFM(u.run(t, etsiSymbols(), nil), radio{}), dacRate, rxRate)
		r, cs := txCmpMeasure(t, fmt.Sprintf("dmrgo audio tier %d", tier-Tier0), txCmpLevel, x, dacRate, etsi, ts)
		d.Rows, curves = append(d.Rows, r), append(curves, cs)
	}
	lv := mmdvmLevel(t, bin)
	u := txUnit{format: fmtAudio, rate: txCmpHz, level: lv, offset: 2048}
	x := u.audioFM(mmdvmTX(t, bin, ts.syms), radio{})
	etsi := resample(u.audioFM(mmdvmTX(t, bin, etsiSymbols()), radio{}), dacRate, rxRate)
	r, ref := txCmpMeasure(t, "MMDVM firmware", -lv, x, dacRate, etsi, ts)
	for k, cs := range curves {
		d.Rows[k].Paired, d.Rows[k].PairedSE = txCmpPaired(cs, ref)
	}
	d.Rows = append(d.Rows, r)
	return d
}

func TestTxCompare(t *testing.T) {
	t.Parallel()
	txDocMu.Lock()
	defer txDocMu.Unlock()
	doc, err := os.ReadFile(txDocFile)
	if errors.Is(err, fs.ErrNotExist) && !*updateTxCmp && !*renderTxDoc {
		t.Skip("docs/tx.md is not in this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	var d txCmpData
	if *updateTxCmp {
		bin := os.Getenv("DMRGO_MMDVMTX")
		if bin == "" {
			t.Fatal("DMRGO_MMDVMTX names the host build of the MMDVM DMO modulator")
		}
		d = measureTxCmp(t, bin)
		js, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(txCmpFile, append(js, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		js, err := os.ReadFile(txCmpFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(js, &d); err != nil {
			t.Fatal(err)
		}
	}
	write := *updateTxCmp || *renderTxDoc
	for _, blk := range []struct{ begin, end, want string }{
		{txCmpBegin, txCmpEnd, renderTxCmp(d)},
		{txCmpMethodBegin, txCmpMethodEnd, renderTxCmpMethods(d)},
	} {
		i, j := bytes.Index(doc, []byte(blk.begin)), bytes.Index(doc, []byte(blk.end))
		if i < 0 || j < i {
			t.Fatalf("%s has no %s ... %s block", txDocFile, blk.begin, blk.end)
		}
		if got := string(doc[i : j+len(blk.end)]); got != blk.want && !write {
			t.Errorf("the %s block of %s differs from %s; run go test ./internal/fsktx -run TestTxCompare -update-txcmp in v2", blk.begin, txDocFile, txCmpFile)
		}
		doc = append(append(append([]byte{}, doc[:i]...), blk.want...), doc[j+len(blk.end):]...)
	}
	graphs := map[string][]byte{"compare_spectrum.svg": txCmpSpectrumSVG(d), "compare_eye.svg": txCmpEyeSVG(d)}
	if write {
		if err := os.WriteFile(txDocFile, doc, 0o600); err != nil {
			t.Fatal(err)
		}
		for name, b := range graphs {
			if err := os.WriteFile(filepath.Join(txCmpDir, name), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	for name, b := range graphs {
		if old, err := os.ReadFile(filepath.Join(txCmpDir, name)); err != nil || !bytes.Equal(old, b) {
			t.Errorf("docs/tx/%s differs from %s; run go test ./internal/fsktx -run TestTxCompare -update-txcmp in v2", name, txCmpFile)
		}
	}
}

func f1s(v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return strconv.FormatFloat(math.Round(v*10)/10+0, 'f', 1, 64)
}

func f2s(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100+0, 'f', 2, 64) }

func writeTxCmpPaired(b *strings.Builder, d txCmpData) {
	ref := d.Rows[len(d.Rows)-1].Name
	b.WriteString("Each receiver column less that of the " + ref + " in dB, over the same noise seeds, with one standard error from leaving out one seed at a time. A negative difference favors dmrgo.\n\n")
	b.WriteString(txTableHead("Transmitter", "Voice, I/Q receiver (dB)", "Data, I/Q receiver (dB)", "Voice, FM receiver (dB)", "Data, FM receiver (dB)"))
	for _, r := range d.Rows {
		if len(r.Paired) != 4 || len(r.PairedSE) != 4 {
			continue
		}
		b.WriteString("| " + r.Name)
		for m, v := range r.Paired {
			b.WriteString(" | " + f2s(v) + " ± " + f2s(r.PairedSE[m]))
		}
		b.WriteString(" |\n")
	}
	b.WriteString("\n")
}

func renderTxCmp(d txCmpData) string {
	var b strings.Builder
	b.WriteString(txCmpBegin + "\n\n")
	b.WriteString("The audio tiers and the DMR DMO modulator of the MMDVM firmware (commit " + d.MMDVM + ") make the same symbol stream at " + strconv.Itoa(txCmpHz/1000) + " kHz into a 12-bit converter and the same radio. [Other transmitters](#other-transmitters) gives the method.\n\n")
	b.WriteString(txTableHead("Transmitter", "Adjacent channel (dBc)", "Alternate channel (dBc)", "Mask D margin (dB)", "FSK error (%)", "Deviation error (%)", "ETSI pattern deviation error (%)", "Voice, I/Q receiver (dB)", "Data, I/Q receiver (dB)", "Voice, FM receiver (dB)", "Data, FM receiver (dB)"))
	for _, r := range d.Rows {
		ebn0 := func(cn float64) string { return f1s(cn + 10*math.Log10(12500.0/9600)) }
		b.WriteString(txTableRow(r.Name, f1s(r.ACP), f1s(r.ALT), f1s(r.Mask), f2s(r.FSKError), f2s(r.DevError), f2s(r.ETSIDev), ebn0(r.VoiceCN90), ebn0(r.DataCN90), ebn0(r.DiscVoiceCN90), ebn0(r.DiscDataCN90)))
	}
	b.WriteString("\nChannel power: lower is better. Mask D margin: higher is better. Receiver columns: Eb/N0 at which a reference receiver recovers 90% of the frames; lower is better.\n\n")
	writeTxCmpPaired(&b, d)
	b.WriteString("![Spectrum of each transmitter, power in 250 Hz against the offset from the carrier](" + txImgDir + "compare_spectrum.svg)\n")
	b.WriteString("![Eye diagram of each transmitter](" + txImgDir + "compare_eye.svg)\n\n")
	b.WriteString(txCmpEnd)
	return b.String()
}

func renderTxCmpMethods(d txCmpData) string {
	var b strings.Builder
	b.WriteString(txCmpMethodBegin + "\n\n")
	b.WriteString("- The MMDVM firmware runs its `DMRDMOTX.cpp` on the host. The tiers run at a Level of " + strconv.Itoa(txCmpLevel) + " codes, and the MMDVM firmware at a DMRTXLevel of 50, which gives " + strconv.Itoa(d.Rows[len(d.Rows)-1].Level) + " codes for a run of symbol +3. The stream is BS-sourced.\n")
	b.WriteString("- The radio is a sample-and-hold converter, an 8 kHz two-pole low pass and an FM modulator set to 1944 Hz for a run of +3.\n")
	b.WriteString("- Adjacent and alternate channel power is the power within 4.25 kHz of 12.5 and 25 kHz off, relative to the carrier, the larger side (EN 300 113 clause 7.4). The Mask D margin is the least margin of the peak spectrum to FCC Emission Mask D in 100 Hz.\n")
	b.WriteString("- FSK error and deviation error compare the symbol-center frequencies with 1944 and 648 Hz (TS 102 361-1 clause 10.2). The ETSI pattern is the +3 +3 -3 -3 test pattern of clause 10.2.2.3. The frequency is the phase step of the FM signal between samples at 48 kHz, divided by the sinc response of that step. The symbol centers come through a root-raised-cosine receive filter 128 symbols to each side, with the timing, gain and offset fitted over the stream. The ETSI pattern deviation is the peak of a fit of a constant and the 1200, 2400 and 3600 Hz tones over a whole number of periods.\n")
	b.WriteString("- The I/Q receiver is rf Tier 2 at 48 kHz, and the FM receiver is rf Tier 0 on the 9.6 kHz audio of a discriminator, over " + strconv.Itoa(d.Seeds) + " noise seeds a point.\n")
	b.WriteString("- The eye diagrams show the frequency of the FM signal after a 2.4 kHz low pass, over 120 symbol pairs.\n")
	b.WriteString("- `DMRGO_MMDVMTX=<mmdvmtx> go test ./internal/fsktx -run TestTxCompare -update-txcmp` in v2 measures them into `v2/internal/fsktx/testdata/txcmp.json` and writes the comparison and its graphs.\n\n")
	b.WriteString(txCmpMethodEnd)
	return b.String()
}

func txCmpClass(name string) string {
	if n, ok := strings.CutPrefix(name, "dmrgo audio tier "); ok {
		if k, err := strconv.Atoi(n); err == nil {
			return "s" + strconv.Itoa(k+1)
		}
	}
	return "r0"
}

func txCmpSpectrumSVG(d txCmpData) []byte {
	series := make([]plotSeries, 0, len(d.Rows))
	for _, r := range d.Rows {
		s := plotSeries{name: r.Name, class: txCmpClass(r.Name)}
		for i, v := range r.Spectrum {
			s.x = append(s.x, (-25000+float64(i)*d.BinHz)/1000)
			s.y = append(s.y, max(v, -100))
		}
		series = append(series, s)
	}
	return linePlot("Spectrum of each transmitter, power in 250 Hz",
		plotAxis{min: -25, max: 25, step: 5, label: "Offset from the carrier (kHz)"},
		plotAxis{min: -100, max: 0, step: 20, label: "Power in 250 Hz (dBc)"}, series)
}

func txCmpEyeSVG(d txCmpData) []byte {
	panels := make([]eyePanel, 0, len(d.Rows))
	for _, r := range d.Rows {
		title := strings.TrimPrefix(r.Name, "dmrgo ")
		p := eyePanel{title: strings.ToUpper(title[:1]) + title[1:], class: txCmpClass(r.Name)}
		for _, tr := range r.Eye {
			v := make([]float64, len(tr))
			for j, x := range tr {
				v[j] = x / 648
			}
			p.traces = append(p.traces, v)
		}
		panels = append(panels, p)
	}
	return eyePlot("Eye of each transmitter after a 2.4 kHz low pass", d.EyeSPS, panels)
}
