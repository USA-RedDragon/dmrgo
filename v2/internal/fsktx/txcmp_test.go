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
	"math/cmplx"
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
	txCmpDir   = "../../../docs/tx"
	txCmpLevel = 960
	txCmpHz    = 24000
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
	f := make([]float64, len(x48))
	for i := 1; i < len(x48); i++ {
		d := x48[i] * cmplx.Conj(x48[i-1])
		f[i] = math.Atan2(imag(d), real(d)) * rxRate / (2 * math.Pi)
	}
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

func txCmpCurve(x48 []complex128, shift int, ts *testStream, disc bool) []point {
	n := int((2*rxPad + float64(len(ts.syms))/SymbolRate) * rxRate)
	var pts []point
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
		p := point{CN: cn}
		for _, s := range sc {
			p.Voice += s.voice
			p.VoiceOK += s.voiceOK
			p.Data += s.data
			p.DataOK += s.dataOK
		}
		pts = append(pts, p)
		if p.VoiceOK == p.Voice && p.DataOK == p.Data {
			break
		}
	}
	return pts
}

func txCmpMeasure(t *testing.T, name string, level int, x []complex128, rate float64, etsi []complex128, ts *testStream) txCmpRow {
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
	r.VoiceCN90, r.DataCN90 = cn90(txCmpCurve(x48, shift, ts, false))
	r.DiscVoiceCN90, r.DiscDataCN90 = cn90(txCmpCurve(x48, shift, ts, true))
	t.Logf("%s: %+v", name, r)
	return r
}

func measureTxCmp(t *testing.T, bin string) txCmpData {
	ts := newTestStream(t, 6, 40)
	d := txCmpData{MMDVM: os.Getenv("DMRGO_MMDVMTX_VERSION"), Seeds: txCmpSeeds, BinHz: 250, EyeSPS: rxRate / SymbolRate}
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		u := txUnit{tier: tier, format: fmtAudio, rate: txCmpHz, level: txCmpLevel}
		x := u.audioFM(u.run(t, ts.syms, nil), radio{})
		etsi := resample(u.audioFM(u.run(t, etsiSymbols(), nil), radio{}), dacRate, rxRate)
		d.Rows = append(d.Rows, txCmpMeasure(t, fmt.Sprintf("dmrgo audio tier %d", tier-Tier0), txCmpLevel, x, dacRate, etsi, ts))
	}
	lv := mmdvmLevel(t, bin)
	u := txUnit{format: fmtAudio, rate: txCmpHz, level: lv, offset: 2048}
	x := u.audioFM(mmdvmTX(t, bin, ts.syms), radio{})
	etsi := resample(u.audioFM(mmdvmTX(t, bin, etsiSymbols()), radio{}), dacRate, rxRate)
	d.Rows = append(d.Rows, txCmpMeasure(t, "MMDVM firmware", -lv, x, dacRate, etsi, ts))
	return d
}

