package main

import (
	"math"
	"strconv"
	"strings"
)

const (
	plotWidth, plotHeight = 700, 420
	plotPadL, plotPadR    = 76, 24
	plotPadT, plotPadB    = 76, 56
	plotLegendY           = 46
	plotLegendStep        = 20
	plotCharW             = 7
	plotStyle             = `text{font-family:sans-serif;font-size:13px;fill:#24292f}.t{font-weight:600}.bg{fill:#ffffff}.ax{stroke:#24292f;stroke-width:1;fill:none}.gr{stroke:#d0d7de;stroke-width:1}.mi{stroke:#eaeef2;stroke-width:1}.r0{stroke:#6e7781}.r1{stroke:#bc4c00}.r2{stroke:#8250df}.r3{stroke:#1b7c83}.r4{stroke:#9a6700}.s1{stroke:#cf222e}.s2{stroke:#0969da}.s3{stroke:#1a7f37}.s4{stroke:#9a6700}.q0{fill:#6e7781}.q1{fill:#bc4c00}.q2{fill:#8250df}.f1{fill:#cf222e}.f2{fill:#0969da}.f3{fill:#1a7f37}.f4{fill:#9a6700}.l{fill:none;stroke-width:2}.p{fill:none;stroke-width:1.5}.e{fill:none;stroke-width:1;stroke-opacity:.25}@media (prefers-color-scheme:dark){text{fill:#e6edf3}.bg{fill:#0d1117}.ax{stroke:#8b949e}.gr{stroke:#30363d}.mi{stroke:#21262d}.r0{stroke:#8b949e}.r1{stroke:#f0883e}.r2{stroke:#d2a8ff}.r3{stroke:#39c5cf}.r4{stroke:#d29922}.s1{stroke:#ff7b72}.s2{stroke:#58a6ff}.s3{stroke:#3fb950}.s4{stroke:#d29922}.q0{fill:#8b949e}.q1{fill:#f0883e}.q2{fill:#d2a8ff}.f1{fill:#ff7b72}.f2{fill:#58a6ff}.f3{fill:#3fb950}.f4{fill:#d29922}}`
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

func (a plotAxis) minorTicks() []float64 {
	if !a.log {
		return nil
	}
	var out []float64
	for _, d := range a.ticks() {
		for m := 2; m < 10; m++ {
			if v := float64(m) * d; v < a.max {
				out = append(out, v)
			}
		}
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

func plotOpen(b *strings.Builder, width, height int, title string) {
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(width) + `" height="` + strconv.Itoa(height) +
		`" viewBox="0 0 ` + strconv.Itoa(width) + ` ` + strconv.Itoa(height) + `">` + "\n")
	b.WriteString("<style>" + plotStyle + "</style>\n")
	b.WriteString(`<rect class="bg" width="100%" height="100%"/>` + "\n")
	b.WriteString(`<text class="t" x="16" y="24">` + title + `</text>` + "\n")
}

func (f plotFrame) hline(class string, y float64) {
	f.b.WriteString(`<line class="` + class + `" x1="` + plotPx(f.x0) + `" y1="` + plotPx(y) + `" x2="` + plotPx(f.x0+f.w) + `" y2="` + plotPx(y) + `"/>` + "\n")
}

func (f plotFrame) grid(xlabels, ylabels bool) {
	b := f.b
	for _, v := range f.ya.minorTicks() {
		f.hline("mi", f.sy(v))
	}
	for _, v := range f.xa.ticks() {
		x := f.sx(v)
		b.WriteString(`<line class="gr" x1="` + plotPx(x) + `" y1="` + plotPx(f.y0+f.h) + `" x2="` + plotPx(x) + `" y2="` + plotPx(f.y0) + `"/>` + "\n")
		if xlabels {
			b.WriteString(`<text x="` + plotPx(x) + `" y="` + plotPx(f.y0+f.h+18) + `" text-anchor="middle">` + plotNum(v) + `</text>` + "\n")
		}
	}
	for _, v := range f.ya.ticks() {
		y := f.sy(v)
		f.hline("gr", y)
		if ylabels {
			b.WriteString(`<text x="` + plotPx(f.x0-8) + `" y="` + plotPx(y+4) + `" text-anchor="end">` + plotNum(v) + `</text>` + "\n")
		}
	}
	b.WriteString(`<rect class="ax" x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(f.w) + `" height="` + plotPx(f.h) + `"/>` + "\n")
}

func (f plotFrame) xLabel(label string) {
	f.b.WriteString(`<text x="` + plotPx(f.x0+f.w/2) + `" y="` + plotPx(f.y0+f.h+plotPadB-12) + `" text-anchor="middle">` + label + `</text>` + "\n")
}

func (f plotFrame) yLabel(label string) {
	cy := f.y0 + f.h/2
	f.b.WriteString(`<text x="18" y="` + plotPx(cy) + `" text-anchor="middle" transform="rotate(-90 18 ` + plotPx(cy) + `)">` + label + `</text>` + "\n")
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

type legendItem struct {
	name, class, dash string
	box               bool
}

func plotLegend(b *strings.Builder, x0, x1 float64, items []legendItem) {
	x, y := x0, float64(plotLegendY)
	for _, it := range items {
		w := 30 + plotCharW*float64(len(it.name))
		if x > x0 && x+w > x1 {
			x, y = x0, y+plotLegendStep
		}
		if it.box {
			b.WriteString(`<rect class="` + it.class + `" x="` + plotPx(x) + `" y="` + plotPx(y-4) + `" width="24" height="8"/>` + "\n")
		} else {
			b.WriteString(`<line class="l ` + it.class + `"` + plotDash(it.dash) + ` x1="` + plotPx(x) + `" y1="` + plotPx(y) + `" x2="` + plotPx(x+24) + `" y2="` + plotPx(y) + `"/>` + "\n")
		}
		b.WriteString(`<text x="` + plotPx(x+30) + `" y="` + plotPx(y+4) + `">` + it.name + `</text>` + "\n")
		x += w + 18
	}
}

func linePlot(title string, xa, ya plotAxis, series []plotSeries) []byte {
	var b strings.Builder
	plotOpen(&b, plotWidth, plotHeight, title)
	f := plotFrame{b: &b, x0: plotPadL, y0: plotPadT, w: plotWidth - plotPadL - plotPadR, h: plotHeight - plotPadT - plotPadB, xa: xa, ya: ya}
	f.grid(true, true)
	f.xLabel(xa.label)
	f.yLabel(ya.label)
	var items []legendItem
	for _, s := range series {
		f.series(s)
		if s.name != "" {
			items = append(items, legendItem{name: s.name, class: s.class, dash: s.dash})
		}
	}
	plotLegend(&b, plotPadL, plotWidth-plotPadR, items)
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

type eyePanel struct {
	title, class string
	traces       [][]float64
}

func eyePlot(title string, steps int, panels []eyePanel) []byte {
	var b strings.Builder
	plotOpen(&b, plotWidth, plotHeight, title)
	const gap = 16
	n := float64(len(panels))
	w := (plotWidth - plotPadL - plotPadR - gap*(n-1)) / n
	ya := plotAxis{min: -4.5, max: 4.5, step: 1.5, label: "Deviation (units of 648 Hz)"}
	xa := plotAxis{min: -1, max: 1, step: 0.5, label: "Time (symbols)"}
	var all plotFrame
	for i, p := range panels {
		f := plotFrame{b: &b, x0: plotPadL + float64(i)*(w+gap), y0: plotPadT, w: w, h: plotHeight - plotPadT - plotPadB, xa: xa, ya: ya}
		f.grid(true, i == 0)
		b.WriteString(`<text x="` + plotPx(f.x0+w/2) + `" y="` + plotPx(f.y0-8) + `" text-anchor="middle">` + p.title + `</text>` + "\n")
		id := "eye" + strconv.Itoa(i)
		b.WriteString(`<clipPath id="` + id + `"><rect x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(w) + `" height="` + plotPx(f.h) + `"/></clipPath>` + "\n")
		b.WriteString(`<g class="e ` + p.class + `" clip-path="url(#` + id + `)">` + "\n")
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
			f.b.WriteString(`<line class="l r0" stroke-dasharray="2 4" x1="` + plotPx(f.x0) + `" y1="` + plotPx(f.sy(lv)) + `" x2="` + plotPx(f.x0+w) + `" y2="` + plotPx(f.sy(lv)) + `"/>` + "\n")
		}
		if i == 0 {
			all = f
			f.yLabel(ya.label)
		}
	}
	all.w = plotWidth - plotPadL - plotPadR
	all.xLabel(xa.label)
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

type barGroup struct {
	name, class string
	v           []float64
}

func barPlot(title string, xa plotAxis, rows []string, groups []barGroup, ref float64) []byte {
	var b strings.Builder
	labelW := 140.0
	for _, name := range rows {
		labelW = max(labelW, plotCharW*float64(len(name))+24)
	}
	rowH := 10.0*float64(len(groups)) + 8
	h := rowH*float64(len(rows)) + 8
	height := int(math.Ceil(plotPadT + h + plotPadB))
	plotOpen(&b, plotWidth, height, title)
	f := plotFrame{b: &b, x0: labelW, y0: plotPadT, w: plotWidth - labelW - plotPadR, h: h, xa: xa, ya: plotAxis{min: 0, max: 1, step: 1}}
	for _, v := range xa.ticks() {
		x := f.sx(v)
		b.WriteString(`<line class="gr" x1="` + plotPx(x) + `" y1="` + plotPx(f.y0+f.h) + `" x2="` + plotPx(x) + `" y2="` + plotPx(f.y0) + `"/>` + "\n")
		b.WriteString(`<text x="` + plotPx(x) + `" y="` + plotPx(f.y0+f.h+18) + `" text-anchor="middle">` + plotNum(v) + `</text>` + "\n")
	}
	for _, v := range xa.minorTicks() {
		x := f.sx(v)
		b.WriteString(`<line class="mi" x1="` + plotPx(x) + `" y1="` + plotPx(f.y0+f.h) + `" x2="` + plotPx(x) + `" y2="` + plotPx(f.y0) + `"/>` + "\n")
	}
	b.WriteString(`<rect class="ax" x="` + plotPx(f.x0) + `" y="` + plotPx(f.y0) + `" width="` + plotPx(f.w) + `" height="` + plotPx(f.h) + `"/>` + "\n")
	f.xLabel(xa.label)
	zero := f.sx(max(xa.min, min(xa.max, 0)))
	if xa.log {
		zero = f.sx(xa.min)
	}
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
		b.WriteString(`<line class="l r0" stroke-dasharray="6 4" x1="` + x + `" y1="` + plotPx(f.y0) + `" x2="` + x + `" y2="` + plotPx(f.y0+f.h) + `"/>` + "\n")
	}
	items := make([]legendItem, 0, len(groups))
	for _, grp := range groups {
		items = append(items, legendItem{name: grp.name, class: grp.class, box: true})
	}
	plotLegend(&b, f.x0, plotWidth-plotPadR, items)
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
