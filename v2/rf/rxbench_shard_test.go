//go:build !tinygo

package rf

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func benchShards() map[string]int {
	return map[string]int{"audio3": 17, "iq3": 8}
}

type benchPiece struct {
	Point, Lo, Hi int
}

type benchShardPoint struct {
	Point   int          `json:"point"`
	Lo      int          `json:"lo"`
	Batches []benchPoint `json:"batches"`
}

type benchShardFile struct {
	Mode   string            `json:"mode"`
	Tier   int               `json:"tier"`
	Points []benchShardPoint `json:"points"`
}

func benchFullCNs() []float64 {
	var cns []float64
	for _, c := range fullCases() {
		if !slices.Contains(cns, c.cn) {
			cns = append(cns, c.cn)
		}
	}
	return cns
}

func benchBaseCases(cn float64) []benchCase {
	return slices.DeleteFunc(fullCases(), func(c benchCase) bool { return c.cn != cn })
}

func benchBaseSeeds() int { return len(benchBaseCases(fullCases()[0].cn)) }

func benchBatches(e benchEntry) int {
	return 1 + (deepStreams(e)-benchBaseSeeds()+deepBatch-1)/deepBatch
}

func benchBatchCases(point, b int, cn float64) []benchCase {
	if b == 0 {
		return benchBaseCases(cn)
	}
	seed := uint64(deepFirstSeed) + uint64(point)<<20 + uint64(b-1)*deepBatch
	cs := make([]benchCase, deepBatch)
	for i := range cs {
		cs[i] = benchCase{cn, seed + uint64(i)}
	}
	return cs
}

func benchClonePoint(p benchPoint) benchPoint {
	q := p
	q.Payloads = make(map[string]*benchScore, len(p.Payloads))
	for k, v := range p.Payloads {
		s := *v
		q.Payloads[k] = &s
	}
	return q
}

func benchAddPoint(p *benchPoint, d benchPoint) {
	p.Seeds += d.Seeds
	for k, v := range d.Payloads {
		a := p.Payloads[k]
		if a == nil {
			a = &benchScore{}
			p.Payloads[k] = a
		}
		a.Truth += v.Truth
		a.Exact += v.Exact
	}
}

func benchWantsBatch(e benchEntry, p *benchPoint, live func(int) bool, need func(*benchScore) bool) bool {
	if p.Seeds >= deepStreams(e) {
		return false
	}
	for i, k := range deepPayloads() {
		if live(i) && need(p.Payloads[k]) {
			return true
		}
	}
	return false
}

func benchStop(e benchEntry, live func(int) bool, batch func(int) (benchPoint, bool), need func(*benchScore) bool) (benchPoint, bool) {
	b0, ok := batch(0)
	if !ok {
		return benchPoint{}, false
	}
	p := benchClonePoint(b0)
	for b := 1; benchWantsBatch(e, &p, live, need); b++ {
		d, ok := batch(b)
		if !ok {
			return benchPoint{}, false
		}
		benchAddPoint(&p, d)
	}
	return p, true
}

func benchNext(st []uint8, p *benchPoint) []uint8 {
	next := slices.Clone(st)
	for i, k := range deepPayloads() {
		sc := p.Payloads[k]
		switch {
		case next[i] >= 2:
		case sc != nil && sc.Truth > 0 && float64(sc.Truth-sc.Exact) < deepFloor*float64(sc.Truth):
			next[i]++
		default:
			next[i] = 0
		}
	}
	return next
}

func benchStep(e benchEntry, st []uint8, batch func(int) (benchPoint, bool), need func(*benchScore) bool) (benchPoint, []uint8, bool) {
	p, ok := benchStop(e, func(i int) bool { return st[i] < 2 }, batch, need)
	if !ok {
		return benchPoint{}, nil, false
	}
	for i, k := range deepPayloads() {
		if st[i] >= 2 {
			delete(p.Payloads, k)
		}
	}
	return p, benchNext(st, &p), true
}

func benchAllStates() []string {
	out := []string{""}
	for range deepPayloads() {
		n := make([]string, 0, 3*len(out))
		for _, s := range out {
			n = append(n, s+"\x00", s+"\x01", s+"\x02")
		}
		out = n
	}
	return out
}

func benchShardRun(e benchEntry, cns []float64, pieces []benchPiece, run func([]benchCase) benchPoint) []benchShardPoint {
	return benchShardRunRule(e, cns, pieces, run, deepNeeds)
}