func TestTxCompare(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(txDocFile)
	if errors.Is(err, fs.ErrNotExist) && !*updateTxCmp {
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
	i, j := bytes.Index(doc, []byte(txCmpBegin)), bytes.Index(doc, []byte(txCmpEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", txDocFile, txCmpBegin, txCmpEnd)
	}
	want := renderTxCmp(d)
	graphs := map[string][]byte{"compare_spectrum.svg": txCmpSpectrumSVG(d)}
	for k, r := range d.Rows {
		graphs["compare_eye_"+strconv.Itoa(k)+".svg"] = txCmpEyeSVG(d, r)
	}
	if *updateTxCmp {
		out := append(append(append([]byte{}, doc[:i]...), want...), doc[j+len(txCmpEnd):]...)
		if err := os.WriteFile(txDocFile, out, 0o600); err != nil {
			t.Fatal(err)
		}
		for name, b := range graphs {
			if err := os.WriteFile(filepath.Join(txCmpDir, name), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if got := string(doc[i : j+len(txCmpEnd)]); got != want {
		t.Errorf("the comparison block of %s differs from %s; run go test ./internal/fsktx -run TestTxCompare -update-txcmp in v2", txDocFile, txCmpFile)
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

func renderTxCmp(d txCmpData) string {
	var b strings.Builder
	b.WriteString(txCmpBegin + "\n\n")
	b.WriteString("## Other transmitters\n\n")
	b.WriteString("The audio tiers and the DMR DMO modulator of the MMDVM firmware (commit " + d.MMDVM + ", `DMRDMOTX.cpp` on the host) make the same BS-sourced symbol stream at " + strconv.Itoa(txCmpHz/1000) + " kHz into a 12-bit converter. The tiers run at a Level of " + strconv.Itoa(txCmpLevel) + " codes, and the MMDVM firmware at a DMRTXLevel of 50, which gives " + strconv.Itoa(d.Rows[len(d.Rows)-1].Level) + " codes for a run of symbol +3. Each output goes through the same radio: a sample-and-hold converter, an 8 kHz two-pole low pass and an FM modulator set to 1944 Hz for a run of +3.\n\n")
	b.WriteString("| Transmitter | Adjacent channel | Alternate channel | Mask D margin | FSK error | Deviation error | ETSI pattern deviation error | Voice at I/Q receiver | Data at I/Q receiver | Voice at FM receiver | Data at FM receiver |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range d.Rows {
		b.WriteString("| " + r.Name + " | " + f1s(r.ACP) + " dBc | " + f1s(r.ALT) + " dBc | " + f1s(r.Mask) + " dB | " + f2s(r.FSKError) + "% | " + f2s(r.DevError) + "% | " + f2s(r.ETSIDev) + "% | " + f1s(r.VoiceCN90+10*math.Log10(12500.0/9600)) + " dB | " + f1s(r.DataCN90+10*math.Log10(12500.0/9600)) + " dB | " + f1s(r.DiscVoiceCN90+10*math.Log10(12500.0/9600)) + " dB | " + f1s(r.DiscDataCN90+10*math.Log10(12500.0/9600)) + " dB |\n")
	}
	b.WriteString("\n- Adjacent and alternate channel power is the power within 4.25 kHz of 12.5 and 25 kHz off, relative to the carrier, the larger side (EN 300 113 clause 7.4). Lower is better.\n")
	b.WriteString("- The Mask D margin is the least margin of the peak spectrum to FCC Emission Mask D in 100 Hz. Higher is better.\n")
	b.WriteString("- FSK error and deviation error compare the symbol-center frequencies with 1944 and 648 Hz (TS 102 361-1 clause 10.2). The ETSI pattern is the +3 +3 -3 -3 test pattern of clause 10.2.2.3.\n")
	b.WriteString("- The receiver columns give the Eb/N0 at which a reference receiver recovers 90% of the voice frames and rate 1/2 blocks of the stream, over " + strconv.Itoa(d.Seeds) + " noise seeds a point: the I/Q receiver is rf Tier 2 at 48 kHz, the FM receiver is rf Tier 0 on the 9.6 kHz audio of a discriminator. Lower is better.\n\n")
	b.WriteString("![Spectrum of each transmitter, power in 250 Hz against the offset from the carrier](" + txImgDir + "compare_spectrum.svg)\n\n")
	b.WriteString("<details><summary>Eye diagrams</summary>\n\nThe frequency of the FM signal after a 2.4 kHz low pass, two symbols wide, over 120 symbol pairs.\n\n")
	for k, r := range d.Rows {
		b.WriteString("![Eye diagram of " + r.Name + "](" + txImgDir + "compare_eye_" + strconv.Itoa(k) + ".svg)\n")
	}
	b.WriteString("\n</details>\n\n")
	b.WriteString("`DMRGO_MMDVMTX=<mmdvmtx> go test ./internal/fsktx -run TestTxCompare -update-txcmp` in v2 measures them into `v2/internal/fsktx/testdata/txcmp.json` and writes this block and its graphs.\n\n")
	b.WriteString(txCmpEnd)
	return b.String()
}

const (
	txSvgW, txSvgH = 700, 360
	txPadL, txPadR = 64, 190
	txPadT, txPadB = 36, 48
	txSvgStyle     = `text{font-family:sans-serif;font-size:12px;fill:#57606a}.ax{stroke:#57606a;stroke-width:1;fill:none}.gr{stroke:#d0d7de;stroke-width:1}.s0{stroke:#cf222e}.s1{stroke:#0969da}.s2{stroke:#1a7f37}.s3{stroke:#9a6700}.l{fill:none;stroke-width:1.5}.e{fill:none;stroke-width:0.6;stroke-opacity:0.5}@media (prefers-color-scheme:dark){text{fill:#c9d1d9}.ax{stroke:#8b949e}.gr{stroke:#30363d}.s0{stroke:#ff7b72}.s1{stroke:#58a6ff}.s2{stroke:#3fb950}.s3{stroke:#d29922}}`
)

func txPx(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func txSvgFrame(b *strings.Builder, title, xTitle, yTitle string, xs, ys []float64, xmin, xmax, ymin, ymax float64) (func(float64) float64, func(float64) float64) {
	pw, ph := float64(txSvgW-txPadL-txPadR), float64(txSvgH-txPadT-txPadB)
	sx := func(x float64) float64 { return txPadL + (x-xmin)/(xmax-xmin)*pw }
	sy := func(y float64) float64 { return txPadT + (ymax-y)/(ymax-ymin)*ph }
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", txSvgW, txSvgH, txSvgW, txSvgH)
	b.WriteString("<style>" + txSvgStyle + "</style>\n")
	b.WriteString(`<text x="` + txPx(txPadL) + `" y="22">` + title + `</text>` + "\n")
	for _, x := range xs {
		fmt.Fprintf(b, `<line class="gr" x1="%s" y1="%s" x2="%s" y2="%s"/>`+"\n", txPx(sx(x)), txPx(sy(ymin)), txPx(sx(x)), txPx(sy(ymax)))
		fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="middle">%s</text>`+"\n", txPx(sx(x)), txPx(sy(ymin)+16), strconv.FormatFloat(x, 'f', -1, 64))
	}
	for _, y := range ys {
		fmt.Fprintf(b, `<line class="gr" x1="%s" y1="%s" x2="%s" y2="%s"/>`+"\n", txPx(sx(xmin)), txPx(sy(y)), txPx(sx(xmax)), txPx(sy(y)))
		fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="end">%s</text>`+"\n", txPx(sx(xmin)-6), txPx(sy(y)+4), strconv.FormatFloat(y, 'f', -1, 64))
	}
	fmt.Fprintf(b, `<rect class="ax" x="%s" y="%s" width="%s" height="%s"/>`+"\n", txPx(sx(xmin)), txPx(sy(ymax)), txPx(pw), txPx(ph))
	fmt.Fprintf(b, `<text x="%s" y="%s" text-anchor="middle">%s</text>`+"\n", txPx(sx((xmin+xmax)/2)), txPx(float64(txSvgH)-10), xTitle)
	fmt.Fprintf(b, `<text x="14" y="%s" transform="rotate(-90 14 %s)" text-anchor="middle">%s</text>`+"\n", txPx(sy((ymin+ymax)/2)), txPx(sy((ymin+ymax)/2)), yTitle)
	return sx, sy
}

func txCmpSpectrumSVG(d txCmpData) []byte {
	var b strings.Builder
	xs := []float64{-25, -20, -15, -10, -5, 0, 5, 10, 15, 20, 25}
	ys := []float64{0, -20, -40, -60, -80, -100}
	sx, sy := txSvgFrame(&b, "Spectrum of each transmitter", "Offset from the carrier (kHz)", "Power in 250 Hz (dBc)", xs, ys, -25, 25, -100, 0)
	for k, r := range d.Rows {
		var pts []string
		for i, v := range r.Spectrum {
			f := (-25000 + float64(i)*d.BinHz) / 1000
			pts = append(pts, txPx(sx(f))+","+txPx(sy(max(v, -100))))
		}
		fmt.Fprintf(&b, `<polyline class="l s%d" points="%s"/>`+"\n", k%4, strings.Join(pts, " "))
		ly := float64(txPadT + 20 + 22*k)
		lx := float64(txSvgW - txPadR + 14)
		fmt.Fprintf(&b, `<line class="l s%d" x1="%s" y1="%s" x2="%s" y2="%s"/>`+"\n", k%4, txPx(lx), txPx(ly-4), txPx(lx+20), txPx(ly-4))
		fmt.Fprintf(&b, `<text x="%s" y="%s">%s</text>`+"\n", txPx(lx+26), txPx(ly), r.Name)
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

func txCmpEyeSVG(d txCmpData, r txCmpRow) []byte {
	var b strings.Builder
	n := 2 * d.EyeSPS
	xs := []float64{0, 0.5, 1, 1.5, 2}
	ys := []float64{-3000, -2000, -1000, 0, 1000, 2000, 3000}
	sx, sy := txSvgFrame(&b, "Eye diagram of "+r.Name, "Time (symbols)", "Frequency (Hz)", xs, ys, 0, 2, -3000, 3000)
	for _, tr := range r.Eye {
		var pts []string
		for j, v := range tr {
			pts = append(pts, txPx(sx(float64(j)/float64(n)*2))+","+txPx(sy(max(-3000, min(3000, v)))))
		}
		fmt.Fprintf(&b, `<polyline class="e s1" points="%s"/>`+"\n", strings.Join(pts, " "))
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
