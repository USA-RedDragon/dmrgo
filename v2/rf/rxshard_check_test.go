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
	t.Parallel()
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
	pool := newBenchPool(runtime.GOMAXPROCS(0))
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
		want := mustJSON(t, pts[0])
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
				files := make([]benchShardFile, 0, len(plan))
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
				gotJSON := mustJSON(t, got)
				split := 0
				for _, chunk := range plan {
					split += len(slices.DeleteFunc(slices.Clone(chunk), func(pc benchPiece) bool { return pc.Lo == 0 }))
				}
				t.Logf("%s, %d shards, hint %v: %d split cells, %d streams, deepen %d, equal %v", c.e.name(), n, len(base.Tiers) > 0, split, streams, refStreams, gotJSON == want)
				if gotJSON != want {
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
	want := mustJSON(t, genPointsAt(t, aware, cases, workers))
	files := make([]floorShardFile, 0, 2)
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
	got := mustJSON(t, pts)
	t.Logf("aware %v: merge equal %v (%d bytes)", aware, got == want, len(want))
	if got != want {
		t.Errorf("aware %v: the floor merge differs from genPoints", aware)
	}
}

func TestRxBenchRuleCheck(t *testing.T) {
	t.Parallel()
	which, out := os.Getenv("DMRRF_RULECHECK"), os.Getenv("DMRRF_RULECHECK_OUT")
	if which == "" || out == "" {
		t.Skip("set DMRRF_RULECHECK to tiers such as iq0,iq2 and DMRRF_RULECHECK_OUT to a directory to sweep them with the stop rule without the upper bound and write both merges")
	}
	fs, ft := benchStream(t, 30)
	sig := cleanBaseband(fs, 1400)
	pool := newBenchPool(runtime.GOMAXPROCS(0))
	cns := benchFullCNs()
	for _, name := range strings.Split(which, ",") {
		i := slices.IndexFunc(benchEntries(), func(e benchEntry) bool { return e.name() == name })
		if i < 0 {
			t.Fatalf("unknown tier %q", name)
		}
		e := benchEntries()[i]
		sh := benchShared{t, pool, sig, ft, []benchEntry{e}}
		run := func(cs []benchCase) benchPoint { return sh.run([][]benchCase{cs})[0][0] }
		pieces := make([]benchPiece, len(cns))
		for j := range pieces {
			pieces[j] = benchPiece{j, 0, benchBatches(e)}
		}
		files := []benchShardFile{{Mode: e.mode, Tier: e.number(), Points: benchShardRunRule(e, cns, pieces, run, deepCounting)}}
		old, err := benchMergeDeepRule(e, len(cns), files, deepCounting)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := benchMergeDeep(e, len(cns), files)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range map[string]any{"batches": files[0], "old": old, "bound": bound} {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(out+"/"+name+"-"+k+".json", b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		n0, n1 := 0, 0
		for j := range old {
			n0, n1 = n0+old[j].Seeds, n1+bound[j].Seeds
		}
		t.Logf("%s: %d streams without the bound, %d with it", name, n0, n1)
	}
}
