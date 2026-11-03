//go:build !tinygo

package rf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func floorShards() map[string][]float64 {
	return map[string][]float64{
		"floor": {-8, -2.5, 0, 5, 7},
		"aware": {-8, 2, 3, 7.5},
	}
}

type floorDelta struct {
	Counts map[string]*benchScore `json:"counts"`
	Conv   []float64              `json:"conv,omitempty"`
}

type floorShardPoint struct {
	Point int                    `json:"point"`
	Base  floorPoint             `json:"base"`
	Deep  map[string]*floorDelta `json:"deep"`
}

type floorShardFile struct {
	Aware  bool              `json:"aware"`
	Points []floorShardPoint `json:"points"`
}

func floorCNs(cases []benchCase) []float64 {
	var cns []float64
	for _, c := range cases {
		if !slices.Contains(cns, c.cn) {
			cns = append(cns, c.cn)
		}
	}
	return cns
}

func cloneFloorPoint(p floorPoint) floorPoint {
	q := floorPoint{CN: p.CN, Counts: map[string]*benchScore{}, Conv: map[string][]float64{}}
	for k, v := range p.Counts {
		s := *v
		q.Counts[k] = &s
	}
	for k, v := range p.Conv {
		q.Conv[k] = slices.Clone(v)
	}
	return q
}

func floorDeepOnly(p *floorPoint, done map[string]bool) map[string]bool {
	only := map[string]bool{}
	for _, k := range floorDeep() {
		v, n, ok := p.dmrFER(k)
		if !ok || done[k] || v == 0 && n >= floorLimit(k)/4 || v > 0.2 || v*float64(n) >= 20 || n >= floorLimit(k) {
			continue
		}
		only[k] = true
	}
	return only
}

func floorMarkDone(p *floorPoint, done map[string]bool) {
	for _, k := range floorDeep() {
		if v, n, ok := p.dmrFER(k); ok && v == 0 && n >= floorLimit(k)/4 {
			done[k] = true
		}
	}
}

func floorShardRun(t *testing.T, aware bool, cases []benchCase, from, to, workers int) []floorShardPoint {
	slots, truth := benchStream(t, 30)
	cns := floorCNs(cases)[from:to]
	sub := slices.DeleteFunc(slices.Clone(cases), func(c benchCase) bool { return !slices.Contains(cns, c.cn) })
	pts := floorRun(t, slots, truth, sub, 1, true, nil, workers, aware, 0)
	out := make([]floorShardPoint, len(pts))
	done := map[string]bool{}
	for li := range pts {
		pi, p := from+li, &pts[li]
		sp := floorShardPoint{Point: pi, Base: cloneFloorPoint(*p), Deep: map[string]*floorDelta{}}
		delta := func(k string) *floorDelta {
			d := sp.Deep[k]
			if d == nil {
				d = &floorDelta{Counts: map[string]*benchScore{}}
				sp.Deep[k] = d
			}
			return d
		}
		for batch := 0; ; batch++ {
			only := floorDeepOnly(p, done)
			if len(only) == 0 {
				break
			}
			deep := make([]benchCase, workers)
			for j := range deep {
				deep[j] = benchCase{p.CN, 1000 + uint64(pi)<<20 + uint64(batch*workers+j)}
			}
			more := floorRun(t, slots, truth, deep, 1, true, only, workers, aware, 0)
			for k, v := range more[0].Counts {
				owner := k
				if !only[k] {
					owner = strings.TrimSuffix(k, "/st")
				}
				if !only[owner] {
					continue
				}
				a := p.Counts[k]
				a.Truth += v.Truth
				a.Exact += v.Exact
				d := delta(owner).Counts
				if d[k] == nil {
					d[k] = &benchScore{}
				}
				d[k].Truth += v.Truth
				d[k].Exact += v.Exact
			}
			for k, v := range more[0].Conv {
				if only[k] {
					p.Conv[k] = append(p.Conv[k], v...)
					delta(k).Conv = append(delta(k).Conv, v...)
				}
			}
		}
		floorMarkDone(p, done)
		out[li] = sp
		t.Logf("C/N %.1f dB done", p.CN)
	}
	return out
}

