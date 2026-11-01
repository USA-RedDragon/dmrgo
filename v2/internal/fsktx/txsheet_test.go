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
	st := limStreamsOf(t, p.u, 1)
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
				v = max(v, dbOf(s.peak[i]))
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
		return txClassName(c)
	}
	return txClassName(c) + " at 48 kHz"
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
		out[txFileTag(c)+"_spectrum.svg"] = linePlot(fmt.Sprintf("Tier %d, %s: spectrum, peak in 100 Hz", c.tier-Tier0, txOutputName(c)),
			plotAxis{min: -40, max: 40, step: 10, label: "Offset from the carrier (kHz)"},
			plotAxis{min: -120, max: 20, step: 20, label: "Power to the reference (dB)"},
			[]plotSeries{
				{name: "Signal", class: "s2", x: fx, y: psd},
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
		out[txFileTag(c)+"_acp.svg"] = linePlot(fmt.Sprintf("Tier %d, %s: power in the EN 300 113 receiver filter", c.tier-Tier0, txOutputName(c)),
			plotAxis{min: 0, max: 40, step: 5, label: "Filter offset from the carrier (kHz)"},
			plotAxis{min: -110, max: 0, step: 10, label: "Power to the total (dBc)"},
			[]plotSeries{{name: "Signal", class: "s2", x: ox, y: acp}, adj, alt})
	}
}

func txEyeGraphs(out map[string][]byte, d txSheetData) {
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		var panels []eyePanel
		for _, e := range d.Eyes {
			c, ok := txClassByName(e.Class)
			if !ok || c.tier != tier || c.pe {
				continue
			}
			p := eyePanel{title: txClassName(c) + ", " + txRadioName(e.Radio)}
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
			g := barGroup{name: txClassName(c), class: "f" + strconv.Itoa(len(groups)+1)}
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
		rows = append(rows, fmt.Sprintf("Tier %d, %s", c.tier-Tier0, txClassName(c)))
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
		[]barGroup{{name: "Hold converter, or I/Q", class: "f2", v: hold}, {name: "Ideal converter", class: "f3", v: ideal}}, math.NaN())
}

const (
	sheetW, sheetH = 700, 420
	plotPadL       = 76
	plotPadR       = 190
	plotPadT       = 40
	plotPadB       = 56
	plotStyle      = `text{font-family:sans-serif;font-size:13px;fill:#24292f}.bg{fill:#ffffff}.ax{stroke:#24292f;stroke-width:1;fill:none}.gr{stroke:#d0d7de;stroke-width:1}.mi{stroke:#eaeef2;stroke-width:1}.r0{stroke:#6e7781}.r1{stroke:#bc4c00}.r2{stroke:#8250df}.r3{stroke:#1a7f37}.r4{stroke:#9a6700}.s1{stroke:#cf222e}.s2{stroke:#0969da}.s3{stroke:#1a7f37}.s4{stroke:#9a6700}.f1{fill:#cf222e}.f2{fill:#0969da}.f3{fill:#1a7f37}.f4{fill:#9a6700}.l{fill:none;stroke-width:2}.p{fill:none;stroke-width:1.5}.e{fill:none;stroke-width:1;stroke-opacity:.25}@media (prefers-color-scheme:dark){text{fill:#e6edf3}.bg{fill:#0d1117}.ax{stroke:#8b949e}.gr{stroke:#30363d}.mi{stroke:#21262d}.r0{stroke:#8b949e}.r1{stroke:#f0883e}.r2{stroke:#d2a8ff}.r3{stroke:#3fb950}.r4{stroke:#d29922}.s1{stroke:#ff7b72}.s2{stroke:#58a6ff}.s3{stroke:#3fb950}.s4{stroke:#d29922}.f1{fill:#ff7b72}.f2{fill:#58a6ff}.f3{fill:#3fb950}.f4{fill:#d29922}}`
)

type plotAxis struct {
	min, max, step float64
	label          string
	log            bool
}

func (a plotAxis) frac(v float64) float64 {
	if a.log {
		return (math.Log10(v) - math.Log10(a.min)) / (math.Log10(a.max) - math.Log10(a.min))
	}
	return (v - a.min) / (a.max - a.min)
}

