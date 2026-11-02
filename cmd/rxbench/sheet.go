package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type sheetEye struct {
	Mode   string    `json:"mode"`
	Tier   int       `json:"tier"`
	CN     float64   `json:"cn"`
	Traces [][]int16 `json:"traces"`
}

type sheetPoint struct {
	X        float64           `json:"x"`
	Payloads map[string]*score `json:"payloads"`
}

type sheetSweep struct {
	Mode   string       `json:"mode"`
	Tier   int          `json:"tier"`
	Points []sheetPoint `json:"points"`
}

type sheetAcq struct {
	CN    float64   `json:"cn"`
	Runs  int       `json:"runs"`
	Times []float64 `json:"times_ms"`
}

type sheetAcqTier struct {
	Mode   string     `json:"mode"`
	Tier   int        `json:"tier"`
	Points []sheetAcq `json:"points"`
}

type sheetTiming struct {
	Mode string  `json:"mode"`
	Tier int     `json:"tier"`
	Bias float64 `json:"bias_symbols"`
}

type sheetAccept struct {
	Hz    int    `json:"hz"`
	Tiers []bool `json:"tiers"`
}

type sheet struct {
	EyeSteps   int            `json:"eye_steps"`
	BiasStep   float64        `json:"bias_step_symbols"`
	OffsetCN   float64        `json:"offset_cn"`
	RateCN     float64        `json:"rate_cn"`
	AcqWindow  float64        `json:"acq_window_ms"`
	Timing     []sheetTiming  `json:"timing"`
	Eyes       []sheetEye     `json:"eyes"`
	Offset     []sheetSweep   `json:"offset"`
	RateError  []sheetSweep   `json:"rate_error"`
	Acquire    []sheetAcqTier `json:"acquire"`
	RatesIQ    []sheetAccept  `json:"rates_iq"`
	RatesAudio []sheetAccept  `json:"rates_audio"`
}

type mcuRow struct {
	Mode     string  `json:"mode"`
	Tier     int     `json:"tier"`
	Rate     int     `json:"rate"`
	PreEmph  bool    `json:"pre_emphasis"`
	MHz      float64 `json:"mhz"`
	RAMBytes int     `json:"ram_bytes"`
	FlashKB  float64 `json:"flash_kb"`
}

type mcuData struct {
	Source string   `json:"source"`
	Rows   []mcuRow `json:"rows"`
}

