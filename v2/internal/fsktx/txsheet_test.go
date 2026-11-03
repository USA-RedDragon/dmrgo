//go:build !tinygo

package fsktx

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	txSheetFile  = "testdata/txsheet.json"
	txSheetDir   = "../../../docs/tx/"
	txSheetBegin = "<!-- txsheet:begin -->"
	txSheetEnd   = "<!-- txsheet:end -->"
	txLevelBegin = "<!-- txsheet:levels:begin -->"
	txLevelEnd   = "<!-- txsheet:levels:end -->"
	txGraphBegin = "<!-- txsheet:graphs:begin -->"
	txGraphEnd   = "<!-- txsheet:graphs:end -->"
	txImgDir     = "tx/"
	txLowBegin   = "<!-- txsheet:lowpass:begin -->"
	txLowEnd     = "<!-- txsheet:lowpass:end -->"
	txPEBegin    = "<!-- txsheet:preemph:begin -->"
	txPEEnd      = "<!-- txsheet:preemph:end -->"
	txSpecSpan   = 40000
	txSpecStep   = 100
	txACPStep    = 250
	txEyeSteps   = 20
	txEyeTraces  = 128
)

type txSheetSpectrum struct {
	Class string  `json:"class"`
	PSD   []int16 `json:"psd_tenth_db"`
	ACP   []int16 `json:"acp_tenth_dbc"`
}

type txSheetEye struct {
	Class  string    `json:"class"`
	Radio  string    `json:"radio"`
	Traces [][]int16 `json:"traces"`
}

type txSheetFSK struct {
	Class    string  `json:"class"`
	Radio    string  `json:"radio"`
	FSKError float64 `json:"fsk_error_pct"`
	DevError float64 `json:"deviation_error_pct"`
}

type txSheetCost struct {
	Name     string  `json:"name"`
	MHz      float64 `json:"mhz"`
	RAMBytes int     `json:"ram_bytes"`
	FlashKB  float64 `json:"flash_kb"`
}

type txSheetData struct {
	Spectra []txSheetSpectrum `json:"spectra"`
	Eyes    []txSheetEye      `json:"eyes"`
	FSK     []txSheetFSK      `json:"fsk"`
}

type txSheetCostData struct {
	Source string        `json:"source"`
	Rows   []txSheetCost `json:"rows"`
}

const txCostFile = "testdata/tier0cost.json"

func txSheetClasses() []limClass {
	var out []limClass
	for _, c := range limClasses() {
		if (c.iq && c.format == fmtS16) || (!c.iq && c.bits == 16) {
			out = append(out, c)
		}
	}
	return out
}

func txSheetUnit(c limClass, r limRadio) limPoint {
	for _, p := range c.points(txDocRate(c), 0) {
		if p.u.audio() && p.radio != r {
			continue
		}
		if !p.u.audio() && p.u.offHz != 0 {
			continue
		}
		return p
	}
	return limPoint{}
}

func tenth(v float64) int16 {
	return int16(max(-32000, min(32000, math.Round(v*10))))
}

func txSheetSignal(t testing.TB, p limPoint, syms []int8) ([]complex128, float64) {
	if p.u.audio() {
		return p.audioSignal(p.u.run(t, syms, nil))
	}
	rate := float64(p.u.rate)
	return decimate(shift(iqComplex(p.u.run(t, syms, keyedOn())), rate, -float64(p.u.offHz)), rate)
}

