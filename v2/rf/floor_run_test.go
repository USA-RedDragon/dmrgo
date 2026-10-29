package rf

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

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

type floorPoint struct {
	CN     float64                `json:"cn"`
	Counts map[string]*benchScore `json:"counts"`
	Conv   map[string][]float64   `json:"conv"`
}

func (p *floorPoint) anyFER(k string) (float64, int, bool) {
	u := p.Conv[k]
	if len(u) == 0 {
		return 0, 0, false
	}
	if k == "rate1_bit" {
		return fanoBound(u, 192), 192 * len(u), true
	}
	return convBound(u), len(u), true
}

func (p *floorPoint) dmrFER(k string) (float64, int, bool) {
	var v float64
	n := 0
	for _, key := range []string{k, k + "/st"} {
		if s := p.Counts[key]; s != nil && s.Truth > 0 {
			v = max(v, 1-float64(s.Exact)/float64(s.Truth))
			n = max(n, s.Truth)
		}
	}
	if a, m, ok := p.anyFER(k); ok {
		v = max(v, a)
		n = max(n, m)
	}
	return v, n, n > 0
}

func floorRun(t testing.TB, slots []rftx.BSSlot, truth []benchSlot, cases []benchCase, d int, tail bool, only map[string]bool, workers int, aware bool, audioHz int) []floorPoint {
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
			e.tail, e.only, e.aware = tail, only, aware
			e.truth = truth
			for i := range next {
				c := cases[i]
				e.f = newFloorSignal(slots, c.cn, c.seed)
				if audioHz != 0 {
					e.f.useAudio(slots, audioHz)
				}
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

func TestFloorProbe(t *testing.T) {
	t.Parallel()
	spec := os.Getenv("DMRRF_FLOOR_PROBE")
	if spec == "" {
		t.Skip("set DMRRF_FLOOR_PROBE=d,tail,seeds,cn1:cn2:step")
		return
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
	audioHz, _ := strconv.Atoi(os.Getenv("DMRRF_FLOOR_AUDIO"))
	pts := floorRun(t, slots, truth, cases, d, tail, only, runtime.NumCPU()*3/4, os.Getenv("DMRRF_FLOOR_AWARE") != "", audioHz)
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
			if v, _, ok := p.anyFER(k); ok {
				cell += fmt.Sprintf("/%.4f", v)
			}
			row = append(row, cell)
		}
		fmt.Printf("cn %5.1f eb %5.2f: %s\n", p.CN, p.CN+10*math.Log10(12500.0/9600), strings.Join(row, ", "))
	}
}
