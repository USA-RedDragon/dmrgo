// Command rxbench writes the receiver sensitivity block of the README and
// its graphs from the sweep data of dmrrf. With -update-readme it first runs
// the long sweep (DMRRF_RXBENCH=full go test -run TestRxBench in
// v2/cmd/dmrrf), then rewrites the block between the rxbench markers and the
// SVG files. With -check-readme it renders the committed data and fails when
// the README block or a graph differs.
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

// Keys of the payload types in the sweep data.
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

// errStale reports a README block or graph that differs from the data.
var errStale = errors.New("out of date: run go run -C cmd/rxbench . -update-readme")

func run(ctx context.Context, root string, update, sweep bool) error {
	if sweep {
		cmd := exec.CommandContext(ctx, "go", "test", "-C", filepath.Join(root, "v2"), "-run", "^TestRxBench$", "-timeout", "6h", "./cmd/dmrrf")
		cmd.Env = append(os.Environ(), "DMRRF_RXBENCH=full")
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sweep: %w", err)
		}
	}
	d, err := load(filepath.Join(root, "v2/cmd/dmrrf/testdata"))
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

// score counts the frames of one payload type at one C/N.
type score struct {
	Truth int `json:"truth"`
	Exact int `json:"exact"`
}

// point is the score of every payload type at one C/N.
type point struct {
	CN       float64           `json:"cn"`
	Payloads map[string]*score `json:"payloads"`
}

// tier is the sweep of one receiver tier.
type tier struct {
	Tier         int     `json:"tier"`
	Full         []point `json:"full"`
	NsPerSecond  float64 `json:"ns_per_second"`
	RAMBytes     int     `json:"ram_bytes"`
	Allocs       float64 `json:"allocs"`
	FalsePerHour float64 `json:"false_per_hour"`
}

// bench is testdata/rxbench.json.
type bench struct {
	Machine string `json:"machine"`
	Tiers   []tier `json:"tiers"`
}

// curve is a floor curve: the frames and the frames recovered exactly at
// each Eb/N0, and the kind of bound it is.
type curve struct {
	EbN0  []float64 `json:"ebn0"`
	Truth []int     `json:"truth"`
	Exact []int     `json:"exact"`
	Bound string    `json:"bound"`
}

// recovery returns the frame recovery at each Eb/N0 of c.
func (c curve) recovery() []float64 {
	y := make([]float64, len(c.EbN0))
	for i := range y {
		if i < len(c.Truth) && c.Truth[i] > 0 {
			y[i] = float64(c.Exact[i]) / float64(c.Truth[i])
		}
	}
	return y
}

// floor is testdata/rxbench_floor.json.
type floor struct {
	Source   string           `json:"source"`
	Payloads map[string]curve `json:"payloads"`
}

// recording is one real capture of testdata/rxbench_recordings.json.
type recording struct {
	Name  string  `json:"name"`
	CN    float64 `json:"cn"`
	Truth int     `json:"truth"`
	Exact []int   `json:"exact"`
}

// data is everything a block renders.
type data struct {
	bench      bench
	floor      floor
	recordings []recording
}

