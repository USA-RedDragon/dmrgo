//go:build !tinygo

package rf

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"
)

func fakeBenchScore(c benchCase, salt uint64) map[string]*benchScore {
	sc := map[string]*benchScore{}
	for i, k := range append(deepPayloads(), "sync") {
		h := splitmix(math.Float64bits(c.cn) ^ c.seed<<8 ^ uint64(i)<<40 ^ salt)
		truth := 20 + int(h%7)*40
		rate := math.Pow(10, -(c.cn-float64(i%5))/1.5)
		if i == 3 && c.cn > 9 && c.cn < 10.5 {
			rate = 0.02
		}
		u := float64(splitmix(h)>>11) / (1 << 53)
		bad := min(truth, int(float64(truth)*min(rate, 1)*2*u))
		sc[k] = &benchScore{Truth: truth, Exact: truth - bad}
	}
	return sc
}

func splitmix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ x>>30) * 0xbf58476d1ce4e5b9
	x = (x ^ x>>27) * 0x94d049bb133111eb
	return x ^ x>>31
}

func fakeBenchRun(salt uint64, streams *int) func([]benchCase) benchPoint {
	return func(cs []benchCase) benchPoint {
		*streams += len(cs)
		res := make([]map[string]*benchScore, len(cs))
		for i, c := range cs {
			res[i] = fakeBenchScore(c, salt)
		}
		return benchPoints(cs, res)[0]
	}
}

func fakeBenchDeepen(e benchEntry, cns []float64, salt uint64) ([]benchPoint, int) {
	streams := 0
	one := fakeBenchRun(salt, &streams)
	run := func(cases [][]benchCase) [][]benchPoint {
		out := make([][]benchPoint, len(cases))
		for j, cs := range cases {
			if cs != nil {
				out[j] = []benchPoint{one(cs)}
			}
		}
		return out
	}
	pts := make([]benchPoint, len(cns))
	for i, cn := range cns {
		pts[i] = one(benchBaseCases(cn))
	}
	all := [][]benchPoint{pts}
	benchShared{es: []benchEntry{e}}.deepenWith(all, run)
	return all[0], streams
}

func benchCoverage(t *testing.T, e benchEntry, plan [][]benchPiece, points int) {
	t.Helper()
	B := benchBatches(e)
	seen := make([][]int, points)
	for i := range seen {
		seen[i] = make([]int, B)
	}
	for _, chunk := range plan {
		for _, pc := range chunk {
			if pc.Lo < 0 || pc.Hi > B || pc.Lo >= pc.Hi {
				t.Fatalf("%s: bad piece %+v", e.name(), pc)
			}
			for b := pc.Lo; b < pc.Hi; b++ {
				seen[pc.Point][b]++
			}
		}
	}
	for i := range seen {
		for b, n := range seen[i] {
			if n != 1 {
				t.Fatalf("%s: point %d batch %d is in %d shards", e.name(), i, b, n)
			}
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRxBenchShardPlan(t *testing.T) {
	t.Parallel()
	base, err := readBenchBase()
	if err != nil {
		t.Fatal(err)
	}
	cns := benchFullCNs()
	for name, n := range benchShards() {
		e, _, _, err := benchShardName(name + "-0")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range []benchData{base, {}} {
			plan := benchPlan(e, n, b, len(cns))
			if len(plan) != n {
				t.Fatalf("%s: %d shards, want %d", name, len(plan), n)
			}
			benchCoverage(t, e, plan, len(cns))
		}
	}
}

func TestRxBenchShardMerge(t *testing.T) {
	t.Parallel()
	cns := benchFullCNs()
	split := 0
	defer func() {
		if split == 0 {
			t.Error("no plan splits a C/N point")
		}
	}()
	for _, e := range []benchEntry{{benchAudio, Tier3}, {benchIQ, Tier3}} {
		for salt := range uint64(2) {
			want, refStreams := fakeBenchDeepen(e, cns, salt)
			if !slices.ContainsFunc(want, func(p benchPoint) bool { return p.Payloads["csbk"] == nil }) {
				t.Fatalf("%s salt %d: no payload of the fake sweep dies", e.name(), salt)
			}
			wantJSON := mustJSON(t, want)
			hint := benchData{Tiers: []benchTier{{Mode: e.mode, Tier: e.number(), Full: want}}}
			for _, base := range []benchData{hint, {}} {
				for _, n := range []int{1, 4, 13} {
					plan := benchPlan(e, n, base, len(cns))
					benchCoverage(t, e, plan, len(cns))
					for _, chunk := range plan {
						split += len(slices.DeleteFunc(slices.Clone(chunk), func(pc benchPiece) bool { return pc.Lo == 0 }))
					}
					streams := 0
					run := fakeBenchRun(salt, &streams)
					files := make([]benchShardFile, 0, len(plan))
					for _, chunk := range plan {
						b, err := json.Marshal(benchShardFile{Mode: e.mode, Tier: e.number(), Points: benchShardRun(e, cns, chunk, run)})
						if err != nil {
							t.Fatal(err)
						}
						var f benchShardFile
						if err := json.Unmarshal(b, &f); err != nil {
							t.Fatal(err)
						}
						files = append(files, f)
					}
					got, err := benchMergeDeep(e, len(cns), files)
					if err != nil {
						t.Fatalf("%s salt %d, %d shards: %v", e.name(), salt, n, err)
					}
					if mustJSON(t, got) != wantJSON {
						t.Fatalf("%s salt %d, %d shards: the merge differs from deepen", e.name(), salt, n)
					}
					if n == 1 && streams != refStreams {
						t.Errorf("%s salt %d: one shard ran %d streams, deepen %d", e.name(), salt, streams, refStreams)
					}
				}
			}
		}
	}
}

func TestRxBenchShardMissing(t *testing.T) {
	t.Parallel()
	e := benchEntry{benchIQ, Tier3}
	cns := benchFullCNs()
	streams := 0
	run := fakeBenchRun(0, &streams)
	plan := benchPlan(e, 3, benchData{}, len(cns))
	files := []benchShardFile{{Points: benchShardRun(e, cns, plan[0], run)}, {Points: benchShardRun(e, cns, plan[2], run)}}
	if _, err := benchMergeDeep(e, len(cns), files); err == nil {
		t.Fatal("a merge without shard 1 succeeded")
	}
	if _, err := benchMergeDeep(e, len(cns), append(files, files[0])); err == nil || !strings.Contains(err.Error(), "two shards") {
		t.Fatalf("a merge with shard 0 twice: %v", err)
	}
}