func floorMerge(files []floorShardFile, points int) ([]floorPoint, error) {
	byPoint := make([]*floorShardPoint, points)
	for _, f := range files {
		for i := range f.Points {
			sp := &f.Points[i]
			if sp.Point < 0 || sp.Point >= points || byPoint[sp.Point] != nil {
				return nil, fmt.Errorf("point %d is out of range or in two shards", sp.Point)
			}
			byPoint[sp.Point] = sp
		}
	}
	out := make([]floorPoint, points)
	done := map[string]bool{}
	for i, sp := range byPoint {
		if sp == nil {
			return nil, fmt.Errorf("no shard has point %d", i)
		}
		p := cloneFloorPoint(sp.Base)
		for _, k := range floorDeep() {
			d := sp.Deep[k]
			if done[k] || d == nil {
				continue
			}
			for key, v := range d.Counts {
				a := p.Counts[key]
				if a == nil {
					return nil, fmt.Errorf("point %d: the base has no %s", i, key)
				}
				a.Truth += v.Truth
				a.Exact += v.Exact
			}
			if len(d.Conv) > 0 {
				p.Conv[k] = append(p.Conv[k], d.Conv...)
			}
		}
		floorMarkDone(&p, done)
		out[i] = p
	}
	return out, nil
}

func floorShardRange(name string) (aware bool, from, to int, err error) {
	kind, k, _ := strings.Cut(name, "-")
	starts := floorShards()[kind]
	idx, err := strconv.Atoi(k)
	if len(starts) == 0 || err != nil || idx < 0 || idx >= len(starts) {
		return false, 0, 0, fmt.Errorf("%q is not a shard of floorShards()", name)
	}
	cns := floorCNs(floorCases())
	from = slices.Index(cns, starts[idx])
	to = len(cns)
	if idx+1 < len(starts) {
		to = slices.Index(cns, starts[idx+1])
	}
	if from < 0 || to < 0 || from >= to {
		return false, 0, 0, fmt.Errorf("%q has no C/N range", name)
	}
	return kind == "aware", from, to, nil
}

func TestRxFloorShard(t *testing.T) {
	t.Parallel()
	name, out := os.Getenv("DMRRF_FLOOR_SHARD"), os.Getenv("DMRRF_FLOOR_SHARD_OUT")
	if name == "" || out == "" {
		t.Skip("set DMRRF_FLOOR_SHARD to a shard such as floor-0 or aware-0 and DMRRF_FLOOR_SHARD_OUT to the file to write")
	}
	aware, from, to, err := floorShardRange(name)
	if err != nil {
		t.Fatal(err)
	}
	f := floorShardFile{Aware: aware, Points: floorShardRun(t, aware, floorCases(), from, to, floorWorkers())}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRxFloorMerge(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRRF_FLOOR_MERGE")
	if dir == "" {
		t.Skip("set DMRRF_FLOOR_MERGE to a directory of floor-<k>.json and aware-<k>.json shards to merge them into " + floorPath + " and " + awarePath)
	}
	points := len(floorCNs(floorCases()))
	for _, kind := range []string{"floor", "aware"} {
		var files []floorShardFile
		for k := range floorShards()[kind] {
			b, err := os.ReadFile(fmt.Sprintf("%s/%s-%d.json", dir, kind, k))
			if errors.Is(err, os.ErrNotExist) && k == 0 {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			var f floorShardFile
			if err := json.Unmarshal(b, &f); err != nil {
				t.Fatal(err)
			}
			if f.Aware != (kind == "aware") {
				t.Fatalf("%s-%d.json holds the other sweep", kind, k)
			}
			files = append(files, f)
		}
		if files == nil {
			continue
		}
		pts, err := floorMerge(files, points)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if kind == "aware" {
			writeAware(t, pts)
		} else {
			writeFloor(t, pts)
		}
	}
}

func TestRxFloorShards(t *testing.T) {
	t.Parallel()
	points := len(floorCNs(floorCases()))
	for kind, starts := range floorShards() {
		next := 0
		for k := range starts {
			aware, from, to, err := floorShardRange(kind + "-" + strconv.Itoa(k))
			if err != nil {
				t.Fatal(err)
			}
			if aware != (kind == "aware") || from != next {
				t.Fatalf("%s-%d starts at point %d, want %d", kind, k, from, next)
			}
			next = to
		}
		if next != points {
			t.Fatalf("the %s shards end at point %d of %d", kind, next, points)
		}
	}
}
