package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// floorPoint is the floor evaluation of every payload at one C/N, summed
// over seeds: the counts of the likelihood test and the information
// densities of the converse.
type floorPoint struct {
	CN     float64                `json:"cn"`
	Counts map[string]*benchScore `json:"counts"`
	Conv   map[string][]float64   `json:"conv"`
}

// fer returns the lower bound on the error rate of payload k at p: the
// larger of the likelihood test and the converse, and of the slot type
// for a data payload that needs it. ok is false when p has no items of k.
func (p *floorPoint) fer(k string) (float64, int, bool) {
	var v float64
	n := 0
	for _, key := range []string{k, k + "/st"} {
		if s := p.Counts[key]; s != nil && s.Truth > 0 {
			v = max(v, 1-float64(s.Exact)/float64(s.Truth))
			n = max(n, s.Truth)
		}
	}
	if u := p.Conv[k]; len(u) > 0 {
		v = max(v, convBound(u))
		n = max(n, len(u))
	}
	return v, n, n > 0
}

// floorRun computes the floor of the sweep stream at each case on workers
// goroutines, with a genie trellis of window d, and returns the points
// summed over seeds.
func floorRun(t testing.TB, slots []BSSlot, truth []benchSlot, cases []benchCase, d int, tail bool, only map[string]bool, workers int) []floorPoint {
	t.Helper()
	type result struct {
		sc   map[string]*benchScore
		conv map[string][]float64
	}
	res := make([]result, len(cases))
	var wg sync.WaitGroup
	next := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := newFloorEval(d)
			e.tail, e.only = tail, only
			e.slots, e.truth = slots, truth
			for i := range next {
				c := cases[i]
				e.f = newFloorSignal(slots, c.cn, c.seed)
				e.stream()
				res[i] = result{e.sc, e.conv}
			}
		}()
	}
	for i := range cases {
		next <- i
	}
	close(next)
	wg.Wait()
	byCN := map[float64]*floorPoint{}
	for i, c := range cases {
		p := byCN[c.cn]
		if p == nil {
			p = &floorPoint{CN: c.cn, Counts: map[string]*benchScore{}, Conv: map[string][]float64{}}
			byCN[c.cn] = p
		}
		for k, v := range res[i].sc {
			a := p.Counts[k]
			if a == nil {
				a = &benchScore{}
				p.Counts[k] = a
			}
			a.Truth += v.Truth
			a.Exact += v.Exact
		}
		for k, v := range res[i].conv {
			p.Conv[k] = append(p.Conv[k], v...)
		}
	}
	out := make([]floorPoint, 0, len(byCN))
	for _, p := range byCN {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b floorPoint) int { return int(math.Copysign(1, a.CN-b.CN)) })
	return out
}

// TestFloorProbe prints the floor of the payloads on a grid, for the
// settings in DMRRF_FLOOR_PROBE: window, tail (A or B), seeds, the C/N
// grid lo:hi:step and optionally the payloads joined by +.
func TestFloorProbe(t *testing.T) {
	t.Parallel()
	spec := os.Getenv("DMRRF_FLOOR_PROBE")
	if spec == "" {
		t.Skip("set DMRRF_FLOOR_PROBE=d,tail,seeds,cn1:cn2:step")
	}
	f := strings.Split(spec, ",")
	d, _ := strconv.Atoi(f[0])
	tail := f[1] == "A"
	ns, _ := strconv.Atoi(f[2])
	g := strings.Split(f[3], ":")
	lo, _ := strconv.ParseFloat(g[0], 64)
	hi, _ := strconv.ParseFloat(g[1], 64)
	st, _ := strconv.ParseFloat(g[2], 64)
	var only map[string]bool
	if len(f) > 4 {
		only = map[string]bool{}
		for _, k := range strings.Split(f[4], "+") {
			only[k] = true
		}
	}
	slots, truth := benchStream(t, 30)
	var cases []benchCase
	for cn := lo; cn <= hi+1e-9; cn += st {
		for s := range uint64(ns) {
			cases = append(cases, benchCase{cn, 100 + s})
		}
	}
	start := time.Now()
	pts := floorRun(t, slots, truth, cases, d, tail, only, runtime.NumCPU()*3/4)
	fmt.Printf("%d streams in %v\n", len(cases), time.Since(start))
	if out := os.Getenv("DMRRF_FLOOR_OUT"); out != "" {
		b, err := json.Marshal(pts)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range pts {
		var row []string
		for _, k := range floorPayloads() {
			s := p.Counts[k]
			if s == nil || s.Truth == 0 {
				continue
			}
			cell := fmt.Sprintf("%s %.4f", k, 1-float64(s.Exact)/float64(s.Truth))
			if u := p.Conv[k]; len(u) > 0 {
				cell += fmt.Sprintf("/%.4f", convBound(u))
			}
			row = append(row, cell)
		}
		fmt.Printf("cn %5.1f eb %5.2f: %s\n", p.CN, p.CN+10*math.Log10(12500.0/9600), strings.Join(row, ", "))
	}
}