func txSheetSpectrumOf(t testing.TB, c limClass) txSheetSpectrum {
	p := txSheetUnit(c, radioPlain)
	st := limStreamsOf(t, p.u, c.superframes())
	x, ra := txSheetSignal(t, p, st.cont)
	n := len(x)
	x = x[n/8 : n*7/8]
	pc := onPower(x)
	ref := limStatic(x, ra, pc)["maskd_ref_db"]
	s := limSpectra(x, ra, pc, 100, 2)
	out := txSheetSpectrum{Class: c.String()}
	for f := -txSpecSpan; f <= txSpecSpan; f += txSpecStep {
		v := math.Inf(-1)
		for i, fr := range s.f {
			if fr >= float64(f)-txSpecStep/2 && fr < float64(f)+txSpecStep/2 {
				v = max(v, dbOf(s.hold[i]))
			}
		}
		out.PSD = append(out.PSD, tenth(v-ref))
	}
	var tot float64
	for _, v := range s.bin {
		tot += v
	}
	for o := 0; o <= txSpecSpan; o += txACPStep {
		var worst float64
		for _, sign := range []float64{1, -1} {
			var a float64
			for i, f := range s.f {
				a += s.bin[i] * math.Pow(10, -annexC(f, sign*float64(o), "nominal")/10)
			}
			worst = max(worst, a)
		}
		out.ACP = append(out.ACP, tenth(dbOf(worst/tot)))
	}
	return out
}

func txSheetEyeOf(t testing.TB, c limClass, r limRadio) (txSheetEye, txSheetFSK) {
	p := txSheetUnit(c, r)
	ts := newTestStream(t, 2, 40)
	x, ra := txSheetSignal(t, p, ts.syms)
	x48 := resample(x, int(ra), rxRate)
	res := fsk(x48, ts.syms)
	f := demod(x48)
	sps := float64(rxRate / SymbolRate)
	gain := 1 + res.DevError/100
	lo := 200
	eye := txSheetEye{Class: c.String(), Radio: r.String()}
	if !p.u.audio() {
		eye.Radio = ""
	}
	cols := make([][]float64, 0, 2*txEyeSteps+1)
	for k := -txEyeSteps; k <= txEyeSteps; k++ {
		cols = append(cols, fskAt(f, res.Delay+float64(k)*sps/txEyeSteps, lo, lo+txEyeTraces))
	}
	for i := range txEyeTraces {
		tr := make([]int16, len(cols))
		for k, col := range cols {
			tr[k] = int16(math.Round((col[i] - res.OffsetHz) / (gain * DeviationHz) * 100))
		}
		eye.Traces = append(eye.Traces, tr)
	}
	return eye, txSheetFSK{Class: c.String(), Radio: eye.Radio, FSKError: math.Round(res.FSKError*1000) / 1000, DevError: math.Round(res.DevError*1000) / 1000}
}

