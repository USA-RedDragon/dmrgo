package rf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

// holdoutChild marks a test binary that a held-out run started.
const holdoutChild = "child"

// TestMain runs the held-out mode of the package: with DMRGO_HOLDOUT=1 it
// runs the selected tests DMRGO_HOLDOUT_RUNS times (default 16), each run
// in its own process with a fresh run seed, so every run draws new payloads
// and noise. It then prints, for each test that failed in any run, the
// failure rate with its Clopper-Pearson 95% upper bound and the command
// that reproduces one failing run. The target of every bar is 0 failures.
func TestMain(m *testing.M) {
	if !testseed.Holdout() {
		os.Exit(m.Run())
	}
	os.Exit(holdoutRuns())
}

var failLine = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)

func holdoutRuns() int {
	runs := 16
	if n, err := strconv.Atoi(os.Getenv("DMRGO_HOLDOUT_RUNS")); err == nil && n > 0 {
		runs = n
	}
	base := testseed.RunSeed()
	args := []string{"-test.v"}
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "-test.v") {
			args = append(args, a)
		}
	}
	fails := map[string][]uint64{}
	for i := range runs {
		seed := runMix(base, uint64(i))
		cmd := exec.CommandContext(context.Background(), os.Args[0], args...)
		cmd.Env = append(os.Environ(), testseed.EnvHoldout+"="+holdoutChild, fmt.Sprintf("%s=%d", testseed.EnvSeed, seed))
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		n := 0
		sc := bufio.NewScanner(&out)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			if m := failLine.FindStringSubmatch(sc.Text()); m != nil {
				fails[m[1]] = append(fails[m[1]], seed)
				n++
			}
		}
		if err != nil && n == 0 {
			fmt.Printf("held-out run %d (seed %d) failed with no test failure: %v\n%s\n", i, seed, err, out.String())
			return 1
		}
		fmt.Printf("held-out run %d of %d, seed %d: %d failures\n", i+1, runs, seed, n)
	}
	names := make([]string, 0, len(fails))
	for k := range fails {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		f := len(fails[k])
		fmt.Printf("HOLDOUT\t%s\t%d/%d\trate %.3g\tupper95 %.3g\tseed %d\n", k, f, runs, float64(f)/float64(runs), testseed.UpperBound(f, runs), fails[k][0])
	}
	fmt.Printf("held-out: %d runs, %d failing tests or subtests; reproduce one with DMRGO_TESTSEED=<seed> go test -count=1 -run '<test>' ./rf\n", runs, len(names))
	if len(names) > 0 {
		return 1
	}
	return 0
}