func load(dir string) (data, error) {
	var d data
	for name, v := range map[string]any{"rxbench.json": &d.bench, "rxbench_floor.json": &d.floor, "rxbench_recordings.json": &d.recordings} {
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

// cnToEbN0 converts C/N in 12.5 kHz to Eb/N0 per channel bit at 9600 bit/s.
func cnToEbN0(cn float64) float64 { return cn + 10*math.Log10(12500.0/9600) }

// payloadNames lists the rows of the sensitivity table: data key and name.
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

// tierName returns the README name of tier i.
func tierName(i int) string {
	return [...]string{"0: discriminator", "1: coherent, two pass", "2: phase trellis, two pass"}[i]
}

// crossing is the Eb/N0 where a curve first reaches 90% recovery. below
// marks a curve that is already there at its first point, Eb/N0.
type crossing struct {
	ebn0      float64
	ok, below bool
}

// at90 returns the Eb/N0 where the curve first reaches 90% recovery, by
// linear interpolation.
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

// tierCurve returns the Eb/N0 and recovery of payload k of tier t.
func tierCurve(t tier, k string) (x, y []float64) {
	for _, p := range t.Full {
		if s := p.Payloads[k]; s != nil && s.Truth > 0 {
			x = append(x, cnToEbN0(p.CN))
			y = append(y, float64(s.Exact)/float64(s.Truth))
		}
	}
	return x, y
}

// grouped returns n with a comma between each group of three digits.
func grouped(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func renderBlock(d data) []byte {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("Loss is the Eb/N0 in dB above the floor at 90% frame recovery. The floor is a lower bound for any receiver: a genie gives it the timing, the carrier, the noise level and every transmitted bit outside the payload (see the sensitivity details). For the data payloads it is the bound for any code with as many values over the same symbols, so their loss includes what a stronger code than the DMR one would gain, which no receiver can.\n\n")
	b.WriteString("| Tier | Voice | Data rate 1/2 | CSBK | Data rate 3/4 | ns per second of signal | RAM | Allocs |\n")
	b.WriteString("|------|-------|---------------|------|---------------|-------------------------|-----|--------|\n")
	for _, t := range d.bench.Tiers {
		row := make([]string, 0, 8)
		row = append(row, tierName(t.Tier))
		for _, k := range []string{keyAMBE, keyRate12, keyCSBK, keyRate34} {
			row = append(row, lossCell(d, t, k))
		}
		row = append(row, grouped(int64(math.Round(t.NsPerSecond/1e6)*1e6)),
			strconv.Itoa((t.RAMBytes+1023)/1024)+" KiB", strconv.FormatFloat(t.Allocs, 'f', -1, 64))
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	b.WriteString("\nCost is the CPU time on " + d.bench.Machine + ".")
	b.WriteString(" RAM is the heap that NewReceiver keeps for input at 48 kHz with ReceiverConfig.OnBurst set; without it the slice of bursts that Write returns adds 28 KiB. Tier 3, a five-symbol window with all 200 carrier phases, is for offline decoding and is not in these tables.\n\n")
	b.WriteString("<details><summary>Sensitivity per payload type</summary>\n\n")
	b.WriteString("Eb/N0 in dB for 90% recovery, C/N in 12.5 kHz in parentheses. The floor of a payload is a lower bound on the error rate of any receiver that knows less than a genie, which gives it the timing, the carrier, the noise level and every transmitted bit outside the payload, and the side information that the tiers use: the color code of the stream for the EMB and the slot type, and the embedded LC of every earlier superframe of the call. Its error rate is the larger of two bounds. In the first, the payload is in error when a candidate value is more likely than the transmitted one, by the exact likelihood of the whole stream, so maximum likelihood decoding would pick it; for the EMB, the slot type and the SYNC the candidates are all values, so this is the error rate of the best receiver. The second is the converse bound of Verdu and Han on the information density of the payload's symbols for its number of values, which no code of that size and no decoder can beat. Both take the error rate as the same for every message. The floor runs on the stream and noise seeds of the tiers; its generator is v2/cmd/dmrrf/floor_gen_test.go and testdata/rxbench_floor.json names the bound of each payload.")
	b.WriteString("\n\n| Payload | Floor |")
	for _, t := range d.bench.Tiers {
		b.WriteString(" Tier " + tierName(t.Tier) + " |")
	}
	b.WriteString("\n|---------|-------|")
	for range d.bench.Tiers {
		b.WriteString("------|")
	}
	b.WriteString("\n")
	for _, p := range payloadNames() {
		b.WriteString("| " + p[1] + " | ")
		if c, ok := d.floor.Payloads[p[0]]; ok {
			b.WriteString(ebCell(at90(c.EbN0, c.recovery())))
		} else {
			b.WriteString("-")
		}
		for _, t := range d.bench.Tiers {
			b.WriteString(" | " + ebCell(at90(tierCurve(t, p[0]))))
		}
		b.WriteString(" |\n")
	}
	b.WriteString("\n</details>\n\n")
	b.WriteString("<details><summary>Real recordings</summary>\n\n")
	b.WriteString("Voice frames recovered exactly from captures of a Motorola XPR 8400 repeater with an RTL-SDR, by tier.")
	for _, r := range d.recordings {
		if slices.Min(r.Exact) < 0 {
			b.WriteString(" n/a marks a long recording that the slowest tier did not run.")
			break
		}
	}
	b.WriteString("\n\n")
	b.WriteString("| Recording | Frames |")
	for _, t := range d.bench.Tiers {
		b.WriteString(" Tier " + strconv.Itoa(t.Tier) + " |")
	}
	b.WriteString("\n|-----------|--------|")
	for range d.bench.Tiers {
		b.WriteString("--------|")
	}
	b.WriteString("\n")
	for _, r := range d.recordings {
		b.WriteString("| " + r.Name + " (" + f1(r.CN) + " dB C/N) | " + strconv.Itoa(r.Truth) + " |")
		for _, n := range r.Exact {
			if n < 0 {
				b.WriteString(" n/a |")
				continue
			}
			b.WriteString(" " + strconv.Itoa(n) + " |")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n</details>\n\n")
	b.WriteString("<details><summary>Error rate graphs</summary>\n\n")
	b.WriteString("Frame error rate per payload type, and the bit error rate of the uncoded rate 1 payload bits, against Eb/N0, for the floor and each tier. A rate 1 block that a tier misses counts all of its bits as wrong. Points with no errors are left out.\n\n")
	for _, g := range graphs() {
		b.WriteString("![" + g.title + ": " + strings.ToLower(g.rate) + " against Eb/N0, floor and tiers](docs/rx/" + g.key + ".svg)\n")
	}
	b.WriteString("\n</details>\n\n")
	return []byte(b.String())
}

// lossCell returns the loss of payload k of tier t against the floor.
func lossCell(d data, t tier, k string) string {
	c, ok := d.floor.Payloads[k]
	if !ok {
		return "-"
	}
	f := at90(c.EbN0, c.recovery())
	v := at90(tierCurve(t, k))
	if !f.ok || !v.ok || v.below {
		return "-"
	}
	if f.below {
		return "> " + f1(v.ebn0-f.ebn0) + " dB"
	}
	return f1(v.ebn0-f.ebn0) + " dB"
}

// ebCell formats an Eb/N0 at 90% with its C/N, as an upper limit for a
// curve that starts above 90%.
func ebCell(c crossing) string {
	if !c.ok {
		return "-"
	}
	cn := f1(c.ebn0 - 10*math.Log10(12500.0/9600))
	if c.below {
		return "< " + f1(c.ebn0) + " (< " + cn + ")"
	}
	return f1(c.ebn0) + " (" + cn + ")"
}

// graph is a README graph: data key, title and error rate name.
type graph struct {
	key, title, rate string
}

// graphs lists the README graphs.
func graphs() []graph {
	const fer = "Frame error rate"
	return []graph{
		{keyAMBE, "Voice frame", fer}, {keyRate12, "Data rate 1/2", fer}, {keyRate34, "Data rate 3/4", fer},
		{keyCSBK, "CSBK", fer}, {"elc", "Embedded LC", fer}, {"rate1_bit", "Data rate 1 payload bits", "Bit error rate"},
	}
}

// errorRate returns 1 minus each recovery of y.
func errorRate(y []float64) []float64 {
	out := make([]float64, len(y))
	for i, v := range y {
		out[i] = 1 - v
	}
	return out
}

// renderGraphs returns one SVG per graph: the error rate against Eb/N0 on
// a log scale, the floor dashed and each tier solid.
func renderGraphs(d data) map[string][]byte {
	out := map[string][]byte{}
	for _, g := range graphs() {
		var series []svgSeries
		if c, ok := d.floor.Payloads[g.key]; ok {
			series = append(series, svgSeries{name: "Floor", x: c.EbN0, y: errorRate(c.recovery()), dash: true, class: "s0"})
		}
		for _, t := range d.bench.Tiers {
			x, y := tierCurve(t, g.key)
			series = append(series, svgSeries{name: "Tier " + strconv.Itoa(t.Tier), x: x, y: errorRate(y), class: "s" + strconv.Itoa(t.Tier+1)})
		}
		out[g.key+".svg"] = svgChart(g.title, g.rate, series)
	}
	return out
}
