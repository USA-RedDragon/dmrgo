package main

import (
	"math"
	"strconv"
	"strings"
)

const (
	sheetW, sheetH = 700, 420
	plotPadL       = 76
	plotPadR       = 190
	plotPadT       = 40
	plotPadB       = 56
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
	b.WriteString("<style>" + chartStyle + "</style>\n")
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
