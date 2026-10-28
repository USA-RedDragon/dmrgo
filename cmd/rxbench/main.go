// Command rxbench writes the receiver sensitivity block of the README and
// its graphs from the sweep data of the receiver in v2/rf. With
// -update-readme it first runs the long sweep (DMRRF_RXBENCH=full go test
// -run TestRxBench in v2/rf), then rewrites the block between the rxbench
// markers and the SVG files. With -check-readme it renders the committed
// data and fails when the README block or a graph differs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	beginMarker = "<!-- rxbench:begin -->"
	endMarker   = "<!-- rxbench:end -->"
)

const (
	keyAMBE   = "ambe"
	keyRate12 = "rate12"
	keyRate34 = "rate34"
	keyCSBK   = "csbk"
)

func main() {
	update := flag.Bool("update-readme", false, "run the long sweep, then rewrite the README block and the graphs")
	check := flag.Bool("check-readme", false, "fail when the README block or a graph differs from the committed data")
	noSweep := flag.Bool("no-sweep", false, "with -update-readme, render the committed data without the sweep")
	flag.Parse()
	if *update == *check {
		fmt.Fprintln(os.Stderr, "rxbench: give -update-readme or -check-readme")
		os.Exit(2)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "rxbench: cannot find the rxbench source directory")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, filepath.Join(filepath.Dir(thisFile), "..", ".."), *update, *update && !*noSweep)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rxbench:", err)
		os.Exit(1)
	}
}

var errStale = errors.New("out of date: run go run -C cmd/rxbench . -update-readme")

func run(ctx context.Context, root string, update, sweep bool) error {
	if sweep {
		cmd := exec.CommandContext(ctx, "go", "test", "-C", filepath.Join(root, "v2"), "-run", "^TestRxBench$", "-timeout", "6h", "./rf")
		cmd.Env = append(os.Environ(), "DMRRF_RXBENCH=full")
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sweep: %w", err)
		}
	}
	d, err := load(filepath.Join(root, "v2/rf/testdata"))
	if err != nil {
		return err
	}
	block := renderBlock(d)
	graphs := renderGraphs(d)
	readmePath := filepath.Join(root, "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	i, j := bytes.Index(readme, []byte(beginMarker)), bytes.Index(readme, []byte(endMarker))
	if i < 0 || j < i {
		return errors.New("README.md has no rxbench markers")
	}
	next := append(append(append([]byte{}, readme[:i+len(beginMarker)]...), block...), readme[j:]...)
	stale := !bytes.Equal(next, readme)
	for name, svg := range graphs {
		p := filepath.Join(root, "docs/rx", name)
		old, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(old, svg) {
			stale = true
			if update {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, svg, 0o600); err != nil {
					return err
				}
			}
		}
	}
	if !update {
		if stale {
			return errStale
		}
		return nil
	}
	return os.WriteFile(readmePath, next, 0o600)
}

type score struct {
	Truth int `json:"truth"`
	Exact int `json:"exact"`
}

type point struct {
	CN       float64           `json:"cn"`
	Payloads map[string]*score `json:"payloads"`
}

type tier struct {
	Mode         string  `json:"mode"`
	Tier         int     `json:"tier"`
	Full         []point `json:"full"`
	NsPerSecond  float64 `json:"ns_per_second"`
	RAMBytes     int     `json:"ram_bytes"`
	Allocs       float64 `json:"allocs"`
	FalsePerHour float64 `json:"false_per_hour"`
}

type bench struct {
	Machine string `json:"machine"`
	Tiers   []tier `json:"tiers"`
}

type curve struct {
	EbN0  []float64 `json:"ebn0"`
	Truth []int     `json:"truth"`
	Exact []int     `json:"exact"`
	Bound string    `json:"bound"`
}

func (c curve) recovery() []float64 {
	y := make([]float64, len(c.EbN0))
	for i := range y {
		if i < len(c.Truth) && c.Truth[i] > 0 {
			y[i] = float64(c.Exact[i]) / float64(c.Truth[i])
		}
	}
	return y
}

const (
	statusExact = "exact"
	statusLower = "lower bound"
)

type floorPair struct {
	Any    curve  `json:"any"`
	DMR    curve  `json:"dmr"`
	Status string `json:"status"`
}

type floors struct {
	Source   string               `json:"source"`
	Payloads map[string]floorPair `json:"payloads"`
}

type curves struct {
	Source   string           `json:"source"`
	Payloads map[string]curve `json:"payloads"`
}

type recording struct {
	Name  string  `json:"name"`
	CN    float64 `json:"cn"`
	Truth int     `json:"truth"`
	Exact []int   `json:"exact"`
}

