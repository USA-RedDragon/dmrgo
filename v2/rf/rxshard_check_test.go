//go:build !tinygo

package rf

import (
	"encoding/json"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRxShardCheck(t *testing.T) {
	which := os.Getenv("DMRRF_SHARDCHECK")
	if which == "" {
		t.Skip("set DMRRF_SHARDCHECK to rxbench, floor or aware to check that the merge of the shards equals the unsharded sweep on a subset of cells")
	}
	for _, w := range strings.Split(which, ",") {
		switch w {
		case "rxbench":
			checkBenchShards(t)
		case "floor", "aware":
			checkFloorShards(t, w == "aware")
		default:
			t.Fatalf("unknown check %q", w)
		}
	}
}

func checkBenchShards(t *testing.T) {
	slots, truth := benchStream(t, 30)
	sig := cleanBaseband(slots, 1400)
	pool := newBenchPool(runtime.GOMAXPROCS(0), benchLevels)
	for _, c := range []struct {
		e   benchEntry
		cns []float64
	}{
		{benchEntry{benchIQ, Tier0}, []float64{8.5, 9, 9.5, 10}},
	} {
		sh := benchShared{t, pool, sig, truth, []benchEntry{c.e}}
		ref := make([][]benchCase, 1)
		for _, cn := range c.cns {
			ref[0] = append(ref[0], benchBaseCases(cn)...)
		}
		pts := sh.run(ref)
		sh.deepen(pts)
		want, _ := json.Marshal(pts[0])
		refStreams := 0
		for _, p := range pts[0] {
			refStreams += p.Seeds
		}
		hint := benchData{Tiers: []benchTier{{Mode: c.e.mode, Tier: c.e.number(), Full: pts[0]}}}
		for _, base := range []benchData{{}, hint} {
			for _, n := range []int{3} {
				plan := benchPlan(c.e, n, base, len(c.cns))
				streams := 0
				run := func(cs []benchCase) benchPoint {
					streams += len(cs)
					return sh.run([][]benchCase{cs})[0][0]
				}
				var files []benchShardFile
				for _, chunk := range plan {
					b, err := json.Marshal(benchShardFile{Mode: c.e.mode, Tier: c.e.number(), Points: benchShardRun(c.e, c.cns, chunk, run)})
					if err != nil {
						t.Fatal(err)
					}
					var f benchShardFile
					if err := json.Unmarshal(b, &f); err != nil {
						t.Fatal(err)
					}
					files = append(files, f)
				}
				got, err := benchMergeDeep(c.e, len(c.cns), files)
				if err != nil {
					t.Fatal(err)
				}
				gotJSON, _ := json.Marshal(got)
				split := 0
				for _, chunk := range plan {
					split += len(slices.DeleteFunc(slices.Clone(chunk), func(pc benchPiece) bool { return pc.Lo == 0 }))
				}
				t.Logf("%s, %d shards, hint %v: %d split cells, %d streams, deepen %d, equal %v", c.e.name(), n, len(base.Tiers) > 0, split, streams, refStreams, string(gotJSON) == string(want))
				if string(gotJSON) != string(want) {
					t.Errorf("%s, %d shards: the merge differs from deepen", c.e.name(), n)
				}
			}
		}
	}
}

func checkFloorShards(t *testing.T, aware bool) {
	const workers = 2
	var cases []benchCase
	for _, c := range floorCases() {
		if c.cn == 7 || c.cn == 7.5 {
			cases = append(cases, c)
		}
	}
	want, _ := json.Marshal(genPointsAt(t, aware, cases, workers))
	var files []floorShardFile
	for from := range 2 {
		b, err := json.Marshal(floorShardFile{Aware: aware, Points: floorShardRun(t, aware, cases, from, from+1, workers)})
		if err != nil {
			t.Fatal(err)
		}
		var f floorShardFile
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		for _, p := range f.Points {
			t.Logf("aware %v point %d: deep payloads in the shard %v", aware, p.Point, slices.Sorted(func(yield func(string) bool) {
				for k := range p.Deep {
					if !yield(k) {
						return
					}
				}
			}))
		}
		files = append(files, f)
	}
	pts, err := floorMerge(files, 2)
	if err != nil {
		t.Fatal(err)
	}
	done := map[string]bool{}
	floorMarkDone(&pts[0], done)
	t.Logf("aware %v: payloads done before point 1 in the unsharded sweep %v", aware, done)
	got, _ := json.Marshal(pts)
	t.Logf("aware %v: merge equal %v (%d bytes)", aware, string(got) == string(want), len(want))
	if string(got) != string(want) {
		t.Errorf("aware %v: the floor merge differs from genPoints", aware)
	}
}
