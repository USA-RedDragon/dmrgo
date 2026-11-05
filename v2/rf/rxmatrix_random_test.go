//go:build !tinygo

package rf

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

// TestRxMatrixRandom runs rows of the TestRxMatrix plan on random scenarios:
// each run of a row draws its own payload, noise and talker geometry. Every
// case that TestRxMatrix expects to pass must pass on every draw. A case on
// the expected-failure list is not checked, since its symptoms belong to
// the fixed draw of the row. By default the test takes a few random rows;
// with DMRGO_HOLDOUT=1 it takes every row 4 times and logs the failure rate
// of the cases with its Clopper-Pearson 95% upper bound.
func TestRxMatrixRandom(t *testing.T) {
	t.Parallel()
	if testmode.Race {
		t.Skip("each receiver runs on one goroutine; the runs without -race check the matrix")
	}
	plan := mxPlan()
	g := testseed.New(t)
	var picks []int
	if testseed.Holdout() {
		for d := range 4 {
			for i := range plan {
				picks = append(picks, d*len(plan)+i)
			}
		}
	} else {
		for range 8 {
			picks = append(picks, g.IntN(len(plan)))
		}
	}
	var mu sync.Mutex
	cases, fails := 0, 0
	used := make([]bool, len(rxMatrixLimits))
	t.Cleanup(func() {
		if testseed.Holdout() {
			t.Logf("held-out: %d of %d cases fail, 95%% upper bound %.4g, target 0", fails, cases, testseed.UpperBound(fails, cases))
		}
	})
	for k, pick := range picks {
		c := plan[pick%len(plan)]
		row := c.row
		t.Run(fmt.Sprintf("%s/draw%d", row.name(), k), func(t *testing.T) {
			t.Parallel()
			res, err := mxRun(t, row, c.rxs)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, x := range c.rxs {
				key := row.name() + "/" + x.name()
				if rxMatrixXFail[key].checks != "" {
					continue
				}
				r := res[x]
				mxApplyLimits(row, x, r, used)
				cases++
				if got := r.symptoms(); got != "" {
					fails++
					t.Errorf("%s: %s: %s", key, got, strings.Join(r.detail, " | "))
				}
			}
		})
	}
}