type data struct {
	bench      bench
	floor      floors
	aware      curves
	recordings []recording
}

func load(dir string) (data, error) {
	var d data
	for name, v := range map[string]any{"rxbench.json": &d.bench, "rxbench_floor.json": &d.floor, "rxbench_aware.json": &d.aware, "rxbench_recordings.json": &d.recordings} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return d, err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return d, fmt.Errorf("%s: %w", name, err)
		}
	}
	return d, nil
}

func cnToEbN0(cn float64) float64 { return cn + 10*math.Log10(12500.0/9600) }

func payloadNames() [][2]string {
	return [][2]string{
		{keyAMBE, "Voice frame (AMBE+2, 49 bits)"},
		{"ambe_c0c1", "Voice frame, protected bits (C0 and C1)"},
		{keyRate12, "Data block, rate 1/2"},
		{keyRate34, "Data block, rate 3/4"},
		{"rate1", "Data block, rate 1"},
		{keyCSBK, "CSBK"},
		{"lc", "Voice LC header and terminator"},
		{"header", "Data header"},
		{"elc", "Embedded LC"},
		{"emb", "EMB"},
		{"slottype", "Slot type"},
		{"sync", "SYNC detection"},
	}
}

const (
	mcuAudio = "2,008 B of RAM with its stack and up to 4.8 MHz, built with TinyGo -opt z,"
	mcuIQLow = "2,024 B and up to 4.5 MHz"
)

const (
	modeAudio = "audio"
	modeIQ    = "iq"
	modeIQLow = "iqlow"
)

func modes() [][3]string {
	return [][3]string{{modeAudio, "FM discriminator audio", "Audio"}, {modeIQLow, "I/Q at 9.6 kHz", "Low-rate I/Q"}, {modeIQ, "I/Q", "I/Q"}}
}

func modeLabel(m string) string {
	for _, v := range modes() {
		if v[0] == m {
			return v[2]
		}
	}
	return m
}

func shortName(t tier) string {
	return modeLabel(t.Mode) + " tier " + strconv.Itoa(t.Tier)
}

func inputName(t tier) string {
	return map[string]string{
		modeAudio + "0": "FM audio, 9.6 kHz, core",
		modeAudio + "1": "FM audio, 9.6 kHz, soft decoding, noise prediction",
		modeAudio + "2": "FM audio, 9.6 kHz, coherent detection",
		modeAudio + "3": "FM audio, 9.6 kHz, coherent detection, data passes",
		modeIQLow + "0": "I/Q, 9.6 kHz, discriminator core",
		modeIQ + "0":    "I/Q, discriminator core",
		modeIQ + "1":    "I/Q, soft decoding, noise prediction",
		modeIQ + "2":    "I/Q, coherent detection",
		modeIQ + "3":    "I/Q, phase trellis with Tier 2 beside it",
	}[t.Mode+strconv.Itoa(t.Tier)]
}

type crossing struct {
	ebn0      float64
	ok, below bool
}

func at90(x, y []float64) crossing {
	if len(y) > 0 && y[0] >= 0.9 {
		return crossing{x[0], true, true}
	}
	for i := 1; i < len(x); i++ {
		if y[i-1] < 0.9 && y[i] >= 0.9 {
			return crossing{x[i-1] + (0.9-y[i-1])/(y[i]-y[i-1])*(x[i]-x[i-1]), true, false}
		}
	}
	return crossing{}
}

func tierCurve(t tier, k string) (x, y []float64) {
	for _, p := range t.Full {
		if s := p.Payloads[k]; s != nil && s.Truth > 0 {
			x = append(x, cnToEbN0(p.CN))
			y = append(y, float64(s.Exact)/float64(s.Truth))
		}
	}
	return x, y
}

func ramCell(n int) string {
	switch {
	case n < 10<<10:
		return grouped(int64(n)) + " B"
	case n < 1<<20:
		return strconv.Itoa((n+1023)/1024) + " KiB"
	}
	return f1(float64(n)/(1<<20)) + " MiB"
}

func grouped(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func f1(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10+0, 'f', 1, 64) }