func (a plotAxis) ticks() []float64 {
	var out []float64
	if a.log {
		for e := math.Ceil(math.Log10(a.min) - 1e-9); e <= math.Log10(a.max)+1e-9; e++ {
			out = append(out, math.Pow(10, e))
		}
		return out
	}
	for k := math.Ceil(a.min/a.step - 1e-9); k*a.step <= a.max+1e-9*math.Abs(a.step); k++ {
		out = append(out, k*a.step+0)
	}
	return out
}

func plotNum(v float64) string {
	if v != 0 && (math.Abs(v) < 1e-2 || math.Abs(v) >= 1e6) {
		return "1e" + strconv.Itoa(int(math.Round(math.Log10(math.Abs(v)))))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func plotPx(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

type plotSeries struct {
	name, class, dash string
	x, y              []float64
	marks             bool
}

type plotFrame struct {
	b      *strings.Builder
	x0, y0 float64
	w, h   float64
	xa, ya plotAxis
}

func (f plotFrame) sx(v float64) float64 { return f.x0 + f.xa.frac(v)*f.w }
func (f plotFrame) sy(v float64) float64 { return f.y0 + (1-f.ya.frac(v))*f.h }

func (f plotFrame) in(x, y float64) bool {
	return x >= f.xa.min && x <= f.xa.max && y >= f.ya.min && y <= f.ya.max && !math.IsNaN(y)
}

func plotOpen(b *strings.Builder, title string) {
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(sheetW) + `" height="` + strconv.Itoa(sheetH) +
		`" viewBox="0 0 ` + strconv.Itoa(sheetW) + ` ` + strconv.Itoa(sheetH) + `">` + "\n")
	b.WriteString("<style>" + plotStyle + "</style>\n")
	b.WriteString(`<rect class="bg" width="100%" height="100%"/>` + "\n")
	b.WriteString(`<text x="` + plotPx(plotPadL) + `" y="24">` + title + `</text>` + "\n")
}

func (f plotFrame) grid(xlabels, ylabels bool) {
	b := f.b
	for _, v := range f.xa.ticks() {
		x := f.sx(v)
		b.WriteString(`<line class="gr" x1="` + plotPx(x) + `" y1="` + plotPx(f.y0+f.h) + `" x2="` + plotPx(x) + `" y2="` + plotPx(f.y0) + `"/>` + "\n")
		if xlabels {
			b.WriteString(`<text x="` + plotPx(x) + `" y="` + plotPx(f.y0+f.h+18) + `" text-anchor="middle">` + plotNum(v) + `</text>` + "\n")
		}
	}
	for _, v := range f.ya.ticks() {
		y := f.sy(v)
		b.WriteString(`<line class="gr" x1="` + plotPx(f.x0) + `" y1="` + plotPx(y) + `" x2="` + plotPx(f.x0+f.w) + `" y2="` + plotPx(y) + `"/>` + "\n")
		if ylabels {
			b.WriteString(`<text x="` + plotPx(f.x0-8) + `" y="` + plotPx(y+4) + `" text-anchor="end">` + plotNum(v) + `</text>` + "\n")
		}
	}
	b.WriteString(`<rect class="ax" x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(f.w) + `" height="` + plotPx(f.h) + `"/>` + "\n")
}

func (f plotFrame) labels() {
	b := f.b
	b.WriteString(`<text x="` + plotPx(f.x0+f.w/2) + `" y="` + plotPx(sheetH-12) + `" text-anchor="middle">` + f.xa.label + `</text>` + "\n")
	cy := f.y0 + f.h/2
	b.WriteString(`<text x="18" y="` + plotPx(cy) + `" text-anchor="middle" transform="rotate(-90 18 ` + plotPx(cy) + `)">` + f.ya.label + `</text>` + "\n")
}

func (f plotFrame) series(s plotSeries) {
	var path, marks strings.Builder
	pen := false
	for k := range s.x {
		if k >= len(s.y) || !f.in(s.x[k], s.y[k]) {
			pen = false
			continue
		}
		op := "L"
		if !pen {
			op = "M"
		}
		pen = true
		x, y := plotPx(f.sx(s.x[k])), plotPx(f.sy(s.y[k]))
		path.WriteString(op + x + "," + y + " ")
		marks.WriteString(`<circle class="p ` + s.class + `" cx="` + x + `" cy="` + y + `" r="2"/>`)
	}
	if path.Len() == 0 {
		return
	}
	f.b.WriteString(`<path class="l ` + s.class + `"` + plotDash(s.dash) + ` d="` + strings.TrimSpace(path.String()) + `"/>` + "\n")
	if s.marks {
		f.b.WriteString(marks.String() + "\n")
	}
}

func plotDash(d string) string {
	if d == "" {
		return ""
	}
	return ` stroke-dasharray="` + d + `"`
}

func plotLegend(b *strings.Builder, i int, class, dash, name string) {
	x := float64(sheetW - plotPadR + 16)
	y := float64(plotPadT + 10 + i*22)
	b.WriteString(`<line class="l ` + class + `"` + plotDash(dash) + ` x1="` + plotPx(x) + `" y1="` + plotPx(y) + `" x2="` + plotPx(x+24) + `" y2="` + plotPx(y) + `"/>` + "\n")
	b.WriteString(`<text x="` + plotPx(x+30) + `" y="` + plotPx(y+4) + `">` + name + `</text>` + "\n")
}

func linePlot(title string, xa, ya plotAxis, series []plotSeries) []byte {
	var b strings.Builder
	plotOpen(&b, title)
	f := plotFrame{b: &b, x0: plotPadL, y0: plotPadT, w: sheetW - plotPadL - plotPadR, h: sheetH - plotPadT - plotPadB, xa: xa, ya: ya}
	f.grid(true, true)
	f.labels()
	i := 0
	for _, s := range series {
		f.series(s)
		if s.name != "" {
			plotLegend(&b, i, s.class, s.dash, s.name)
			i++
		}
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

type eyePanel struct {
	title  string
	traces [][]float64
}

func eyePlot(title string, steps int, panels []eyePanel) []byte {
	var b strings.Builder
	plotOpen(&b, title)
	const gap = 16
	n := float64(len(panels))
	w := (sheetW - plotPadL - 24 - gap*(n-1)) / n
	ya := plotAxis{min: -4.5, max: 4.5, step: 1.5, label: "Deviation (units of 648 Hz)"}
	xa := plotAxis{min: -1, max: 1, step: 0.5, label: "Time (symbols)"}
	for i, p := range panels {
		f := plotFrame{b: &b, x0: plotPadL + float64(i)*(w+gap), y0: plotPadT + 18, w: w, h: sheetH - plotPadT - 18 - plotPadB, xa: xa, ya: ya}
		f.grid(true, i == 0)
		b.WriteString(`<text x="` + plotPx(f.x0+w/2) + `" y="` + plotPx(f.y0-6) + `" text-anchor="middle">` + p.title + `</text>` + "\n")
		id := "eye" + strconv.Itoa(i)
		b.WriteString(`<clipPath id="` + id + `"><rect x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(w) + `" height="` + plotPx(f.h) + `"/></clipPath>` + "\n")
		b.WriteString(`<g class="e s2" clip-path="url(#` + id + `)">` + "\n")
		for _, tr := range p.traces {
			var path strings.Builder
			for k, v := range tr {
				op := "L"
				if k == 0 {
					op = "M"
				}
				x := float64(k-steps) / float64(steps)
				path.WriteString(op + strconv.Itoa(int(math.Round(f.sx(x)))) + "," + strconv.Itoa(int(math.Round(f.sy(v)))))
			}
			b.WriteString(`<path d="` + path.String() + `"/>` + "\n")
		}
		b.WriteString("</g>\n")
		for _, lv := range []float64{-3, -1, 1, 3} {
			y := plotPx(f.sy(lv))
			b.WriteString(`<line class="l r0" stroke-dasharray="2 4" x1="` + plotPx(f.x0) + `" y1="` + y + `" x2="` + plotPx(f.x0+w) + `" y2="` + y + `"/>` + "\n")
		}
		if i == 0 {
			cy := f.y0 + f.h/2
			b.WriteString(`<text x="18" y="` + plotPx(cy) + `" text-anchor="middle" transform="rotate(-90 18 ` + plotPx(cy) + `)">` + ya.label + `</text>` + "\n")
		}
	}
	b.WriteString(`<text x="` + plotPx((plotPadL+sheetW-24)/2) + `" y="` + plotPx(sheetH-12) + `" text-anchor="middle">` + xa.label + `</text>` + "\n")
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

type barGroup struct {
	name, class string
	v           []float64
}

func barPlot(title string, xa plotAxis, rows []string, groups []barGroup, ref float64) []byte {
	var b strings.Builder
	labelW := 300.0
	for _, name := range rows {
		labelW = max(labelW, 7*float64(len(name))+24)
	}
	width := int(labelW) + sheetW - 300
	rowH := 10.0*float64(len(groups)) + 8
	h := rowH*float64(len(rows)) + 40
	height := int(math.Ceil(plotPadT + h + plotPadB + 22*float64(len(groups))))
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) +
		`" viewBox="0 0 ` + strconv.Itoa(width) + ` ` + strconv.Itoa(height) + `">` + "\n")
	b.WriteString("<style>" + plotStyle + "</style>\n")
	b.WriteString(`<rect class="bg" width="100%" height="100%"/>` + "\n")
	b.WriteString(`<text x="16" y="24">` + title + `</text>` + "\n")
	f := plotFrame{b: &b, x0: labelW, y0: plotPadT, w: sheetW - 300 - 24, h: h - 40, xa: xa, ya: plotAxis{min: 0, max: 1, step: 1}}
	for _, v := range xa.ticks() {
		x := f.sx(v)
		b.WriteString(`<line class="gr" x1="` + plotPx(x) + `" y1="` + plotPx(f.y0+f.h) + `" x2="` + plotPx(x) + `" y2="` + plotPx(f.y0) + `"/>` + "\n")
		b.WriteString(`<text x="` + plotPx(x) + `" y="` + plotPx(f.y0+f.h+18) + `" text-anchor="middle">` + plotNum(v) + `</text>` + "\n")
	}
	b.WriteString(`<rect class="ax" x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(f.w) + `" height="` + plotPx(f.h) + `"/>` + "\n")
	b.WriteString(`<text x="` + plotPx(f.x0+f.w/2) + `" y="` + plotPx(f.y0+f.h+38) + `" text-anchor="middle">` + xa.label + `</text>` + "\n")
	zero := f.sx(max(xa.min, min(xa.max, 0)))
	for r, name := range rows {
		top := f.y0 + 4 + float64(r)*rowH
		b.WriteString(`<text x="` + plotPx(f.x0-8) + `" y="` + plotPx(top+rowH/2) + `" text-anchor="end">` + name + `</text>` + "\n")
		for g, grp := range groups {
			if r >= len(grp.v) || math.IsNaN(grp.v[r]) {
				continue
			}
			x := f.sx(max(xa.min, min(xa.max, grp.v[r])))
			lo, hi := min(zero, x), max(zero, x)
			b.WriteString(`<rect class="` + grp.class + `" x="` + plotPx(lo) + `" y="` + plotPx(top+float64(g)*10) + `" width="` + plotPx(max(hi-lo, 1)) + `" height="8"/>` + "\n")
		}
	}
	if !math.IsNaN(ref) {
		x := plotPx(f.sx(ref))
		b.WriteString(`<line class="l r1" stroke-dasharray="6 4" x1="` + x + `" y1="` + plotPx(f.y0) + `" x2="` + x + `" y2="` + plotPx(f.y0+f.h) + `"/>` + "\n")
	}
	for g, grp := range groups {
		y := f.y0 + f.h + 58 + float64(g)*22
		b.WriteString(`<rect class="` + grp.class + `" x="` + plotPx(f.x0) + `" y="` + plotPx(y-8) + `" width="24" height="8"/>` + "\n")
		b.WriteString(`<text x="` + plotPx(f.x0+30) + `" y="` + plotPx(y) + `">` + grp.name + `</text>` + "\n")
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
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

func renderTxSheet(t testing.TB, d txSheetData, cost txSheetCostData, margins map[string]map[string]float64) string {
	var b strings.Builder
	b.WriteString(txSheetBegin + "\n\n")
	cs := txSheetClasses()
	var shown []limClass
	for _, c := range cs {
		if !c.pe {
			shown = append(shown, c)
		}
	}
	head, sep := "| Parameter |", "| --- |"
	for _, c := range shown {
		head += fmt.Sprintf(" Tier %d, %s |", c.tier-Tier0, txClassName(c))
		sep += " ---: |"
	}
	b.WriteString(head + "\n" + sep + "\n")
	fskOf := func(c limClass, radio string) string {
		for _, f := range d.FSK {
			if f.Class == c.String() && f.Radio == radio {
				return fmt.Sprintf("%.2f%%", f.FSKError)
			}
		}
		return "-"
	}
	row := func(name string, cell func(c limClass) string) {
		b.WriteString("| " + name + " |")
		for _, c := range shown {
			b.WriteString(" " + cell(c) + " |")
		}
		b.WriteString("\n")
	}
	row("Sample rate", func(c limClass) string {
		if hz := txDocRate(c); hz >= 1000000 {
			return strconv.FormatFloat(float64(hz)/1e6, 'f', -1, 64) + " MHz"
		}
		return khz(txDocRate(c)) + " kHz"
	})
	row("FSK error, hold converter or I/Q", func(c limClass) string {
		if c.iq {
			return fskOf(c, "")
		}
		return fskOf(c, "plain")
	})
	row("FSK error, ideal converter", func(c limClass) string {
		if c.iq {
			return "-"
		}
		return fskOf(c, "ideal")
	})
	row("Deviation error, TS 102 361-1 pattern", func(c limClass) string {
		if v, ok := margins[c.String()]["ts_dev_pct"]; ok {
			return fmt.Sprintf("%.1f%%", 10-v)
		}
		return "-"
	})
	for _, r := range txRegimes {
		row("Margin over the need, "+r.name+", 1 W", func(c limClass) string {
			if v, ok := txRegimeMargin(t, margins[c.String()], r); ok {
				return fmt.Sprintf("%+.1f dB", v)
			}
			return "-"
		})
	}
	b.WriteString("\n- FSK error is the RMS error of the symbol deviations after the receive filter, as a percent of the deviation of symbol +3, over a random stream. A hold converter keeps each code until the next sample; an ideal converter interpolates it with no images. I/Q has no converter model.\n")
	b.WriteString("- Deviation error is the error of the peak deviation of the ETSI test pattern from 2749 Hz. TS 102 361-1 clause 10.2.2.3 sets a limit of 10%.\n")
	b.WriteString("- Each margin is the least, over the limits of the region and service at 1 W (30 dBm) with no input low pass, of the margin in dB less the margin that a setup needs on that limit: 10 dB on a spurious domain limit and 3 dB on the others. A setup is accepted from 0. [Measured margins](#measured-margins) gives each limit and power class.\n\n")
	if len(cost.Rows) > 0 {
		b.WriteString("| Tier 0 transmitter | Cortex-M4F CPU | RAM with stack | Flash |\n| --- | ---: | ---: | ---: |\n")
		for _, r := range cost.Rows {
			fmt.Fprintf(&b, "| %s | %.1f MHz | %d B | %.1f KB |\n", r.Name, r.MHz, r.RAMBytes, r.FlashKB)
		}
		b.WriteString("\n" + cost.Source + "\n\n")
	}
	b.WriteString(txSheetEnd)
	return b.String()
}

func renderTxLevels() string {
	var b strings.Builder
	b.WriteString(txLevelBegin + "\n\n| Bits | Tier 0 level | Tier 1 and Tier 2 level at 48 kHz | Tier 1 and Tier 2 peak at that level |\n| ---: | ---: | ---: | ---: |\n")
	for _, bits := range audioBitClasses {
		l0 := measuredLevel(Setup{Tier: Tier0, Bits: bits}, 48000)
		l1 := measuredLevel(Setup{Tier: Tier1, Bits: bits}, 48000)
		at := l1 >> (bits - Setup{Tier: Tier1, Bits: bits}.MeasuredBits())
		pk := "-"
		if a, err := NewAudio(AudioConfig{Hz: 48000, Level: at, Tier: Tier1}); err == nil {
			pk = fmt.Sprintf("%.2f times the level", a.Peak()/float64(at))
		}
		fmt.Fprintf(&b, "| %d | %d codes | %d codes | %s |\n", bits, l0, l1, pk)
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
	return txLowBegin + "\n`AudioTxConfig.InputLowpassHz` states the cutoff of a low pass of at least two poles that the radio puts before its modulator input. 0 is none: the converter drives the modulator with no filter. The limits are measured with a two-pole Butterworth low pass at " +
		strings.Join(grid[:len(grid)-1], ", ") + " and " + grid[len(grid)-1] + " Hz, and a stated cutoff takes the next looser of these. Thus " + strconv.Itoa(example) + " Hz counts as " + strconv.Itoa(counts) +
		" Hz, and a cutoff above " + grid[0] + " Hz counts as none. A low pass lowers the least rate that keeps the limits. A negative cutoff gets a `*dmrgo.ConfigError`.\n" + txLowEnd
}

func renderTxPreEmph() string {
	return txPEBegin + "\nThe microphone input of an FM radio usually pre-emphasizes the audio, with a zero near " + strconv.Itoa(preZeroHz) + " Hz and a pole near " + strconv.Itoa(prePoleHz) +
		" Hz, as the limits model it. A DMR signal through it has the wrong pulse shape and spreads into the adjacent channel. Use the flat modulator input of the radio (its data port) when it has one. When it does not, set `AudioTxConfig.PreEmphasis` to a `PreEmphasis`: the transmitter then shapes its output so that the pre-emphasis of the radio gives the right signal. It is opt-in: nil is a radio with no pre-emphasis. Set one up with `Init` in memory of the caller, or with `NewPreEmphasis`, one for each transmitter. Only Tier 0 at " +
		khz(peSamples*SymbolRate) + " kHz takes it, and only where its limits are measured to keep their margin: the Tier 0 table of [Least sample rate](#least-sample-rate) gives these setups. Elsewhere it gets a `RegionError`. A program that sets up no `PreEmphasis` links none of its code or tables.\n" + txPEEnd
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
	section("Spectrum", "The peak power in 100 Hz against the offset from the carrier, relative to the reference power of FCC Mask D, with Mask D for 1 W and 100 W and Mask H and the EA mask for 1 kW. Audio goes through the hold converter and an FM modulator.", func(n string) bool { return strings.HasSuffix(n, "_spectrum.svg") })
	section("Adjacent channel power", "The power that the receiver filter of EN 300 113 Annex C passes, against the offset of the filter from the carrier, relative to the total power. The dashed lines are the limit of EN 300 113 in the adjacent and alternate channels. The flat floor far out is the 90 dB stopband of the filter of Annex C, not the signal.", func(n string) bool { return strings.HasSuffix(n, "_acp.svg") })
	section("Margins", "The margin to each limit in dB at 1 W, the worst over the measured points, from `testdata/txmargins.json`.", func(n string) bool { return strings.HasSuffix(n, "_margins.svg") })
	section("Eye diagrams", "The deviation after the root raised cosine receive filter of the FSK error measurement, at 48 kHz, on the symbol timing that the measurement fits. Audio goes through an ideal converter, which interpolates the codes with no images, and through a hold converter, which keeps each code until the next sample.", func(n string) bool { return strings.HasSuffix(n, "_eye.svg") })
	section("FSK error", "The FSK error of each tier and output.", func(n string) bool { return n == "fsk_error.svg" })
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
		if !*updateTxDoc {
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
		if got := string(doc[i : j+len(blk.end)]); got != blk.want && !*updateTxDoc {
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
