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
	Mode         string  `json:"mode"`
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

// curve is a reference curve: the frames and the frames recovered exactly
// at each Eb/N0, and the kind of bound or receiver it is.
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

// The status of a DMR floor in testdata/rxbench_floor.json. Any other
// status marks a DMR floor that the generator cannot compute.
const (
	// statusExact marks a DMR floor that tests every value of the payload.
	statusExact = "exact"
	// statusLower marks a DMR floor that tests the values of a search, a
	// lower bound on it.
	statusLower = "lower bound"
)

// floorPair is the two floors of one payload: the 4FSK floor of any code
// of its size and the DMR floor of its DMR code, with the status of the
// DMR floor.
type floorPair struct {
	Any    curve  `json:"any"`
	DMR    curve  `json:"dmr"`
	Status string `json:"status"`
}

// floors is testdata/rxbench_floor.json.
type floors struct {
	Source   string               `json:"source"`
	Payloads map[string]floorPair `json:"payloads"`
}

// curves is testdata/rxbench_aware.json, the code-aware receiver.
type curves struct {
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

// The input modes of a tier: FM discriminator audio, or I/Q samples.
const (
	modeAudio = "audio"
	modeIQ    = "iq"
)

// modes lists the input modes in the order of the README: the mode, the
// title of its graphs, and its short name.
func modes() [][3]string {
	return [][3]string{{modeAudio, "FM discriminator audio", "Audio"}, {modeIQ, "I/Q", "I/Q"}}
}

// modeLabel returns the short README name of mode m.
func modeLabel(m string) string {
	for _, v := range modes() {
		if v[0] == m {
			return v[2]
		}
	}
	return m
}

// tierName returns the README name of tier t, with its input mode.
func tierName(t tier) string {
	desc := map[string]string{
		modeAudio + "0": "discriminator core, 9.6 kHz",
		modeIQ + "1":    "coherent, two pass",
		modeIQ + "2":    "phase trellis, two pass",
	}[t.Mode+strconv.Itoa(t.Tier)]
	name := modeLabel(t.Mode) + " " + strconv.Itoa(t.Tier)
	if desc == "" {
		return name
	}
	return name + ": " + desc
}

// shortName returns the name of tier t in the recordings table and the
// graphs.
func shortName(t tier) string {
	return modeLabel(t.Mode) + " tier " + strconv.Itoa(t.Tier)
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
// ramCell formats n bytes: in bytes below 10 KiB, in KiB above.
func ramCell(n int) string {
	if n < 10<<10 {
		return grouped(int64(n)) + " B"
	}
	return strconv.Itoa((n+1023)/1024) + " KiB"
}

// sig2 rounds v to two significant digits.
func sig2(v float64) int64 {
	if v < 1 {
		return 0
	}
	p := math.Pow(10, math.Floor(math.Log10(v))-1)
	return int64(math.Round(v/p) * p)
}

func grouped(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// f1 formats v with one decimal, with no sign on a value that rounds to 0.
func f1(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10+0, 'f', 1, 64) }

func renderBlock(d data) []byte {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("Loss is the Eb/N0 in dB above the DMR floor at 90% frame recovery. The DMR floor is the error rate of maximum likelihood decoding of the DMR code of the payload, for a receiver that a genie gives the timing, the carrier, the noise level and every transmitted bit outside the payload. Where the floor is only bounded, the loss is a range: its high end is against the lower bound of the DMR floor, and its low end is against the code-aware receiver, which has the same genie and the decoders of the tiers. The sensitivity details give the floors and the cost of the DMR codes.\n\n")
	b.WriteString("| Tier | Voice | Data rate 1/2 | CSBK | Data rate 3/4 | ns per second of signal | RAM | Allocs |\n")
	b.WriteString("|------|-------|---------------|------|---------------|-------------------------|-----|--------|\n")
	for _, t := range d.bench.Tiers {
		row := make([]string, 0, 8)
		row = append(row, tierName(t))
		for _, k := range []string{keyAMBE, keyRate12, keyCSBK, keyRate34} {
			row = append(row, lossCell(d, t, k))
		}
		row = append(row, grouped(sig2(t.NsPerSecond)),
			ramCell(t.RAMBytes), strconv.FormatFloat(t.Allocs, 'f', -1, 64))
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	b.WriteString("\nCost is the CPU time on " + d.bench.Machine + ".")
	b.WriteString(" The audio tiers take the audio of an FM discriminator at 9.6 kHz, which the sweep makes from the same noisy signal with the IF filter of a 12.5 kHz radio, so their Eb/N0 is that of the signal at the antenna. Audio tier 0 is rf.DiscReceiver. The I/Q tiers are rf.Receiver on I/Q samples at 48 kHz. RAM is what the receiver keeps with OnBurst set: for audio tier 0 the size of rf.DiscReceiver on a 64-bit host, its whole state, which rf.DiscReceiver.Init can also place in static memory (1,040 B on a Cortex-M4F); for the I/Q tiers the heap, to which the slice of bursts that Write returns without OnBurst adds 28 KiB. Tier 3, a five-symbol window with all 200 carrier phases, is for offline decoding and is not in these tables.\n\n")
	renderSensitivity(&b, d)
	b.WriteString("<details><summary>Real recordings</summary>\n\n")
	b.WriteString("Voice frames recovered exactly from captures of a Motorola XPR 8400 repeater with an RTL-SDR, by tier.")
	for _, r := range d.recordings {
		if slices.Min(r.Exact) < 0 {
			b.WriteString(" n/a marks a tier that has not run on the recording.")
			break
		}
	}
	b.WriteString("\n\n")
	b.WriteString("| Recording | Frames |")
	for _, t := range d.bench.Tiers {
		b.WriteString(" " + shortName(t) + " |")
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
	b.WriteString("Frame error rate per payload type, and the bit error rate of the uncoded rate 1 payload bits, against Eb/N0, for the 4FSK floor (dashed) and each tier. The graphs leave out the DMR floor: for the voice frame and the BPTC, rate 3/4 and embedded LC codes, which have 2^49 to 2^96 values, the phase memory of the 4FSK modulation keeps the searches from bounding it, and the sensitivity details give what is known. A rate 1 block that a tier misses counts all of its bits as wrong. Points with no errors are left out.\n\n")
	for _, m := range modes() {
		b.WriteString("#### " + m[1] + "\n\n")
		for _, g := range graphs() {
			b.WriteString("![" + g.title + ", " + m[1] + ": " + strings.ToLower(g.rate) + " against Eb/N0, references and tiers](docs/rx/" + m[0] + "/" + g.key + ".svg)\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n</details>\n\n")
	return []byte(b.String())
}

// renderSensitivity writes the sensitivity details: the Eb/N0 at 90%
// recovery of each payload for the floors, the code-aware receiver and
// each tier.
func renderSensitivity(b *strings.Builder, d data) {
	b.WriteString("<details><summary>Sensitivity per payload type</summary>\n\n")
	b.WriteString("Eb/N0 in dB for 90% recovery, C/N in 12.5 kHz in parentheses. Both floors are lower bounds on the error rate of any receiver that knows less than a genie, which gives it the timing, the carrier, the noise level, every transmitted bit outside the payload, and the side information that the tiers use: the color code of the stream for the EMB and the slot type, and the embedded LC of every earlier superframe of the call.\n\n")
	b.WriteString("- 4FSK floor (any code): the converse bound of Verdu and Han on the information density of the payload's symbols for its number of values. No code of that size on those symbols and no decoder can do better. For the rate 1 payload bits it is Fano's inequality on the same densities.\n")
	b.WriteString("- DMR floor (ETSI codes): maximum likelihood decoding of the DMR code of the payload. The payload is in error when a value is more likely than the transmitted one, by the exact likelihood of the whole stream. For the EMB, the slot type, the SYNC and the rate 1 payload the test covers all values. For the voice frame and the rate 3/4 block a search finds the values, so the DMR floor is a lower bound. For the BPTC payloads (rate 1/2, CSBK, LC and data header) and the embedded LC it is not computed: maximum likelihood over 2^72 to 2^96 values is out of reach, and the search finds no value more likely than the transmitted one where the 4FSK floor allows errors. The DMR floor of a data payload also counts its slot type.\n")
	b.WriteString("- Code cost: the DMR floor less the 4FSK floor, which the DMR code costs against the best code of its size. It includes the slack of the converse bound: every code of the size of the rate 1 payload is the same code, so its cost is that slack only.\n")
	b.WriteString("- Code-aware: a receiver with the knowledge of the genie that detects the bits of the payload exactly, bit by bit, then decodes them with the decoders of the coherent tiers and their passes of detection again: for a voice frame with the C0 and C1 codewords of the first decode as priors, and for a BPTC block with the extrinsic information of its grid decode. It is a receiver, not a bound, so the DMR floor is at or under it.\n\n")
	b.WriteString("Where the DMR floor is a lower bound, the table gives it as \"at least\", and the code cost as a range up to the code-aware receiver. The floors run on the stream and noise seeds of the tiers. Their generator is v2/rf/floor_gen_test.go, and testdata/rxbench_floor.json names the bound of each payload. The code-aware receiver is in v2/rf/floor_aware_test.go.")
	b.WriteString("\n\n| Payload | 4FSK floor | DMR floor | Code cost | Code-aware |")
	for _, t := range d.bench.Tiers {
		b.WriteString(" " + tierName(t) + " |")
	}
	b.WriteString("\n|---------|------------|-----------|-----------|------------|")
	for range d.bench.Tiers {
		b.WriteString("------|")
	}
	b.WriteString("\n")
	for _, p := range payloadNames() {
		f, ok := d.floor.Payloads[p[0]]
		cells := make([]string, 5, 5+len(d.bench.Tiers))
		cells[0] = p[1]
		for i := 1; i < 5; i++ {
			cells[i] = "-"
		}
		if ok {
			cells[1] = ebCell(at90(f.Any.EbN0, f.Any.recovery()))
			cells[2] = dmrCell(f)
			cells[3] = costCell(d, p[0])
		}
		if c, ok := d.aware.Payloads[p[0]]; ok {
			cells[4] = ebCell(at90(c.EbN0, c.recovery()))
		}
		for _, t := range d.bench.Tiers {
			cells = append(cells, ebCell(at90(tierCurve(t, p[0]))))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	b.WriteString("\n</details>\n\n")
}

// dmrCell returns the DMR floor cell of the sensitivity table.
func dmrCell(f floorPair) string {
	c := at90(f.DMR.EbN0, f.DMR.recovery())
	switch {
	case f.Status == statusExact:
		return ebCell(c)
	case f.Status == statusLower && c.ok && !c.below:
		return "at least " + ebCell(c)
	}
	return "-"
}

// span is a range of dB, from lo to hi, either end missing when not ok.
type span struct {
	lo, hi     float64
	loOK, hiOK bool
}

// text formats s with unit after each number: one value when the ends
// agree to 0.1, a range, or one end as "at least" or "at most".
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

// gap returns the Eb/N0 of crossing a less that of b, and whether both are
// found and inside their curves.
func gap(a, b crossing) (float64, bool) {
	return a.ebn0 - b.ebn0, a.ok && b.ok && !a.below && !b.below
}

// referenceCrossings returns the 90% crossings of payload k for the DMR
// floor, the 4FSK floor and the code-aware receiver, the status of the DMR
// floor, and false when k has no floors.
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

// lossCell returns the loss of payload k of tier t against the DMR floor:
// a range from the loss against the code-aware receiver to that against
// the lower bound when the DMR floor is not exact.
func lossCell(d data, t tier, k string) string {
	v := at90(tierCurve(t, k))
	dmr, _, aware, status, ok := referenceCrossings(d, k)
	if !ok || !v.ok || v.below {
		return "-"
	}
	if status == statusExact {
		if !dmr.ok {
			return "-"
		}
		if dmr.below {
			return "> " + f1(v.ebn0-dmr.ebn0) + " dB"
		}
		return f1(v.ebn0-dmr.ebn0) + " dB"
	}
	var s span
	s.hi, s.hiOK = gap(v, dmr)
	s.lo, s.loOK = gap(v, aware)
	return s.text(" dB")
}

// costCell returns the code cost of payload k: the DMR floor less the 4FSK
// floor, a range up to the code-aware receiver when the DMR floor is not
// exact.
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

// renderGraphs returns one SVG per graph and input mode, by its path under
// docs/rx: the error rate against Eb/N0 on a log scale, the 4FSK floor
// dashed and each tier of the mode solid.
func renderGraphs(d data) map[string][]byte {
	out := map[string][]byte{}
	for _, m := range modes() {
		for _, g := range graphs() {
			out[m[0]+"/"+g.key+".svg"] = renderGraph(d, g, m[0])
		}
	}
	return out
}

// renderGraph returns the SVG of graph g for the tiers of mode m.
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