func renderMCU(b *strings.Builder, d data) {
	if len(d.mcu.Rows) == 0 {
		return
	}
	b.WriteString("### Tier 0 on a Cortex-M4F\n\n| Tier | Rate | CPU | RAM with stack | Flash |\n|------|------|-----|----------------|-------|\n")
	for _, r := range d.mcu.Rows {
		name := shortName(tier{Mode: r.Mode, Tier: r.Tier})
		if r.PreEmph {
			name += ", `PreEmphasis`"
		}
		fmt.Fprintf(b, "| %s | %s kHz | %s MHz | %s B | %s KB |\n", name, strconv.FormatFloat(float64(r.Rate)/1000, 'f', -1, 64), f1(r.MHz), grouped(int64(r.RAMBytes)), f1(r.FlashKB))
	}
	b.WriteString("\n" + d.mcu.Source + "\n\n")
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

const sheetPayload = keyAMBE

func recovered(p sheetPoint) float64 {
	s := p.Payloads[sheetPayload]
	if s == nil || s.Truth == 0 {
		return math.NaN()
	}
	return float64(s.Exact) / float64(s.Truth)
}

type sweepSpan struct {
	lo, hi         float64
	loEdge, hiEdge bool
}

func sweepRange(s sheetSweep) (sweepSpan, bool) {
	i := slices.IndexFunc(s.Points, func(p sheetPoint) bool { return p.X == 0 })
	if i < 0 || !(recovered(s.Points[i]) >= recoverMin) {
		return sweepSpan{}, false
	}
	a, b := i, i
	for a > 0 && recovered(s.Points[a-1]) >= recoverMin {
		a--
	}
	for b < len(s.Points)-1 && recovered(s.Points[b+1]) >= recoverMin {
		b++
	}
	return sweepSpan{lo: s.Points[a].X, hi: s.Points[b].X, loEdge: a == 0, hiEdge: b == len(s.Points)-1}, true
}

func (s sweepSpan) text(scale float64, unit string) string {
	f := func(v float64) string { return grouped64(v / scale) }
	pre := ""
	if s.loEdge && s.hiEdge {
		pre = "at least "
	}
	lo, hi := f(s.lo), f(s.hi)
	if s.loEdge && !s.hiEdge {
		lo = "below " + lo
	}
	if s.hiEdge && !s.loEdge {
		hi = "above " + hi
	}
	if s.lo == -s.hi && s.loEdge == s.hiEdge {
		return pre + "±" + f(s.hi) + " " + unit
	}
	return pre + lo + " to " + hi + " " + unit
}

func grouped64(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) >= 1000 {
		if v < 0 {
			return "-" + grouped(int64(-v))
		}
		return grouped(int64(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func findSweep(ss []sheetSweep, t tier) (sheetSweep, bool) {
	for _, s := range ss {
		if s.Mode == t.Mode && s.Tier == t.Tier {
			return s, true
		}
	}
	return sheetSweep{}, false
}

func findAcq(d sheet, t tier) (sheetAcqTier, bool) {
	for _, a := range d.Acquire {
		if a.Mode == t.Mode && a.Tier == t.Tier {
			return a, true
		}
	}
	return sheetAcqTier{}, false
}

func median(v []float64) (float64, bool) {
	if len(v) == 0 {
		return 0, false
	}
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2], true
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2, true
}

func acqAt(a sheetAcqTier, cn float64) string {
	for _, p := range a.Points {
		if p.CN != cn {
			continue
		}
		m, ok := median(p.Times)
		if !ok {
			return "-"
		}
		cell := strconv.FormatFloat(math.Round(m), 'f', 0, 64) + " ms"
		if len(p.Times) < p.Runs {
			cell += fmt.Sprintf(" (%d of %d)", len(p.Times), p.Runs)
		}
		return cell
	}
	return "-"
}

const (
	acqLowCN  = 8
	acqHighCN = 20
)

func floorCell(d data, k string) string {
	if f, ok := d.floor.Payloads[k]; ok {
		return dbCell(at90(f.Any.EbN0, f.Any.recovery()))
	}
	return "-"
}

func renderSpecs(d data) []byte {
	var b strings.Builder
	keys := []string{keyAMBE, keyRate12, keyRate34, keyCSBK, keyELC}
	b.WriteString("\n### Sensitivity\n\n")
	fmt.Fprintf(&b, "Eb/N0 at which the tier recovers %s of the frames exactly. Lower is better. C/N in %s is %s dB lower.\n\n", pct(recoverMin), bandText(), f1(cnToEbN0(0)))
	b.WriteString("| Tier | Voice | Data rate 1/2 | Data rate 3/4 | CSBK | Embedded LC |\n")
	b.WriteString("|------|-------|---------------|---------------|------|-------------|\n")
	row := make([]string, 0, 1+len(keys))
	row = append(row, "Best possible (4FSK floor)")
	for _, k := range keys {
		row = append(row, floorCell(d, k))
	}
	b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	for _, t := range d.bench.Tiers {
		row := make([]string, 0, 1+len(keys))
		row = append(row, shortName(t))
		for _, k := range keys {
			row = append(row, dbCell(at90(tierCurve(t, k))))
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	s := d.sheet
	b.WriteString("\n### Operating characteristics\n\n")
	fmt.Fprintf(&b, "| Tier | Input | Carrier offset | Sample clock error | Late entry at C/N %d dB | Late entry at C/N %d dB | CPU at C/N 6 dB | Worst-case CPU | RAM |\n", acqLowCN, acqHighCN)
	b.WriteString("|------|-------|----------------|--------------------|------|------|------|------|-----|\n")
	for _, t := range d.bench.Tiers {
		off, rate := "-", "-"
		if sw, ok := findSweep(s.Offset, t); ok {
			if r, ok := sweepRange(sw); ok {
				off = r.text(1000, "kHz")
			}
		}
		if sw, ok := findSweep(s.RateError, t); ok {
			if r, ok := sweepRange(sw); ok {
				rate = r.text(1, "ppm")
			}
		}
		lowAcq, highAcq := "-", "-"
		if a, ok := findAcq(s, t); ok {
			lowAcq, highAcq = acqAt(a, acqLowCN), acqAt(a, acqHighCN)
		}
		b.WriteString("| " + strings.Join([]string{shortName(t), inputName(t), off, rate, lowAcq, highAcq, msCell(t.NsPerSecond), msCell(t.WorstNs), ramCell(t.RAMBytes)}, " | ") + " |\n")
	}
	fmt.Fprintf(&b, "\n- Carrier offset is the range around zero, on the grid of [its graph](#carrier-offset), in which the tier recovers at least %s of the voice frames at C/N %s dB. Sample clock error is the same on the grid of [its graph](#sample-clock-error), at C/N %s dB. \"At least\" marks a range that reaches the ends of the grid.\n", pct(recoverMin), f1(s.OffsetCN), f1(s.RateCN))
	fmt.Fprintf(&b, "- Late entry is the median time from the first sample to the end of the first burst that the tier recovers exactly, when the input starts at a random point of a call. A count gives the runs that recovered a burst within %s ms, when some did not.\n", strconv.FormatFloat(s.AcqWindow, 'f', -1, 64))
	b.WriteString("- CPU is per second of signal, measured on an " + d.bench.Machine + ". Worst-case CPU is the highest over C/N 0 to 40 dB. RAM is the state of the receiver.")
	if !slices.ContainsFunc(d.bench.Tiers, func(t tier) bool { return t.Allocs > 0 }) {
		b.WriteString(" No tier allocates after it starts.")
	}
	b.WriteString("\n\n")
	renderMCU(&b, d)
	return []byte(b.String())
}

func ratesCell(rs []sheetAccept, tierIdx int) string {
	var spans []string
	start := -1
	for i, r := range rs {
		ok := tierIdx < len(r.Tiers) && r.Tiers[tierIdx]
		if ok && start < 0 {
			start = i
		}
		if ok && (i == len(rs)-1 || tierIdx >= len(rs[i+1].Tiers) || !rs[i+1].Tiers[tierIdx]) {
			if start == i {
				spans = append(spans, grouped(int64(r.Hz)))
			} else {
				spans = append(spans, grouped(int64(rs[start].Hz))+" to "+grouped(int64(r.Hz)))
			}
			start = -1
		}
	}
	if len(spans) == 0 {
		return "none"
	}
	return strings.Join(spans, ", ") + " Hz"
}

func renderLimits(d data) []byte {
	var b strings.Builder
	b.WriteString("\n| Tier | I/Q rates accepted | Audio rates accepted |\n|------|------|------|\n")
	n := 0
	for _, r := range append(slices.Clone(d.sheet.RatesIQ), d.sheet.RatesAudio...) {
		n = max(n, len(r.Tiers))
	}
	for i := range n {
		fmt.Fprintf(&b, "| Tier %d | %s | %s |\n", i, ratesCell(d.sheet.RatesIQ, i), ratesCell(d.sheet.RatesAudio, i))
	}
	b.WriteString("\nA span gives the first and last rate of the probe that the constructor accepts in a row. It returns an error for the other rates of the probe. The probe tries ")
	var all []string
	for _, r := range d.sheet.RatesIQ {
		all = append(all, grouped(int64(r.Hz)))
	}
	b.WriteString(strings.Join(all, ", ") + " Hz for I/Q and ")
	all = all[:0]
	for _, r := range d.sheet.RatesAudio {
		all = append(all, grouped(int64(r.Hz)))
	}
	b.WriteString(strings.Join(all, ", ") + " Hz for audio.\n\n")
	return []byte(b.String())
}

func renderTiming(b *strings.Builder, d data) {
	worst := 0.0
	for _, t := range d.sheet.Timing {
		worst = max(worst, math.Abs(t.Bias))
	}
	step := d.sheet.BiasStep
	if worst < step/2 {
		fmt.Fprintf(b, "On a clean signal, the symbol centers of every tier fall on `Burst.Seconds` plus whole symbols, within the search step of %s symbol (%s µs).\n\n", strconv.FormatFloat(step, 'f', -1, 64), f1(step/symbolRate*1e6))
		return
	}
	fmt.Fprintf(b, "On a clean signal, the symbol centers of each tier fall on `Burst.Seconds` plus whole symbols and this offset, which the eye diagrams take out:\n\n| Tier | Offset |\n|------|--------|\n")
	for _, t := range d.sheet.Timing {
		fmt.Fprintf(b, "| %s | %+.2f symbols (%+.0f µs) |\n", shortName(tier{Mode: t.Mode, Tier: t.Tier}), t.Bias, t.Bias/symbolRate*1e6)
	}
	b.WriteString("\n")
}

func sheetTiers(d data, m string) []tier {
	var out []tier
	for _, t := range d.bench.Tiers {
		if t.Mode == m {
			out = append(out, t)
		}
	}
	return out
}

func eyeGraphs(out map[string][]byte, d data, m string) {
	for _, t := range sheetTiers(d, m) {
		var panels []eyePanel
		for _, e := range d.sheet.Eyes {
			if e.Mode != t.Mode || e.Tier != t.Tier {
				continue
			}
			p := eyePanel{title: "C/N " + strconv.FormatFloat(e.CN, 'f', -1, 64) + " dB"}
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
			out[m+"/eye_tier"+strconv.Itoa(t.Tier)+".svg"] = eyePlot(shortName(t)+": eye at the matched filter, on the symbol timing of the tier", d.sheet.EyeSteps, panels)
		}
	}
}

func sweepGraph(d data, m, title string, xs []sheetSweep, xa plotAxis, scale float64) []byte {
	var series []plotSeries
	for _, t := range sheetTiers(d, m) {
		sw, ok := findSweep(xs, t)
		if !ok {
			continue
		}
		var x, y []float64
		for _, p := range sw.Points {
			if r := recovered(p); !math.IsNaN(r) {
				x, y = append(x, p.X*scale), append(y, 100*r)
			}
		}
		series = append(series, plotSeries{name: "Tier " + strconv.Itoa(t.Tier), class: "s" + strconv.Itoa(t.Tier+1), x: x, y: y, marks: true})
	}
	return linePlot(modeLabel(m)+": "+title, xa, plotAxis{min: 0, max: 100, step: 10, label: "Voice frames recovered (%)"}, series)
}

func sweepAxis(xs []sheetSweep, scale, step float64, label string) plotAxis {
	lo, hi := 0.0, 0.0
	for _, s := range xs {
		for _, p := range s.Points {
			lo, hi = min(lo, p.X*scale), max(hi, p.X*scale)
		}
	}
	return plotAxis{min: lo, max: hi, step: step, label: label}
}

func lateGraph(d data, m string) []byte {
	var series []plotSeries
	lo, hi, top := math.Inf(1), math.Inf(-1), 0.0
	for _, t := range sheetTiers(d, m) {
		a, ok := findAcq(d.sheet, t)
		if !ok {
			continue
		}
		var x, y []float64
		for _, p := range a.Points {
			if v, ok := median(p.Times); ok {
				x, y = append(x, p.CN), append(y, v)
				lo, hi, top = min(lo, p.CN), max(hi, p.CN), max(top, v)
			}
		}
		series = append(series, plotSeries{name: "Tier " + strconv.Itoa(t.Tier), class: "s" + strconv.Itoa(t.Tier+1), x: x, y: y, marks: true})
	}
	if len(series) == 0 {
		lo, hi = 0, 1
	}
	return linePlot(modeLabel(m)+": late entry, median time to the first exact burst", plotAxis{min: lo, max: hi, step: 2, label: "C/N in " + bandText() + " (dB)"},
		plotAxis{min: 0, max: math.Max(50, math.Ceil(top/50)*50), step: 25, label: "Time (ms)"}, series)
}

func sheetGraphs(d data) map[string][]byte {
	out := map[string][]byte{}
	s := d.sheet
	for _, m := range modes() {
		eyeGraphs(out, d, m[0])
		out[m[0]+"/offset.svg"] = sweepGraph(d, m[0], "voice frames against carrier offset, C/N "+f1(s.OffsetCN)+" dB", s.Offset, sweepAxis(s.Offset, 1e-3, 1, "Carrier offset (kHz)"), 1e-3)
		out[m[0]+"/clock.svg"] = sweepGraph(d, m[0], "voice frames against sample clock error, C/N "+f1(s.RateCN)+" dB", s.RateError, sweepAxis(s.RateError, 1, 500, "Sample clock error (ppm)"), 1)
		out[m[0]+"/late_entry.svg"] = lateGraph(d, m[0])
	}
	return out
}

func modeTitle(m string) string {
	for _, v := range modes() {
		if v[0] == m {
			return v[1]
		}
	}
	return m
}

func renderSheetGraphs(b *strings.Builder, d data) {
	b.WriteString("### Eye diagrams\n\n")
	renderTiming(b, d)
	for _, m := range modes() {
		for _, t := range sheetTiers(d, m[0]) {
			b.WriteString("![" + shortName(t) + ", " + m[1] + ": eye diagrams at three C/N](" + imgDir + m[0] + "/eye_tier" + strconv.Itoa(t.Tier) + ".svg)\n")
		}
	}
	b.WriteString("\n### Carrier offset\n\n")
	for _, m := range modes() {
		b.WriteString("![" + modeTitle(m[0]) + ": voice frames recovered against carrier offset](" + imgDir + m[0] + "/offset.svg)\n")
	}
	b.WriteString("\n### Sample clock error\n\n")
	for _, m := range modes() {
		b.WriteString("![" + modeTitle(m[0]) + ": voice frames recovered against sample clock error](" + imgDir + m[0] + "/clock.svg)\n")
	}
	b.WriteString("\n### Late entry\n\n")
	for _, m := range modes() {
		b.WriteString("![" + modeTitle(m[0]) + ": late entry time against C/N](" + imgDir + m[0] + "/late_entry.svg)\n")
	}
	b.WriteString("\n")
}
