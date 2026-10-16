package main

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
)

// Chart geometry, in pixels.
const (
	chartW = 900
	chartH = 560
	padL   = 70
	padR   = 230
	padT   = 40
	padB   = 56
)

// palette returns the series colors.
func palette() []string {
	return []string{"#1f6fb2", "#c2541b", "#2b8a3e", "#8e44ad", "#b8860b", "#d6336c", "#0b7285", "#5c5f66"}
}

// axis is a linear or log10 chart axis.
type axis struct {
	title    string
	lo, hi   float64
	log      bool
	from, to float64
}

func (a axis) pos(v float64) float64 {
	if a.log {
		v, a.lo, a.hi = math.Log10(v), math.Log10(a.lo), math.Log10(a.hi)
	}
	return a.from + (v-a.lo)/(a.hi-a.lo)*(a.to-a.from)
}

// ticks returns the tick values of a.
func (a axis) ticks() []float64 {
	var out []float64
	if a.log {
		for e := math.Floor(math.Log10(a.lo)); e <= math.Ceil(math.Log10(a.hi)); e++ {
			for _, m := range []float64{1, 2, 5} {
				if v := m * math.Pow(10, e); v >= a.lo*0.999 && v <= a.hi*1.001 {
					out = append(out, v)
				}
			}
		}
		return out
	}
	step := niceStep((a.hi - a.lo) / 8)
	for v := math.Ceil(a.lo/step) * step; v <= a.hi+step*1e-6; v += step {
		out = append(out, math.Round(v/step)*step)
	}
	return out
}

func niceStep(raw float64) float64 {
	e := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*e >= raw {
			return m * e
		}
	}
	return 10 * e
}

func tickLabel(v float64, log bool) string {
	if log && (v >= 1e4 || v < 1e-2) {
		e := math.Floor(math.Log10(v) + 1e-9)
		m := v / math.Pow(10, e)
		if math.Abs(m-1) < 1e-9 {
			return "1e" + strconv.Itoa(int(e))
		}
		return strconv.FormatFloat(m, 'g', 2, 64) + "e" + strconv.Itoa(int(e))
	}
	return strconv.FormatFloat(v, 'g', 4, 64)
}

// chartSeries is a set of marks of one chart.
type chartSeries struct {
	name  string
	x, y  []float64
	label []string
	// line joins the points; dots marks them.
	line, dots bool
	dash       string
	color      string
	width      float64
}

// refLine is a horizontal (or, with vertical, vertical) reference line.
type refLine struct {
	name     string
	at       float64
	vertical bool
	dash     string
	color    string
}

// chart is one SVG chart.
type chart struct {
	title  string
	x, y   axis
	series []chartSeries
	refs   []refLine
	note   string
}

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func (c *chart) svg() []byte {
	c.x.from, c.x.to = padL, chartW-padR
	c.y.from, c.y.to = chartH-padB, padT
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" font-family="sans-serif" font-size="12">`+"\n", chartW, chartH)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#ffffff"/>`+"\n", chartW, chartH)
	fmt.Fprintf(&b, `<text x="%d" y="22" font-size="15" font-weight="bold" fill="#222">%s</text>`+"\n", padL, html.EscapeString(c.title))
	for _, v := range c.x.ticks() {
		x := c.x.pos(v)
		fmt.Fprintf(&b, `<line x1="%s" y1="%d" x2="%s" y2="%d" stroke="#e3e3e3"/>`, f1(x), padT, f1(x), chartH-padB)
		fmt.Fprintf(&b, `<text x="%s" y="%d" text-anchor="middle" fill="#444">%s</text>`+"\n", f1(x), chartH-padB+16, tickLabel(v, c.x.log))
	}
	for _, v := range c.y.ticks() {
		y := c.y.pos(v)
		fmt.Fprintf(&b, `<line x1="%d" y1="%s" x2="%d" y2="%s" stroke="#e3e3e3"/>`, padL, f1(y), chartW-padR, f1(y))
		fmt.Fprintf(&b, `<text x="%d" y="%s" text-anchor="end" fill="#444">%s</text>`+"\n", padL-6, f1(y+4), tickLabel(v, c.y.log))
	}
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#888"/>`+"\n", padL, padT, chartW-padR-padL, chartH-padB-padT)
	fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#222">%s</text>`+"\n", (padL+chartW-padR)/2, chartH-14, html.EscapeString(c.x.title))
	fmt.Fprintf(&b, `<text transform="translate(18 %d) rotate(-90)" text-anchor="middle" fill="#222">%s</text>`+"\n", (padT+chartH-padB)/2, html.EscapeString(c.y.title))
	fmt.Fprintf(&b, `<clipPath id="plot"><rect x="%d" y="%d" width="%d" height="%d"/></clipPath><g clip-path="url(#plot)">`+"\n", padL, padT, chartW-padR-padL, chartH-padB-padT)
	for _, r := range c.refs {
		if r.vertical {
			x := c.x.pos(r.at)
			fmt.Fprintf(&b, `<line x1="%s" y1="%d" x2="%s" y2="%d" stroke="%s" stroke-width="1.5" stroke-dasharray="%s"/>`+"\n", f1(x), padT, f1(x), chartH-padB, r.color, r.dash)
		} else {
			y := c.y.pos(r.at)
			fmt.Fprintf(&b, `<line x1="%d" y1="%s" x2="%d" y2="%s" stroke="%s" stroke-width="1.5" stroke-dasharray="%s"/>`+"\n", padL, f1(y), chartW-padR, f1(y), r.color, r.dash)
		}
	}
	for _, s := range c.series {
		if s.line && len(s.x) > 1 {
			pts := make([]string, 0, len(s.x))
			for i := range s.x {
				pts = append(pts, f1(c.x.pos(s.x[i]))+","+f1(c.y.pos(s.y[i])))
			}
			fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%s" stroke-dasharray="%s"/>`+"\n", strings.Join(pts, " "), s.color, f1(max(s.width, 1.5)), s.dash)
		}
		if s.dots {
			for i := range s.x {
				fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="3.5" fill="%s"/>`, f1(c.x.pos(s.x[i])), f1(c.y.pos(s.y[i])), s.color)
				if i < len(s.label) && s.label[i] != "" {
					fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="10" fill="#333">%s</text>`, f1(c.x.pos(s.x[i])+5), f1(c.y.pos(s.y[i])-5), html.EscapeString(s.label[i]))
				}
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("</g>\n")
	ly := padT + 8
	legend := func(name, color, dash string, dot bool) {
		lx := chartW - padR + 14
		if dot {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="3.5" fill="%s"/>`, lx+12, ly, color)
		} else {
			fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="2" stroke-dasharray="%s"/>`, lx, ly, lx+24, ly, color, dash)
		}
		fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#222">%s</text>`+"\n", lx+30, ly+4, html.EscapeString(name))
		ly += 18
	}
	for _, r := range c.refs {
		legend(r.name, r.color, r.dash, false)
	}
	for _, s := range c.series {
		if s.name != "" {
			legend(s.name, s.color, s.dash, s.dots && !s.line)
		}
	}
	if c.note != "" {
		for i, line := range strings.Split(c.note, "\n") {
			fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="10" fill="#555">%s</text>`+"\n", chartW-padR+14, ly+12+14*i, html.EscapeString(line))
		}
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