func benchShardRunRule(e benchEntry, cns []float64, pieces []benchPiece, run func([]benchCase) benchPoint, need func(*benchScore) bool) []benchShardPoint {
	zero := string(make([]byte, len(deepPayloads())))
	var states []string
	if len(pieces) > 0 && pieces[0].Point == 0 && pieces[0].Lo == 0 {
		states = []string{zero}
	}
	out := make([]benchShardPoint, 0, len(pieces))
	for _, pc := range pieces {
		cn := cns[pc.Point]
		sp := benchShardPoint{Point: pc.Point, Lo: pc.Lo}
		if pc.Lo > 0 {
			for b := pc.Lo; b < pc.Hi; b++ {
				sp.Batches = append(sp.Batches, run(benchBatchCases(pc.Point, b, cn)))
			}
			out, states = append(out, sp), nil
			continue
		}
		sts := states
		if sts == nil {
			sts = benchAllStates()
		}
		live := func(i int) bool {
			return slices.ContainsFunc(sts, func(s string) bool { return s[i] < 2 })
		}
		sp.Batches = append(sp.Batches, run(benchBatchCases(pc.Point, 0, cn)))
		p := benchClonePoint(sp.Batches[0])
		done := false
		for b := 1; ; b++ {
			if !benchWantsBatch(e, &p, live, need) {
				done = true
				break
			}
			if b >= pc.Hi {
				break
			}
			d := run(benchBatchCases(pc.Point, b, cn))
			sp.Batches = append(sp.Batches, d)
			benchAddPoint(&p, d)
		}
		out = append(out, sp)
		states = nil
		if !done {
			continue
		}
		batch := func(b int) (benchPoint, bool) {
			if b < len(sp.Batches) {
				return sp.Batches[b], true
			}
			return benchPoint{}, false
		}
		next := map[string]bool{}
		stops := map[string]benchPoint{}
		for _, s := range sts {
			mask := strings.Map(func(r rune) rune { return min(r, 2) / 2 }, s)
			q, seen := stops[mask]
			if !seen {
				var ok bool
				if q, ok = benchStop(e, func(i int) bool { return mask[i] == 0 }, batch, need); !ok {
					next = nil
					break
				}
				stops[mask] = q
			}
			next[string(benchNext([]uint8(s), &q))] = true
		}
		if next != nil {
			states = slices.Sorted(maps.Keys(next))
		}
	}
	return out
}

func benchMergeDeep(e benchEntry, points int, files []benchShardFile) ([]benchPoint, error) {
	return benchMergeDeepRule(e, points, files, deepNeeds)
}

func benchMergeDeepRule(e benchEntry, points int, files []benchShardFile, need func(*benchScore) bool) ([]benchPoint, error) {
	have := make([]map[int]benchPoint, points)
	for i := range have {
		have[i] = map[int]benchPoint{}
	}
	for _, f := range files {
		for _, sp := range f.Points {
			if sp.Point < 0 || sp.Point >= points {
				return nil, fmt.Errorf("%s: point %d is out of range", e.name(), sp.Point)
			}
			for j, d := range sp.Batches {
				if _, dup := have[sp.Point][sp.Lo+j]; dup {
					return nil, fmt.Errorf("%s: point %d batch %d is in two shards", e.name(), sp.Point, sp.Lo+j)
				}
				have[sp.Point][sp.Lo+j] = d
			}
		}
	}
	st := make([]uint8, len(deepPayloads()))
	out := make([]benchPoint, points)
	for i := range points {
		var miss int
		batch := func(b int) (benchPoint, bool) {
			d, ok := have[i][b]
			miss = b
			return d, ok
		}
		p, next, ok := benchStep(e, st, batch, need)
		if !ok {
			return nil, fmt.Errorf("%s: no shard has point %d batch %d", e.name(), i, miss)
		}
		out[i], st = p, next
	}
	return out, nil
}

func benchHints(e benchEntry, base benchData, points int) (batches []int, zero []bool) {
	B := benchBatches(e)
	batches, zero = make([]int, points), make([]bool, points)
	want, have := base.find(e)
	if !have || len(want.Full) != points {
		for i := range batches {
			batches[i] = B
		}
		return batches, zero
	}
	st := make([]uint8, len(deepPayloads()))
	for i, p := range want.Full {
		batches[i] = min(B, max(1, 1+(p.Seeds-benchBaseSeeds())/deepBatch))
		zero[i] = !slices.ContainsFunc(st, func(v uint8) bool { return v != 0 })
		for j, k := range deepPayloads() {
			sc := p.Payloads[k]
			switch {
			case st[j] >= 2:
			case sc != nil && sc.Truth > 0 && float64(sc.Truth-sc.Exact) < deepFloor*float64(sc.Truth):
				st[j]++
			default:
				st[j] = 0
			}
		}
	}
	return batches, zero
}

func benchPlan(e benchEntry, n int, base benchData, points int) [][]benchPiece {
	chunks, _ := benchPlanLimit(e, n, base, points)
	return chunks
}

