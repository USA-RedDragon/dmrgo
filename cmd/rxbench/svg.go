package main

import (
	"math"
	"strconv"
	"strings"
)

// Layout of a chart, in pixels, and its axes: Eb/N0 in dB on x and the
// error rate from 10^yMinExp to 1 on a log scale on y.
const (
	chartW, chartH       = 660, 420
	padL, padR           = 76, 150
	padT, padB           = 40, 56
	xMin, xMax, xStep    = -8.0, 14.0, 2.0
	yMinExp              = -4
	plotW, plotH         = chartW - padL - padR, chartH - padT - padB
	chartStyle           = `text{font-family:sans-serif;font-size:12px;fill:#57606a}.ax{stroke:#57606a;stroke-width:1;fill:none}.gr{stroke:#d0d7de;stroke-width:1}.mi{stroke:#eaeef2;stroke-width:1}.s0{stroke:#6e7781}.s1{stroke:#cf222e}.s2{stroke:#0969da}.s3{stroke:#1a7f37}.s4{stroke:#8250df}.l{fill:none;stroke-width:2}.p{fill:none;stroke-width:1.5}@media (prefers-color-scheme:dark){text{fill:#c9d1d9}.ax{stroke:#8b949e}.gr{stroke:#30363d}.mi{stroke:#21262d}.s0{stroke:#8b949e}.s1{stroke:#ff7b72}.s2{stroke:#58a6ff}.s3{stroke:#3fb950}.s4{stroke:#d2a8ff}}`
	legendX, legendStepY = chartW - padR + 16, 22
)

// svgSeries is one line of a chart. A y value of zero or under 10^yMinExp
// breaks the line. A series with a dash pattern is a reference curve, drawn
// without point marks.
type svgSeries struct {
	name  string
	x, y  []float64
	dash  string
	class string
}

func px(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func sx(x float64) float64 { return padL + (x-xMin)/(xMax-xMin)*plotW }

// sy returns the pixel row of the error rate y, which is above 0.
func sy(y float64) float64 { return padT + math.Log10(y)/yMinExp*plotH }

// decadeLabel returns the y axis label of 10^e.
func decadeLabel(e int) string {
	switch e {
	case 0:
		return "1"
	case -1:
		return "0.1"
	}
	return "1e" + strconv.Itoa(e)
}

// svgChart draws the series as an error rate on a log scale against Eb/N0
// in dB, with a transparent background, a decade grid, axis titles and a
// legend. yTitle names the error rate.
func svgChart(title, yTitle string, series []svgSeries) []byte {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + strconv.Itoa(chartW) + `" height="` + strconv.Itoa(chartH) +
		`" viewBox="0 0 ` + strconv.Itoa(chartW) + ` ` + strconv.Itoa(chartH) + `">` + "\n")
	b.WriteString("<style>" + chartStyle + "</style>\n")
	b.WriteString(`<text x="` + px(padL) + `" y="24">` + title + `: ` + strings.ToLower(yTitle) + `</text>` + "\n")
	bottom, top := padT+plotH, float64(padT)
	for e := 0; e > yMinExp; e-- {
		for m := 2; m < 10; m++ {
			y := sy(float64(m) * math.Pow(10, float64(e-1)))
			b.WriteString(`<line class="mi" x1="` + px(sx(xMin)) + `" y1="` + px(y) + `" x2="` + px(sx(xMax)) + `" y2="` + px(y) + `"/>` + "\n")
		}
	}
	for x := xMin; x <= xMax+1e-9; x += xStep {
		b.WriteString(`<line class="gr" x1="` + px(sx(x)) + `" y1="` + px(float64(bottom)) + `" x2="` + px(sx(x)) + `" y2="` + px(top) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(sx(x)) + `" y="` + px(float64(bottom)+18) + `" text-anchor="middle">` + strconv.FormatFloat(x, 'f', 0, 64) + `</text>` + "\n")
	}
	for e := 0; e >= yMinExp; e-- {
		y := sy(math.Pow(10, float64(e)))
		b.WriteString(`<line class="gr" x1="` + px(sx(xMin)) + `" y1="` + px(y) + `" x2="` + px(sx(xMax)) + `" y2="` + px(y) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(sx(xMin)-8) + `" y="` + px(y+4) + `" text-anchor="end">` + decadeLabel(e) + `</text>` + "\n")
	}
	b.WriteString(`<rect class="ax" x="` + px(sx(xMin)) + `" y="` + px(top) + `" width="` + px(plotW) + `" height="` + px(plotH) + `"/>` + "\n")
	b.WriteString(`<text x="` + px(sx((xMin+xMax)/2)) + `" y="` + px(chartH-12) + `" text-anchor="middle">Eb/N0 (dB)</text>` + "\n")
	cy := top + plotH/2
	b.WriteString(`<text x="18" y="` + px(cy) + `" text-anchor="middle" transform="rotate(-90 18 ` + px(cy) + `)">` + yTitle + `</text>` + "\n")
	low := math.Pow(10, yMinExp)
	for i, s := range series {
		var path strings.Builder
		var marks strings.Builder
		pen := false
		for k := range s.x {
			if s.x[k] < xMin || s.x[k] > xMax || !(s.y[k] >= low) || s.y[k] > 1 {
				pen = false
				continue
			}
			op := "L"
			if !pen {
				op = "M"
			}
			pen = true
			x, y := px(sx(s.x[k])), px(sy(s.y[k]))
			path.WriteString(op + x + "," + y + " ")
			marks.WriteString(`<circle class="p ` + s.class + `" cx="` + x + `" cy="` + y + `" r="2"/>`)
		}
		dash := ""
		if s.dash != "" {
			dash = ` stroke-dasharray="` + s.dash + `"`
		}
		if path.Len() > 0 {
			b.WriteString(`<path class="l ` + s.class + `"` + dash + ` d="` + strings.TrimSpace(path.String()) + `"/>` + "\n")
			if s.dash == "" {
				b.WriteString(marks.String() + "\n")
			}
		}
		ly := float64(padT + 10 + i*legendStepY)
		b.WriteString(`<line class="l ` + s.class + `"` + dash + ` x1="` + px(legendX) + `" y1="` + px(ly) + `" x2="` + px(legendX+24) + `" y2="` + px(ly) + `"/>` + "\n")
		b.WriteString(`<text x="` + px(legendX+30) + `" y="` + px(ly+4) + `">` + s.name + `</text>` + "\n")
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
