package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Keys of the payload types in the results.
const (
	keyAMBE   = "ambe"
	keyRate12 = "rate12"
	keyRate34 = "rate34"
	keyCSBK   = "csbk"
	keyELC    = "elc"
	keySync   = "sync"
)

// lossPayloads returns the payload types of the worst loss column and the
// default fine grid.
func lossPayloads() []string {
	return []string{keyAMBE, keyRate12, keyRate34, keyCSBK, keyELC, keySync}
}

// reportPayloads returns the payload types of the report: key and name.
func reportPayloads() [][2]string {
	return [][2]string{
		{keyAMBE, "Voice frame (AMBE+2)"},
		{keyRate12, "Data block, rate 1/2"},
		{keyRate34, "Data block, rate 3/4"},
		{"rate1", "Data block, rate 1"},
		{keyCSBK, "CSBK"},
		{"lc", "Voice LC header and terminator"},
		{"header", "Data header"},
		{keyELC, "Embedded LC"},
		{"emb", "EMB"},
		{"slottype", "Slot type"},
		{keySync, "SYNC detection"},
		{"rate1_bit", "Rate 1 payload bits"},
	}
}

// The targets of the smallest tier.
const (
	targetRAM = 1024
	targetMHz = 4
)

// floorCurve and floorPair are the curves of testdata/rxbench_floor.json.
type floorCurve struct {
	EbN0  []float64 `json:"ebn0"`
	Truth []int     `json:"truth"`
	Exact []int     `json:"exact"`
}

type floorPair struct {
	Any    floorCurve `json:"any"`
	DMR    floorCurve `json:"dmr"`
	Status string     `json:"status"`
}

func (c floorCurve) recovery() []float64 {
	y := make([]float64, len(c.EbN0))
	for i := range y {
		if i < len(c.Truth) && c.Truth[i] > 0 {
			y[i] = float64(c.Exact[i]) / float64(c.Truth[i])
		}
	}
	return y
}

// floorRef is the 90% point of the two floors of a payload, in Eb/N0.
type floorRef struct {
	any, dmr    crossing
	status      string
	anyCurve    floorCurve
	dmrCurve    floorCurve
	hasDMRCurve bool
	hasAnyCurve bool
}

