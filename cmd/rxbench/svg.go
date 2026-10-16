package main

import (
	"strconv"
	"strings"
)

// Layout of a chart, in pixels, and its axes.
const (
	chartW, chartH       = 640, 400
	padL, padR           = 60, 150
	padT, padB           = 40, 50
	xMin, xMax, xStep    = -2.0, 14.0, 2.0
	plotW, plotH         = chartW - padL - padR, chartH - padT - padB
	chartStyle           = `text{font-family:sans-serif;font-size:12px;fill:#57606a}.ax{stroke:#57606a;stroke-width:1;fill:none}.gr{stroke:#d0d7de;stroke-width:1}.s0{stroke:#6e7781}.s1{stroke:#cf222e}.s2{stroke:#0969da}.s3{stroke:#1a7f37}.l{fill:none;stroke-width:2}@media (prefers-color-scheme:dark){text{fill:#c9d1d9}.ax{stroke:#8b949e}.gr{stroke:#30363d}.s0{stroke:#8b949e}.s1{stroke:#ff7b72}.s2{stroke:#58a6ff}.s3{stroke:#3fb950}}`
	legendX, legendStepY = chartW - padR + 16, 22
)

// svgSeries is one line of a chart.
type svgSeries struct {
	name  string
	x, y  []float64
	dash  bool
	class string
}

func px(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func sx(x float64) float64 { return padL + (x-xMin)/(xMax-xMin)*plotW }

func sy(y float64) float64 { return padT + (1-y)*plotH }

// svgChart draws the series as frame recovery (0 to 1) against Eb/N0 in
// dB, with a transparent background, axis labels and a legend.
func svgChart(title string, series []svgSeries) []byte {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(chartW) + `" height="` + strconv.Itoa(chartH) +
		`" viewBox="0 0 ` + strconv.Itoa(chartW) + ` ` + strconv.Itoa(chartH) + `">` + "\n")
	b.WriteString("<style>" + chartStyle + "</style>\n")
	b.WriteString(`<text x="` + px(padL) + `" y="24">` + title + `: frame recovery</text>` + "\n")
	for x := xMin; x <= xMax+1e-9; x += xStep {
		b.WriteString(`<line class="gr" x1="` + px(sx(x)) + `" y1="` + px(sy(0)) + `" x2="` + px(sx(x)) + `" y2="` + px(sy(1)) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(sx(x)) + `" y="` + px(sy(0)+18) + `" text-anchor="middle">` + strconv.FormatFloat(x, 'f', 0, 64) + `</text>` + "\n")
	}
	for i := range 6 {
		y := float64(i) / 5
		b.WriteString(`<line class="gr" x1="` + px(sx(xMin)) + `" y1="` + px(sy(y)) + `" x2="` + px(sx(xMax)) + `" y2="` + px(sy(y)) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(sx(xMin)-8) + `" y="` + px(sy(y)+4) + `" text-anchor="end">` + strconv.Itoa(i*20) + `%</text>` + "\n")
	}
	b.WriteString(`<rect class="ax" x="` + px(sx(xMin)) + `" y="` + px(sy(1)) + `" width="` + px(plotW) + `" height="` + px(plotH) + `"/>` + "\n")
	b.WriteString(`<text x="` + px(sx((xMin+xMax)/2)) + `" y="` + px(chartH-10) + `" text-anchor="middle">Eb/N0 (dB)</text>` + "\n")
	for i, s := range series {
		var pts []string
		for k := range s.x {
			if s.x[k] < xMin || s.x[k] > xMax {
				continue
			}
			pts = append(pts, px(sx(s.x[k]))+","+px(sy(s.y[k])))
		}
		dash := ""
		if s.dash {
			dash = ` stroke-dasharray="6 4"`
		}
		b.WriteString(`<polyline class="l ` + s.class + `"` + dash + ` points="` + strings.Join(pts, " ") + `"/>` + "\n")
		ly := float64(padT + 10 + i*legendStepY)
		b.WriteString(`<line class="l ` + s.class + `"` + dash + ` x1="` + px(legendX) + `" y1="` + px(ly) + `" x2="` + px(legendX+24) + `" y2="` + px(ly) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(legendX+30) + `" y="` + px(ly+4) + `">` + s.name + `</text>` + "\n")
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