func benchPlanLimit(e benchEntry, n int, base benchData, points int) ([][]benchPiece, int) {
	B := benchBatches(e)
	exp, zero := benchHints(e, base, points)
	baseGap := deepBatch - benchBaseSeeds()
	span := func(lo, hi int) int {
		c := (hi - lo) * deepBatch
		if lo == 0 && hi > 0 {
			c -= baseGap
		}
		return c
	}
	greedy := func(limit int) ([][]benchPiece, int) {
		var chunks [][]benchPiece
		var cur []benchPiece
		c, age, reset, most := 0, 0, false, 0
		closeChunk := func() {
			chunks, most = append(chunks, cur), max(most, c)
			cur, c, age, reset = nil, 0, 0, true
		}
		whole := func(i int) int {
			if reset && age < 2 && !zero[i] {
				return span(0, B)
			}
			return span(0, exp[i])
		}
		for i := range points {
			if exp[i] < B {
				w := whole(i)
				if len(cur) > 0 && c+w > limit {
					closeChunk()
					w = whole(i)
				}
				cur = append(cur, benchPiece{i, 0, B})
				c += w
				age++
				continue
			}
			for lo := 0; lo < B; {
				room := limit - c
				if lo == 0 {
					room += baseGap
				}
				hi := min(B, lo+max(0, room/deepBatch))
				if hi == lo && len(cur) == 0 {
					hi++
				}
				c += span(lo, hi)
				if hi == lo {
					closeChunk()
					continue
				}
				cur = append(cur, benchPiece{i, lo, hi})
				if lo == 0 && hi == B {
					age++
				} else {
					age, reset = 0, true
				}
				lo = hi
				if lo < B {
					closeChunk()
				}
			}
		}
		if len(cur) > 0 {
			closeChunk()
		}
		return chunks, most
	}
	total := span(0, B) * points
	chunks, best := greedy(total)
	for limit := deepBatch; limit < total; limit += max(deepBatch, total/1024) {
		if c, most := greedy(limit); len(c) <= n && most < best {
			chunks, best = c, most
		}
	}
	for len(chunks) < n {
		chunks = append(chunks, nil)
	}
	return chunks, best
}

func benchShardName(name string) (benchEntry, int, int, error) {
	tier, k, ok := strings.Cut(name, "-")
	n := benchShards()[tier]
	idx, err := strconv.Atoi(k)
	if !ok || n == 0 || err != nil || idx < 0 || idx >= n {
		return benchEntry{}, 0, 0, fmt.Errorf("%q is not a shard of benchShards()", name)
	}
	for _, e := range benchEntries() {
		if e.name() == tier {
			return e, idx, n, nil
		}
	}
	return benchEntry{}, 0, 0, fmt.Errorf("%q names no tier", name)
}

func readBenchBase() (benchData, error) {
	var base benchData
	err := readBench(rxbenchPath, &base)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return base, err
}

func TestRxBenchShard(t *testing.T) {
	t.Parallel()
	name, out := os.Getenv("DMRRF_RXBENCH_SHARD"), os.Getenv("DMRRF_RXBENCH_SHARD_OUT")
	if name == "" || out == "" {
		t.Skip("set DMRRF_RXBENCH_SHARD to a shard such as audio3-0 and DMRRF_RXBENCH_SHARD_OUT to the file to write")
	}
	e, k, n, err := benchShardName(name)
	if err != nil {
		t.Fatal(err)
	}
	base, err := readBenchBase()
	if err != nil {
		t.Fatal(err)
	}
	cns := benchFullCNs()
	pieces := benchPlan(e, n, base, len(cns))[k]
	fs, ft := benchStream(t, 30)
	sh := benchShared{t, newBenchPool(runtime.GOMAXPROCS(0)), cleanBaseband(fs, 1400), ft, []benchEntry{e}}
	run := func(cs []benchCase) benchPoint { return sh.run([][]benchCase{cs})[0][0] }
	f := benchShardFile{Mode: e.mode, Tier: e.number(), Points: benchShardRun(e, cns, pieces, run)}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func benchMergeShards(dir string, e benchEntry) ([]benchPoint, bool, error) {
	n := benchShards()[e.name()]
	if n == 0 {
		return nil, false, nil
	}
	var files []benchShardFile
	for k := range n {
		var f benchShardFile
		b, err := os.ReadFile(fmt.Sprintf("%s/%s-%d.json", dir, e.name(), k))
		if errors.Is(err, os.ErrNotExist) && k == 0 {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, false, err
		}
		if f.Mode != e.mode || f.Tier != e.number() {
			return nil, false, fmt.Errorf("%s-%d.json holds %s%d", e.name(), k, f.Mode, f.Tier)
		}
		files = append(files, f)
	}
	full, err := benchMergeDeep(e, len(benchFullCNs()), files)
	return full, true, err
}