func loadFloors(path string) (map[string]floorRef, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Payloads map[string]json.RawMessage `json:"payloads"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := map[string]floorRef{}
	for k, raw := range f.Payloads {
		var p floorPair
		if err := json.Unmarshal(raw, &p); err != nil || len(p.Any.EbN0) == 0 {
			continue
		}
		r := floorRef{status: p.Status, anyCurve: p.Any, dmrCurve: p.DMR, hasAnyCurve: true}
		r.any = at90(p.Any.EbN0, p.Any.recovery())
		if p.Status == "exact" || p.Status == "lower bound" {
			r.dmr = at90(p.DMR.EbN0, p.DMR.recovery())
			r.hasDMRCurve = true
		}
		out[k] = r
	}
	return out, nil
}

// row is the report of one variant.
type row struct {
	rec      variantRecord
	pts      []point
	ebn0     map[string]crossing
	cost     *costRecord
	m4       map[string]json.RawMessage
	cpuMS    float64 // ms of host CPU per second, locked at 10 dB
	searchMS float64
	ram      int
	// mhz and mhzSearch are the Cortex-M4F MHz, locked and on noise, and
	// flash the code bytes, from the -m4 command; 0 when none.
	mhz, mhzSearch float64
	flash          int
	worst          float64 // worst loss over the space payloads, dB
}

func report(out, floorPath string) error {
	rows, floors, err := loadRows(out, floorPath)
	if err != nil {
		return err
	}
	keys := spacePayloads(rows)
	for _, r := range rows {
		r.worst = math.Inf(-1)
		for _, k := range keys {
			if l, ok := loss(r, floors, k); ok {
				r.worst = max(r.worst, l)
			} else {
				r.worst = math.Inf(1)
			}
		}
	}
	dir := filepath.Join(out, "report")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(dir, "variants.csv"), rows, floors); err != nil {
		return err
	}
	md := summary(rows, floors, keys)
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(md), 0o600); err != nil {
		return err
	}
	for _, p := range reportPayloads() {
		charts := map[string]*chart{
			"cpu-": scatter(rows, floors, p, costCPU),
			"ram-": scatter(rows, floors, p, costRAM),
			"m4-":  scatter(rows, floors, p, costM4),
			"fer-": curves(rows, floors, p),
		}
		for prefix, c := range charts {
			if c == nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, prefix+p[0]+".svg"), c.svg(), 0o600); err != nil {
				return err
			}
		}
	}
	fmt.Print(md)
	return nil
}

// loadRows reads the results of the output directory out and the floors.
func loadRows(out, floorPath string) ([]*row, map[string]floorRef, error) {
	recs, err := readLines[variantRecord](filepath.Join(out, variantsFile))
	if err != nil {
		return nil, nil, err
	}
	cases, err := readLines[caseResult](filepath.Join(out, resultsFile))
	if err != nil {
		return nil, nil, err
	}
	costs, err := readLines[costRecord](filepath.Join(out, costFile))
	if err != nil {
		return nil, nil, err
	}
	m4s, err := readLines[m4Record](filepath.Join(out, m4File))
	if err != nil {
		return nil, nil, err
	}
	floors, err := loadFloors(floorPath)
	if err != nil {
		return nil, nil, fmt.Errorf("floors: %w", err)
	}
	byID := map[string][]caseResult{}
	for _, c := range cases {
		byID[c.ID] = append(byID[c.ID], c)
	}
	costBy := map[string]*costRecord{}
	for i := range costs {
		costBy[costs[i].ID] = &costs[i]
	}
	m4By := map[string]map[string]json.RawMessage{}
	for _, m := range m4s {
		m4By[m.ID] = m.Result
	}
	latest := map[string]variantRecord{}
	var order []string
	for _, r := range recs {
		if _, ok := latest[r.ID]; !ok {
			order = append(order, r.ID)
		}
		latest[r.ID] = r
	}
	var rows []*row
	for _, id := range order {
		cs := byID[id]
		if len(cs) == 0 {
			continue
		}
		r := &row{rec: latest[id], pts: curveOf(cs), ebn0: map[string]crossing{}, cost: costBy[id], m4: m4By[id]}
		for _, p := range reportPayloads() {
			cn, y := recovery(r.pts, p[0])
			c := at90(cn, y)
			c.X = cnToEbN0(c.X)
			r.ebn0[p[0]] = c
		}
		var cpu, sec float64
		for _, p := range r.pts {
			cpu += p.CPU
			sec += p.Sec
		}
		r.cpuMS = cpu / sec * 1e3
		if r.cost != nil {
			r.cpuMS = r.cost.LockedNsPerSecond["10"] / 1e6
			r.searchMS = r.cost.SearchNsPerSecond / 1e6
			r.ram = r.cost.RAMBytes
		}
		for k, p := range map[string]any{"mhz_locked": &r.mhz, "mhz_search": &r.mhzSearch, "text": &r.flash, "heap_onburst": &r.ram} {
			if v, ok := r.m4[k]; ok {
				if err := json.Unmarshal(v, p); err != nil {
					return nil, nil, fmt.Errorf("m4 %s of %s: %w", k, r.rec.Name, err)
				}
			}
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("no results in %s", out)
	}
	return rows, floors, nil
}

// spacePayloads returns the payloads of the worst-loss column: those of the
// report that every variant has a crossing for, from the space defaults.
func spacePayloads(rows []*row) []string {
	var out []string
	for _, k := range lossPayloads() {
		for _, r := range rows {
			if _, ok := r.ebn0[k]; ok {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// loss returns the Eb/N0 of payload k of r above its DMR floor, or above
// the 4FSK floor where the DMR floor is not computed.
func loss(r *row, floors map[string]floorRef, k string) (float64, bool) {
	c := r.ebn0[k]
	f, ok := floors[k]
	if !c.OK || c.Below || !ok {
		return 0, false
	}
	if f.dmr.OK {
		return c.X - f.dmr.X, true
	}
	if f.any.OK {
		return c.X - f.any.X, true
	}
	return 0, false
}

// costKind selects the cost axis of a scatter.
type costKind int

const (
	costCPU costKind = iota
	costRAM
	costM4
)

func (k costKind) value(r *row) float64 {
	switch k {
	case costRAM:
		return float64(r.ram)
	case costM4:
		return r.mhz
	case costCPU:
	}
	return r.cpuMS
}

func (k costKind) title() string {
	switch k {
	case costRAM:
		return "RAM, bytes (heap of New with OnBurst; on the M4F when measured)"
	case costM4:
		return "Cortex-M4F MHz, locked at 10 dB"
	case costCPU:
	}
	return "Host CPU, ms per second of signal, locked at 10 dB"
}

// front returns the indexes of the points of xs and ys that no other point
// beats in both, in increasing x.
func front(xs, ys []float64) []int {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int {
		if c := cmpFloat(xs[a], xs[b]); c != 0 {
			return c
		}
		return cmpFloat(ys[a], ys[b])
	})
	var out []int
	best := math.Inf(1)
	for _, i := range idx {
		if ys[i] < best {
			out = append(out, i)
			best = ys[i]
		}
	}
	return out
}

// scatter returns the chart of the 90% Eb/N0 of payload p of each variant
// against cost k, with the Pareto front, or nil when no variant has both.
func scatter(rows []*row, floors map[string]floorRef, p [2]string, k costKind) *chart {
	var xs, ys []float64
	var names []string
	for _, r := range rows {
		c := r.ebn0[p[0]]
		x := k.value(r)
		if !c.OK || c.Below || x <= 0 {
			continue
		}
		xs = append(xs, x)
		ys = append(ys, c.X)
		names = append(names, r.rec.Name)
	}
	if len(xs) == 0 {
		return nil
	}
	ch := &chart{title: p[1] + ": Eb/N0 at 90% recovery against cost", x: axis{title: k.title(), log: true}, y: axis{title: "Eb/N0 at 90% recovery, dB"}}
	lo, hi := slices.Min(xs), slices.Max(xs)
	ylo, yhi := slices.Min(ys), slices.Max(ys)
	f := floors[p[0]]
	if f.any.OK {
		ch.refs = append(ch.refs, refLine{name: "4FSK floor (any code)", at: f.any.X, dash: "6 4", color: "#555"})
		ylo = min(ylo, f.any.X)
	}
	if f.dmr.OK {
		name := "DMR floor (ETSI codes)"
		if f.status != "exact" {
			name = "DMR floor, lower bound"
		}
		ch.refs = append(ch.refs, refLine{name: name, at: f.dmr.X, dash: "10 3", color: "#111"})
		ylo = min(ylo, f.dmr.X)
	}
	switch k {
	case costRAM:
		ch.refs = append(ch.refs, refLine{name: "1 KiB target", at: targetRAM, vertical: true, dash: "3 3", color: "#c92a2a"})
		lo = min(lo, targetRAM)
	case costM4:
		ch.refs = append(ch.refs, refLine{name: "4 MHz target", at: targetMHz, vertical: true, dash: "3 3", color: "#c92a2a"})
		lo = min(lo, targetMHz)
	case costCPU:
	}
	ch.x.lo, ch.x.hi = lo/1.3, hi*1.3
	ch.y.lo, ch.y.hi = math.Floor(ylo-0.5), math.Ceil(yhi+0.5)
	fr := front(xs, ys)
	onFront := map[int]bool{}
	var fx, fy []float64
	var fl []string
	for _, i := range fr {
		onFront[i] = true
		fx, fy, fl = append(fx, xs[i]), append(fy, ys[i]), append(fl, names[i])
	}
	var ox, oy []float64
	for i := range xs {
		if !onFront[i] {
			ox, oy = append(ox, xs[i]), append(oy, ys[i])
		}
	}
	ch.series = append(ch.series,
		chartSeries{name: "variant", x: ox, y: oy, dots: true, color: "#a5b4c4"},
		chartSeries{name: "Pareto front", x: fx, y: fy, label: fl, dots: true, color: palette()[0]})
	var sx, sy []float64
	for i := range fx {
		if i > 0 {
			sx, sy = append(sx, fx[i]), append(sy, fy[i-1])
		}
		sx, sy = append(sx, fx[i]), append(sy, fy[i])
	}
	ch.series = append(ch.series, chartSeries{x: sx, y: sy, line: true, color: palette()[0], width: 2})
	return ch
}

// curves returns the chart of the frame error rate of payload p of the
// variants on the CPU Pareto front, with both floors.
func curves(rows []*row, floors map[string]floorRef, p [2]string) *chart {
	var xs, ys []float64
	var cand []*row
	for _, r := range rows {
		c := r.ebn0[p[0]]
		if !c.OK || c.Below || r.cpuMS <= 0 {
			continue
		}
		xs, ys = append(xs, r.cpuMS), append(ys, c.X)
		cand = append(cand, r)
	}
	if len(cand) == 0 {
		return nil
	}
	ch := &chart{title: p[1] + ": error rate of the CPU Pareto front", x: axis{title: "Eb/N0, dB"}, y: axis{title: "error rate", log: true, lo: 1e-4, hi: 1}}
	xlo, xhi := math.Inf(1), math.Inf(-1)
	add := func(name string, x, rec []float64, dash, color string) {
		var sx, sy []float64
		for i := range x {
			if e := 1 - rec[i]; e > 0 {
				sx, sy = append(sx, x[i]), append(sy, max(e, 1e-4))
				xlo, xhi = min(xlo, x[i]), max(xhi, x[i])
			}
		}
		if len(sx) > 0 {
			ch.series = append(ch.series, chartSeries{name: name, x: sx, y: sy, line: true, dash: dash, color: color})
		}
	}
	f := floors[p[0]]
	if f.hasAnyCurve {
		add("4FSK floor (any code)", f.anyCurve.EbN0, f.anyCurve.recovery(), "6 4", "#555")
	}
	if f.hasDMRCurve {
		add("DMR floor (ETSI codes)", f.dmrCurve.EbN0, f.dmrCurve.recovery(), "10 3", "#111")
	}
	for n, i := range front(xs, ys) {
		r := cand[i]
		cn, y := recovery(r.pts, p[0])
		x := make([]float64, len(cn))
		for j := range cn {
			x[j] = cnToEbN0(cn[j])
		}
		add(fmt.Sprintf("%s (%.1f ms/s)", r.rec.Name, r.cpuMS), x, y, "", palette()[n%len(palette())])
	}
	if math.IsInf(xlo, 1) {
		return nil
	}
	ch.x.lo, ch.x.hi = math.Floor(xlo), math.Ceil(xhi)
	return ch
}

// fmtCross returns the cell of crossing c.
func fmtCross(c crossing) string {
	switch {
	case !c.OK:
		return "-"
	case c.Below:
		return "<" + strconv.FormatFloat(c.X, 'f', 1, 64)
	}
	return strconv.FormatFloat(c.X, 'f', 2, 64)
}

func summary(rows []*row, floors map[string]floorRef, keys []string) string {
	var b strings.Builder
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, c *row) int { return cmpFloat(a.cpuMS, c.cpuMS) })
	xs := make([]float64, len(sorted))
	ys := make([]float64, len(sorted))
	for i, r := range sorted {
		xs[i], ys[i] = r.cpuMS, r.worst
	}
	onFront := map[int]bool{}
	for _, i := range front(xs, ys) {
		onFront[i] = true
	}
	fmt.Fprintf(&b, "# rxsweep report\n\nEb/N0 in dB at 90%% recovery. Loss is the Eb/N0 above the DMR floor (ETSI codes), or above the 4FSK floor (any code) where the DMR floor is not computed. Worst loss is the largest loss over %s. A * marks the Pareto front of worst loss against host CPU.\n\n", strings.Join(keys, ", "))
	b.WriteString("| | Variant | CPU ms/s | Search ms/s | RAM B | M4F MHz | M4F search MHz | Flash KiB | Worst loss |")
	for _, k := range keys {
		b.WriteString(" " + k + " |")
	}
	b.WriteString("\n|---|---|---|---|---|---|---|---|---|")
	for range keys {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	b.WriteString("| | 4FSK floor (any code) | | | | | | | |")
	for _, k := range keys {
		b.WriteString(" " + fmtCross(floors[k].any) + " |")
	}
	b.WriteString("\n| | DMR floor (ETSI codes) | | | | | | | |")
	for _, k := range keys {
		b.WriteString(" " + fmtCross(floors[k].dmr) + " |")
	}
	b.WriteString("\n")
	for i, r := range sorted {
		mark := ""
		if onFront[i] {
			mark = "*"
		}
		m4, m4s, flash := "-", "-", "-"
		if r.mhz > 0 {
			m4, m4s = strconv.FormatFloat(r.mhz, 'f', 1, 64), strconv.FormatFloat(r.mhzSearch, 'f', 1, 64)
		}
		if r.flash > 0 {
			flash = strconv.Itoa(r.flash / 1024)
		}
		ram := "-"
		if r.ram > 0 {
			ram = strconv.Itoa(r.ram)
		}
		fmt.Fprintf(&b, "| %s | %s | %.2f | %.2f | %s | %s | %s | %s | %s |", mark, r.rec.Name, r.cpuMS, r.searchMS, ram, m4, m4s, flash, lossCell(r.worst))
		for _, k := range keys {
			b.WriteString(" " + fmtCross(r.ebn0[k]) + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func lossCell(v float64) string {
	if math.IsInf(v, 0) {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func writeCSV(path string, rows []*row, floors map[string]floorRef) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	head := []string{"id", "name", "tree", "commit", "cpu_ms_per_s", "search_ms_per_s", "ram_bytes", "m4_mhz", "m4_search_mhz", "flash_bytes", "false_per_hour", "allocs", "worst_loss_db"}
	for _, p := range reportPayloads() {
		head = append(head, p[0]+"_ebn0", p[0]+"_loss")
	}
	_ = w.Write(head)
	for _, r := range rows {
		var fph, allocs string
		if r.cost != nil {
			fph, allocs = strconv.FormatFloat(r.cost.FalsePerHour, 'f', 2, 64), strconv.FormatFloat(r.cost.Allocs, 'f', 2, 64)
		}
		rec := []string{r.rec.ID, r.rec.Name, r.rec.Tree, r.rec.Commit, strconv.FormatFloat(r.cpuMS, 'f', 3, 64), strconv.FormatFloat(r.searchMS, 'f', 3, 64), strconv.Itoa(r.ram), strconv.FormatFloat(r.mhz, 'f', 2, 64), strconv.FormatFloat(r.mhzSearch, 'f', 2, 64), strconv.Itoa(r.flash), fph, allocs, lossCell(r.worst)}
		for _, p := range reportPayloads() {
			c := r.ebn0[p[0]]
			l, ok := loss(r, floors, p[0])
			ls := ""
			if ok {
				ls = strconv.FormatFloat(l, 'f', 3, 64)
			}
			rec = append(rec, fmtCross(c), ls)
		}
		_ = w.Write(rec)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