func measureTxSheet(t *testing.T) txSheetData {
	var d txSheetData
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	run := func(f func()) {
		wg.Add(1)
		go func() {
			sem <- struct{}{}
			defer func() { <-sem; wg.Done() }()
			f()
		}()
	}
	for _, c := range txSheetClasses() {
		run(func() {
			s := txSheetSpectrumOf(t, c)
			mu.Lock()
			d.Spectra = append(d.Spectra, s)
			mu.Unlock()
		})
	}
	for _, c := range limClasses() {
		radios := []limRadio{radioPlain}
		if !c.iq {
			radios = append(radios, radioIdeal)
		}
		for _, r := range radios {
			run(func() {
				e, f := txSheetEyeOf(t, c, r)
				mu.Lock()
				d.FSK = append(d.FSK, f)
				if (c.iq && c.format == fmtS16) || (!c.iq && c.bits == 16) {
					d.Eyes = append(d.Eyes, e)
				}
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	order := map[string]int{}
	for i, c := range limClasses() {
		order[c.String()] = i
	}
	slices.SortFunc(d.Spectra, func(a, b txSheetSpectrum) int { return order[a.Class] - order[b.Class] })
	slices.SortFunc(d.Eyes, func(a, b txSheetEye) int {
		return cmp.Or(order[a.Class]-order[b.Class], strings.Compare(a.Radio, b.Radio))
	})
	slices.SortFunc(d.FSK, func(a, b txSheetFSK) int {
		return cmp.Or(order[a.Class]-order[b.Class], strings.Compare(a.Radio, b.Radio))
	})
	return d
}

func txClassByName(name string) (limClass, bool) {
	for _, c := range limClasses() {
		if c.String() == name {
			return c, true
		}
	}
	return limClass{}, false
}

func txOutputName(c limClass) string {
	if c.iq {
		return txOutput(c)
	}
	return txOutput(c) + " at 48 kHz"
}

func txFileTag(c limClass) string {
	switch {
	case c.iq:
		return fmt.Sprintf("tier%d_iq", c.tier-Tier0)
	case c.pe:
		return fmt.Sprintf("tier%d_audio_pe", c.tier-Tier0)
	}
	return fmt.Sprintf("tier%d_audio", c.tier-Tier0)
}

func txRadioName(r string) string {
	switch r {
	case "ideal":
		return "ideal converter"
	case "plain":
		return "hold converter"
	}
	return "I/Q"
}

func renderTxSheetGraphs(t testing.TB, d txSheetData, margins map[string]map[string]float64) map[string][]byte {
	out := map[string][]byte{}
	txSpectrumGraphs(out, d)
	txEyeGraphs(out, d)
	txMarginGraphs(t, out, margins)
	txFSKGraph(out, d)
	return out
}

func txSpectrumGraphs(out map[string][]byte, d txSheetData) {
	for _, s := range d.Spectra {
		c, ok := txClassByName(s.Class)
		if !ok {
			continue
		}
		var fx, psd, md1, md100, mh, ea []float64
		for i, v := range s.PSD {
			f := float64(-txSpecSpan + i*txSpecStep)
			fx = append(fx, f/1000)
			psd = append(psd, float64(v)/10)
			md1 = append(md1, -maskdAttn(f, 1))
			md100 = append(md100, -maskdAttn(f, 100))
			mh = append(mh, -maskhAttn(f, 1000))
			ea = append(ea, -eaAttn(f, 1000))
		}
		out[txFileTag(c)+"_spectrum.svg"] = linePlot(txTierName(c)+", "+txOutputName(c)+": spectrum, 10-minute peak hold in 100 Hz",
			plotAxis{min: -40, max: 40, step: 10, label: "Offset from the carrier (kHz)"},
			plotAxis{min: -120, max: 20, step: 20, label: "Power to the reference (dB)"},
			[]plotSeries{
				{name: "Signal", class: txTierClass(c.tier), x: fx, y: psd},
				{name: "Mask D, 1 W", class: "r1", dash: "6 4", x: fx, y: md1},
				{name: "Mask D, 100 W", class: "r2", dash: "6 4", x: fx, y: md100},
				{name: "Mask H, 1 kW", class: "r3", dash: "2 3", x: fx, y: mh},
				{name: "EA mask, 1 kW", class: "r4", dash: "2 3", x: fx, y: ea},
			})
		var ox, acp []float64
		for i, v := range s.ACP {
			ox = append(ox, float64(i*txACPStep)/1000)
			acp = append(acp, float64(v)/10)
		}
		lim := func(at float64) plotSeries {
			return plotSeries{class: "r1", dash: "6 4", x: []float64{at - 4.25, at + 4.25}, y: []float64{-60, -60}}
		}
		adj, alt := lim(12.5), lim(25)
		adj.name = "EN 300 113 limit"
		out[txFileTag(c)+"_acp.svg"] = linePlot(txTierName(c)+", "+txOutputName(c)+": power in the EN 300 113 receiver filter",
			plotAxis{min: 0, max: 40, step: 5, label: "Filter offset from the carrier (kHz)"},
			plotAxis{min: -110, max: 0, step: 10, label: "Power to the total (dBc)"},
			[]plotSeries{{name: "Signal", class: txTierClass(c.tier), x: ox, y: acp}, adj, alt})
	}
}

func txTierClass(t Tier) string { return "s" + strconv.Itoa(int(t-Tier0)+1) }

func txEyeGraphs(out map[string][]byte, d txSheetData) {
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		var panels []eyePanel
		for _, e := range d.Eyes {
			c, ok := txClassByName(e.Class)
			if !ok || c.tier != tier || c.pe {
				continue
			}
			p := eyePanel{title: txClassName(c) + ", " + txRadioName(e.Radio), class: txTierClass(tier)}
			if c.iq {
				p.title = txClassName(c)
			}
			for _, tr := range e.Traces {
				v := make([]float64, len(tr))
				for i, x := range tr {
					v[i] = float64(x) / 100
				}
				p.traces = append(p.traces, v)
			}
			panels = append(panels, p)
		}
		if len(panels) > 0 {
			out[fmt.Sprintf("tier%d_eye.svg", tier-Tier0)] = eyePlot(fmt.Sprintf("Tier %d: eye after the receive filter of the FSK error measurement", tier-Tier0), txEyeSteps, panels)
		}
	}
}

func txMarginGraphs(t testing.TB, out map[string][]byte, margins map[string]map[string]float64) {
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		var rows []string
		var keys [][]string
		for _, r := range txLimitRows() {
			k := r.at(t, 30)
			if len(k) == 0 || !strings.HasSuffix(k[0], "_db") || strings.HasPrefix(k[0], "en_slope_") {
				continue
			}
			rows, keys = append(rows, strings.Replace(r.name, ", dB", "", 1)), append(keys, k)
		}
		var groups []barGroup
		for _, c := range txTierClasses(tier) {
			if (c.iq && c.format != fmtS16) || (!c.iq && c.bits != 16) {
				continue
			}
			g := barGroup{name: txClassName(c), class: "q" + strconv.Itoa(len(groups)+1)}
			for _, k := range keys {
				v := math.NaN()
				for _, key := range k {
					if m, ok := margins[c.String()][key]; ok {
						m -= limNeed(key)
						if math.IsNaN(v) {
							v = m
						}
						v = min(v, m)
					}
				}
				g.v = append(g.v, v)
			}
			groups = append(groups, g)
		}
		out[fmt.Sprintf("tier%d_margins.svg", tier-Tier0)] = barPlot(fmt.Sprintf("Tier %d: margin to each limit at 1 W over the margin that a setup needs", tier-Tier0),
			plotAxis{min: -30, max: 60, step: 10, label: "Margin over the need (dB), accepted from 0"}, rows, groups, 0)
	}
}

func txFSKGraph(out map[string][]byte, d txSheetData) {
	var rows []string
	var hold, ideal []float64
	for _, f := range d.FSK {
		c, ok := txClassByName(f.Class)
		if !ok {
			continue
		}
		if f.Radio == "ideal" {
			continue
		}
		rows = append(rows, txTierName(c)+", "+txOutput(c))
		hold = append(hold, f.FSKError)
		iv := math.NaN()
		for _, g := range d.FSK {
			if g.Class == f.Class && g.Radio == "ideal" {
				iv = g.FSKError
			}
		}
		ideal = append(ideal, iv)
	}
	top := 0.5
	for _, v := range slices.Concat(hold, ideal) {
		if !math.IsNaN(v) {
			top = max(top, math.Ceil(v*2)/2)
		}
	}
	out["fsk_error.svg"] = barPlot("FSK error per tier and output",
		plotAxis{min: 0, max: top, step: 0.25, label: "FSK error (% of the deviation of symbol +3)"}, rows,
		[]barGroup{{name: "Hold converter, or I/Q", class: "q1", v: hold}, {name: "Ideal converter", class: "q2", v: ideal}}, math.NaN())
}

func txRegimeMargin(t testing.TB, g map[string]float64, r txRegime) (float64, bool) {
	t.Helper()
	keys, err := regimeKeys(r.region, r.service, 30)
	if err != nil {
		t.Fatal(err)
	}
	worst, ok := math.Inf(1), false
	for _, k := range keys {
		if !strings.HasSuffix(k, "_db") || strings.HasPrefix(k, "en_slope_") {
			continue
		}
		if v, has := g[k]; has {
			worst, ok = min(worst, v-limNeed(k)), true
		}
	}
	return worst, ok
}

func txTableHead(cols ...string) string {
	var h, sep strings.Builder
	for i, c := range cols {
		h.WriteString("| " + c + " ")
		if i == 0 || c == "Output" || c == "Limit" || c == "Input" {
			sep.WriteString("|:---")
		} else {
			sep.WriteString("|---:")
		}
	}
	return h.String() + "|\n" + sep.String() + "|\n"
}

func txTableRow(cells ...string) string { return "| " + strings.Join(cells, " | ") + " |\n" }

func txTierName(c limClass) string {
	if c.iq {
		return fmt.Sprintf("I/Q tier %d", c.tier-Tier0)
	}
	return fmt.Sprintf("Audio tier %d", c.tier-Tier0)
}

func txOutput(c limClass) string {
	_, out, _ := strings.Cut(txClassName(c), " ")
	return out
}

func txRateKHz(hz int) string {
	s := khz(hz)
	if whole, frac, _ := strings.Cut(s, "."); len(whole) > 3 {
		s = whole[:len(whole)-3] + "," + whole[len(whole)-3:]
		if frac != "" {
			s += "." + frac
		}
	}
	return s
}

func txSheetRows() []limClass {
	var audio, iq []limClass
	for _, c := range txSheetClasses() {
		switch {
		case c.pe:
		case c.iq:
			iq = append(iq, c)
		default:
			audio = append(audio, c)
		}
	}
	return append(audio, iq...)
}

func renderTxSheet(t testing.TB, d txSheetData, cost txSheetCostData, margins map[string]map[string]float64) string {
	var b strings.Builder
	b.WriteString(txSheetBegin + "\n\n")
	shown := txSheetRows()
	fskOf := func(c limClass, radio string) string {
		for _, f := range d.FSK {
			if f.Class == c.String() && f.Radio == radio {
				return fmt.Sprintf("%.2f", f.FSKError)
			}
		}
		return "-"
	}
	b.WriteString("### Signal quality\n\n")
	b.WriteString(txTableHead("Tier", "Output", "Rate (kHz)", "FSK error, hold (%)", "FSK error, ideal (%)", "Deviation error (%)"))
	for _, c := range shown {
		hold, ideal := fskOf(c, "plain"), fskOf(c, "ideal")
		if c.iq {
			hold, ideal = fskOf(c, ""), "-"
		}
		dev := "-"
		if v, ok := margins[c.String()]["ts_dev_pct"]; ok {
			dev = fmt.Sprintf("%.1f", 10-v)
		}
		b.WriteString(txTableRow(txTierName(c), txOutput(c), txRateKHz(txDocRate(c)), hold, ideal, dev))
	}
	b.WriteString("\n- FSK error: RMS error of the symbol deviations after the receive filter, as a percent of the deviation of symbol +3. Hold: a converter that keeps each code until the next sample, or I/Q. Ideal: a converter with no images.\n")
	b.WriteString("- Deviation error: error of the peak deviation of the TS 102 361-1 test pattern from 2749 Hz. The limit is 10%.\n\n")
	b.WriteString("### Margin over the need at 1 W\n\n")
	b.WriteString("Least margin in dB to the limits of each region and service at 1 W (30 dBm), less the margin that a setup needs: 10 dB on a spurious domain limit and 3 dB on the others. A setup is accepted from 0. Higher is better.\n\n")
	cols := make([]string, 0, 2+len(txRegimes))
	cols = append(cols, "Tier", "Output")
	for _, r := range txRegimes {
		cols = append(cols, r.name)
	}
	b.WriteString(txTableHead(cols...))
	for _, c := range shown {
		cells := make([]string, 0, len(cols))
		cells = append(cells, txTierName(c), txOutput(c))
		for _, r := range txRegimes {
			cell := "-"
			if v, ok := txRegimeMargin(t, margins[c.String()], r); ok {
				cell = fmt.Sprintf("%+.1f", v)
			}
			cells = append(cells, cell)
		}
		b.WriteString(txTableRow(cells...))
	}
	b.WriteString("\n")
	if len(cost.Rows) > 0 {
		b.WriteString("### Tier 0 on a Cortex-M4F\n\n")
		b.WriteString(txTableHead("Tier", "Rate (kHz)", "CPU (MHz)", "RAM with stack (B)", "Flash (KB)"))
		rows := slices.Clone(cost.Rows)
		slices.SortStableFunc(rows, func(a, b txSheetCost) int {
			return cmp.Compare(strings.Index(a.Name, "I/Q"), strings.Index(b.Name, "I/Q"))
		})
		for _, r := range rows {
			kind, rest, _ := strings.Cut(r.Name, " at ")
			rate, out, _ := strings.Cut(rest, " kHz, ")
			b.WriteString(txTableRow(kind+" tier 0, "+out, rate, fmt.Sprintf("%.1f", r.MHz), strconv.Itoa(r.RAMBytes), fmt.Sprintf("%.1f", r.FlashKB)))
		}
		b.WriteString("\n" + cost.Source + "\n\n")
	}
	b.WriteString(txSheetEnd)
	return b.String()
}

func renderTxLevels() string {
	var b strings.Builder
	b.WriteString(txLevelBegin + "\n\n" + txTableHead("Bits", "Tier 0 level (codes)", "Tier 1 and 2 level at 48 kHz (codes)", "Tier 1 and 2 peak (times the level)"))
	for _, bits := range audioBitClasses {
		l0 := measuredLevel(Setup{Tier: Tier0, Bits: bits}, 48000)
		l1 := measuredLevel(Setup{Tier: Tier1, Bits: bits}, 48000)
		at := l1 >> (bits - Setup{Tier: Tier1, Bits: bits}.MeasuredBits())
		pk := "-"
		if a, err := NewAudio(AudioConfig{Hz: 48000, Level: at, Tier: Tier1}); err == nil {
			pk = fmt.Sprintf("%.2f", a.Peak()/float64(at))
		}
		fmt.Fprintf(&b, "| %d | %d | %d | %s |\n", bits, l0, l1, pk)
	}
	b.WriteString("\n" + txLevelEnd)
	return b.String()
}

func renderTxLowpass() string {
	grid := make([]string, 0, len(inputLowpassGrid)-1)
	for _, hz := range inputLowpassGrid[1:] {
		grid = append(grid, strconv.Itoa(hz))
	}
	example := inputLowpassGrid[2] + 1000
	counts := inputLowpassGrid[lowpassClass(example)]
	return txLowBegin + "\nState a low pass of the radio before its modulator input in `AudioTxConfig.InputLowpassHz`: it lowers the least rate that keeps the limits. The limits are measured with a two-pole Butterworth low pass at " +
		strings.Join(grid[:len(grid)-1], ", ") + " and " + grid[len(grid)-1] + " Hz, and a stated cutoff takes the next looser of these: " + strconv.Itoa(example) + " Hz counts as " + strconv.Itoa(counts) +
		" Hz, and a cutoff above " + grid[0] + " Hz as none.\n" + txLowEnd
}

func renderTxPreEmph() string {
	return txPEBegin + "\nUse the flat modulator input (data port) of the radio when it has one. The microphone input pre-emphasizes the audio, with a zero near " + strconv.Itoa(preZeroHz) + " Hz and a pole near " + strconv.Itoa(prePoleHz) +
		" Hz, and spreads a DMR signal into the adjacent channel. For it, set `AudioTxConfig.PreEmphasis` to a `PreEmphasis` from `Init` or `NewPreEmphasis`, one for each transmitter. Every tier takes it, in the setups of the pre-emphasis rows of [Least sample rate](#least-sample-rate).\n" + txPEEnd
}

func renderTxGraphList(graphs map[string][]byte) string {
	var b strings.Builder
	b.WriteString(txGraphBegin + "\n\n")
	section := func(title, intro string, match func(name string) bool) {
		b.WriteString("### " + title + "\n\n" + intro + "\n\n")
		for _, name := range txGraphList(graphs) {
			if match(name) {
				b.WriteString("![" + title + ": " + strings.TrimSuffix(name, ".svg") + "](" + txImgDir + name + ")\n")
			}
		}
		b.WriteString("\n")
	}
	section("Spectrum", "10-minute peak hold in 100 Hz against the offset from the carrier, relative to the reference power of FCC Mask D, with the masks dashed. Audio goes through the hold converter and an FM modulator.", func(n string) bool { return strings.HasSuffix(n, "_spectrum.svg") })
	section("Adjacent channel power", "Power that the receiver filter of EN 300 113 Annex C passes, relative to the total, with the limit of EN 300 113 dashed. The floor far out is the 90 dB stopband of that filter.", func(n string) bool { return strings.HasSuffix(n, "_acp.svg") })
	section("Margins", "Margin to each limit at 1 W over the margin that a setup needs, the worst over the measured points.", func(n string) bool { return strings.HasSuffix(n, "_margins.svg") })
	section("Eye diagrams", "Deviation after the receive filter of the FSK error measurement, at 48 kHz, on the symbol timing that the measurement fits.", func(n string) bool { return strings.HasSuffix(n, "_eye.svg") })
	section("FSK error", "FSK error of each tier and output.", func(n string) bool { return n == "fsk_error.svg" })
	b.WriteString(txGraphEnd)
	return b.String()
}

func txGraphList(graphs map[string][]byte) []string {
	names := make([]string, 0, len(graphs))
	for k := range graphs {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

func txSheetDoc(t *testing.T, doc []byte, margins map[string]map[string]float64) []byte {
	t.Helper()
	var d txSheetData
	if *updateTxDoc {
		d = measureTxSheet(t)
		js, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(txSheetFile, append(js, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	} else if err := readJSON(txSheetFile, &d); err != nil {
		t.Fatal(err)
	}
	var cost txSheetCostData
	if err := readJSON(txCostFile, &cost); err != nil {
		t.Fatal(err)
	}
	graphs := renderTxSheetGraphs(t, d, margins)
	for _, name := range txGraphList(graphs) {
		p := filepath.Join(txSheetDir, name)
		old, err := os.ReadFile(p)
		if err == nil && bytes.Equal(old, graphs[name]) {
			continue
		}
		if !writeTxDoc() {
			t.Errorf("%s differs from %s; run go test ./internal/fsktx -run TestTxDoc -update-txdoc in v2", p, txSheetFile)
			continue
		}
		if err := os.WriteFile(p, graphs[name], 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, blk := range []struct{ begin, end, want string }{
		{txSheetBegin, txSheetEnd, renderTxSheet(t, d, cost, margins)},
		{txLevelBegin, txLevelEnd, renderTxLevels()},
		{txGraphBegin, txGraphEnd, renderTxGraphList(graphs)},
		{txLowBegin, txLowEnd, renderTxLowpass()},
		{txPEBegin, txPEEnd, renderTxPreEmph()},
	} {
		i, j := bytes.Index(doc, []byte(blk.begin)), bytes.Index(doc, []byte(blk.end))
		if i < 0 || j < i {
			t.Fatalf("%s has no %s ... %s block", txDocFile, blk.begin, blk.end)
		}
		if got := string(doc[i : j+len(blk.end)]); got != blk.want && !writeTxDoc() {
			t.Errorf("the %s block of %s differs from %s; run go test ./internal/fsktx -run TestTxDoc -update-txdoc in v2", blk.begin, txDocFile, txSheetFile)
		}
		doc = append(append(append([]byte{}, doc[:i]...), blk.want...), doc[j+len(blk.end):]...)
	}
	return doc
}

func readJSON(path string, v any) error {
	js, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(js, v)
}
