//go:build !tinygo

package rf

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

var updateDirectSeeds = flag.Bool("update-directseeds", false, "rewrite testdata/directseeds.json from this receiver")

const (
	directSeedsFile   = "testdata/directseeds.json"
	directSeedsFirst  = 9000
	directSeedsLong   = 256
	directSeedsZ      = 2
	directSeedsFloor  = 0.01
	directSeedsSpread = 1
	directSeedsShort  = 12
)

type directSeedsRef struct {
	Truth int              `json:"truth"`
	Cells map[string][]int `json:"cells"`
}

func directSeedsCells() []struct {
	tier Tier
	ppm  float64
} {
	var out []struct {
		tier Tier
		ppm  float64
	}
	for _, ppm := range []float64{0, -200, 200} {
		for tier := Tier0; tier <= Tier3; tier++ {
			out = append(out, struct {
				tier Tier
				ppm  float64
			}{tier, ppm})
		}
	}
	return out
}

func directSeedsKey(tier Tier, ppm float64) string {
	return fmt.Sprintf("t%d/%+g", tier-Tier0, ppm)
}

func directSeedsRun(t *testing.T, seeds []uint64) (map[string][]int, int) {
	d := directShared(t)
	cells := directSeedsCells()
	out := make(map[string][]int, len(cells))
	streams := map[float64][2][]float64{}
	for _, c := range cells {
		out[directSeedsKey(c.tier, c.ppm)] = make([]int, len(seeds))
		if _, ok := streams[c.ppm]; !ok {
			f, e := resampleFM(d.fm, d.env, c.ppm)
			streams[c.ppm] = [2][]float64{f, e}
		}
	}
	var truth int
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range cells {
		for i, seed := range seeds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				s := streams[c.ppm]
				sc := scoreMS(d.truth, receiveBaseband(noisyFM(t, s[0], s[1], 1400, 5, seed), c.tier))["ambe_c0c1"]
				mu.Lock()
				out[directSeedsKey(c.tier, c.ppm)][i], truth = sc.Exact, sc.Truth
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return out, truth
}

func TestReceiverDirectSeeds(t *testing.T) {
	t.Parallel()
	all := make([]uint64, directSeedsLong)
	for i := range all {
		all[i] = directSeedsFirst + uint64(i)
	}
	if *updateDirectSeeds {
		cells, truth := directSeedsRun(t, all)
		b, err := json.MarshalIndent(directSeedsRef{Truth: truth, Cells: cells}, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(directSeedsFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(directSeedsFile)
	if err != nil {
		t.Fatal(err)
	}
	var ref directSeedsRef
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	seeds := all[:directSeedsShort]
	if testmode.Long() {
		seeds = all
	} else if testmode.Race {
		seeds = seeds[:1]
	}
	got, truth := directSeedsRun(t, seeds)
	if truth != ref.Truth {
		t.Fatalf("%d frames per call set, the reference has %d", truth, ref.Truth)
	}
	for _, c := range directSeedsCells() {
		k := directSeedsKey(c.tier, c.ppm)
		want := ref.Cells[k]
		if len(want) != len(all) {
			t.Fatalf("%s: %d reference seeds, want %d", k, len(want), len(all))
		}
		diff := make([]float64, len(seeds))
		var have, base int
		for i, seed := range seeds {
			r := want[slices.Index(all, seed)]
			diff[i] = float64(got[k][i] - r)
			have, base = have+got[k][i], base+r
		}
		mean, se := meanSE(diff)
		if len(seeds) == 1 {
			se = 0
		}
		se = max(se, directSeedsSpread/math.Sqrt(float64(len(seeds))))
		if mean < 0 && -mean > directSeedsZ*se {
			t.Errorf("%s: %+d frames over %d seeds, SE %.1f", k, have-base, len(seeds), se*float64(len(seeds)))
		}
		n := float64(truth * len(seeds))
		if share, floor := float64(have)/n, float64(base)/n-directSeedsFloor; share < floor && !math.IsNaN(share) {
			t.Errorf("%s: %.4f of the frames, floor %.4f", k, share, floor)
		}
	}
}