func renderBlock(d data) []byte {
	var b strings.Builder
	b.WriteString("\n| Tier | Input | Voice | Data rate 1/2 | CSBK | Data rate 3/4 | CPU per second of signal | RAM |\n")
	b.WriteString("|------|-------|-------|---------------|------|---------------|--------------------------|-----|\n")
	keys := []string{keyAMBE, keyRate12, keyCSBK, keyRate34}
	row := make([]string, 0, 2+len(keys)+2)
	row = append(row, "Best possible (4FSK floor)", "-")
	for _, k := range keys {
		cell := "-"
		if f, ok := d.floor.Payloads[k]; ok {
			cell = dbCell(at90(f.Any.EbN0, f.Any.recovery()))
		}
		row = append(row, cell)
	}
	b.WriteString("| " + strings.Join(append(row, "-", "-"), " | ") + " |\n")
	for _, t := range d.bench.Tiers {
		row := make([]string, 0, 2+len(keys)+2)
		row = append(row, shortName(t), inputName(t))
		for _, k := range keys {
			row = append(row, dbCell(at90(tierCurve(t, k))))
		}
		row = append(row, msCell(t.NsPerSecond), ramCell(t.RAMBytes))
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	b.WriteString("\n- Each cell is the Eb/N0 at which the tier recovers 90% of the frames. Lower is better. C/N in 12.5 kHz is 1.2 dB lower.\n")
	b.WriteString("- CPU is measured on an " + d.bench.Machine + ". RAM is the state of the receiver. On a Cortex-M4F, audio tier 0 needs " + mcuAudio + " and low-rate I/Q tier 0 " + mcuIQLow + ".")
	if !slices.ContainsFunc(d.bench.Tiers, func(t tier) bool { return t.Allocs > 0 }) {
		b.WriteString(" No tier allocates after it starts.")
	}
	b.WriteString("\n")
	b.WriteString("- Audio tiers take FM discriminator audio, as from a radio. I/Q tiers take samples at 48 kHz from a software radio, and low-rate I/Q tier 0 takes I/Q at 9.6 kHz that a radio chip filtered to the channel.\n")
	b.WriteString("- [docs/rx](docs/rx/README.md) tells how the sweep measures and what the floors are.\n\n")
	renderSensitivity(&b, d)
	var rec []tier
	var cols []int
	for i, t := range d.bench.Tiers {
		for _, r := range d.recordings {
			if i < len(r.Exact) && r.Exact[i] >= 0 {
				rec, cols = append(rec, t), append(cols, i)
				break
			}
		}
	}
	b.WriteString("<details><summary>Real recordings</summary>\n\n")
	b.WriteString("Voice frames recovered exactly from captures of a Motorola XPR 8400 repeater with an RTL-SDR.\n\n")
	b.WriteString("| Recording | C/N | Frames |")
	for _, t := range rec {
		b.WriteString(" " + shortName(t) + " |")
	}
	b.WriteString("\n|-----------|-----|--------|")
	for range rec {
		b.WriteString("------|")
	}
	b.WriteString("\n")
	for _, r := range d.recordings {
		b.WriteString("| " + r.Name + " | " + f1(r.CN) + " dB | " + strconv.Itoa(r.Truth) + " |")
		for _, i := range cols {
			b.WriteString(" " + strconv.Itoa(r.Exact[i]) + " |")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n</details>\n\n")
	b.WriteString("<details><summary>Error rate graphs</summary>\n\n")
	b.WriteString("Error rate against Eb/N0 for each tier, with the 4FSK floor dashed. Points with no errors are left out.\n\n")
	for _, m := range modes() {
		b.WriteString("#### " + m[1] + "\n\n")
		for _, g := range graphs() {
			b.WriteString("![" + g.title + ", " + m[1] + ": " + strings.ToLower(g.rate) + " against Eb/N0, references and tiers](docs/rx/" + m[0] + "/" + g.key + ".svg)\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("</details>\n\n")
	return []byte(b.String())
}

func renderSensitivity(b *strings.Builder, d data) {
	b.WriteString("<details><summary>Sensitivity per payload type</summary>\n\n")
	b.WriteString("Eb/N0 in dB for 90% recovery. [docs/rx](docs/rx/README.md) defines the floors.\n\n")
	b.WriteString("| Payload | 4FSK floor | DMR floor | Code cost | Code-aware |\n")
	b.WriteString("|---------|------------|-----------|-----------|------------|\n")
	for _, p := range payloadNames() {
		cells := []string{p[1], "-", "-", "-", "-"}
		if f, ok := d.floor.Payloads[p[0]]; ok {
			cells[1] = dbCell(at90(f.Any.EbN0, f.Any.recovery()))
			cells[2] = dmrCell(f)
			cells[3] = costCell(d, p[0])
		}
		if c, ok := d.aware.Payloads[p[0]]; ok {
			cells[4] = dbCell(at90(c.EbN0, c.recovery()))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	b.WriteString("\n| Payload |")
	for _, t := range d.bench.Tiers {
		b.WriteString(" " + shortName(t) + " |")
	}
	b.WriteString("\n|---------|")
	for range d.bench.Tiers {
		b.WriteString("------|")
	}
	b.WriteString("\n")
	for _, p := range payloadNames() {
		cells := make([]string, 0, 1+len(d.bench.Tiers))
		cells = append(cells, p[1])
		for _, t := range d.bench.Tiers {
			cells = append(cells, dbCell(at90(tierCurve(t, p[0]))))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	b.WriteString("\n</details>\n\n")
}

func dmrCell(f floorPair) string {
	c := at90(f.DMR.EbN0, f.DMR.recovery())
	switch {
	case f.Status == statusExact:
		return dbCell(c)
	case f.Status == statusLower && c.ok && !c.below:
		return "at least " + dbCell(c)
	}
	return "-"
}

type span struct {
	lo, hi     float64
	loOK, hiOK bool
}

func (s span) text(unit string) string {
	switch {
	case s.loOK && s.hiOK:
		lo := min(s.lo, s.hi)
		if f1(lo) == f1(s.hi) {
			return f1(s.hi) + unit
		}
		return f1(lo) + " to " + f1(s.hi) + unit
	case s.hiOK:
		return "at most " + f1(s.hi) + unit
	case s.loOK:
		return "at least " + f1(s.lo) + unit
	}
	return "-"
}

func gap(a, b crossing) (float64, bool) {
	return a.ebn0 - b.ebn0, a.ok && b.ok && !a.below && !b.below
}

func referenceCrossings(d data, k string) (dmr, anyCode, aware crossing, status string, ok bool) {
	f, ok := d.floor.Payloads[k]
	if !ok {
		return crossing{}, crossing{}, crossing{}, "", false
	}
	if c, has := d.aware.Payloads[k]; has {
		aware = at90(c.EbN0, c.recovery())
	}
	return at90(f.DMR.EbN0, f.DMR.recovery()), at90(f.Any.EbN0, f.Any.recovery()), aware, f.Status, true
}

func costCell(d data, k string) string {
	dmr, anyCode, aware, status, ok := referenceCrossings(d, k)
	if !ok {
		return "-"
	}
	var s span
	s.lo, s.loOK = gap(dmr, anyCode)
	s.lo = max(0, s.lo)
	if status == statusExact {
		s.hi, s.hiOK = s.lo, s.loOK
	} else {
		s.hi, s.hiOK = gap(aware, anyCode)
		if s.loOK && f1(s.lo) == f1(0) {
			s.loOK = false
		}
	}
	return s.text("")
}

func dbCell(c crossing) string {
	if !c.ok {
		return "-"
	}
	if c.below {
		return "< " + f1(c.ebn0) + " dB"
	}
	return f1(c.ebn0) + " dB"
}

func msCell(ns float64) string {
	ms := ns / 1e6
	switch {
	case ms >= 10:
		return strconv.FormatFloat(math.Round(ms), 'f', 0, 64) + " ms"
	case ms >= 1:
		return strconv.FormatFloat(math.Round(ms*10)/10, 'f', 1, 64) + " ms"
	}
	return strconv.FormatFloat(math.Round(ms*100)/100, 'f', 2, 64) + " ms"
}

type graph struct {
	key, title, rate string
}

func graphs() []graph {
	const fer = "Frame error rate"
	return []graph{
		{keyAMBE, "Voice frame", fer}, {keyRate12, "Data rate 1/2", fer}, {keyRate34, "Data rate 3/4", fer},
		{keyCSBK, "CSBK", fer}, {"elc", "Embedded LC", fer}, {"rate1_bit", "Data rate 1 payload bits", "Bit error rate"},
	}
}

func errorRate(y []float64) []float64 {
	out := make([]float64, len(y))
	for i, v := range y {
		out[i] = 1 - v
	}
	return out
}

func renderGraphs(d data) map[string][]byte {
	out := map[string][]byte{}
	for _, m := range modes() {
		for _, g := range graphs() {
			out[m[0]+"/"+g.key+".svg"] = renderGraph(d, g, m[0])
		}
	}
	return out
}

func renderGraph(d data, g graph, m string) []byte {
	var series []svgSeries
	if f, ok := d.floor.Payloads[g.key]; ok {
		series = append(series, svgSeries{name: "4FSK floor (any code)", x: f.Any.EbN0, y: errorRate(f.Any.recovery()), dash: "6 4", class: "r0"})
	}
	for _, t := range d.bench.Tiers {
		if t.Mode == m {
			x, y := tierCurve(t, g.key)
			series = append(series, svgSeries{name: "Tier " + strconv.Itoa(t.Tier), x: x, y: errorRate(y), class: "s" + strconv.Itoa(t.Tier+1)})
		}
	}
	return svgChart(g.title+", "+modeLabel(m), g.rate, series)
}
