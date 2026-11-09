// Package testseed gives the tests of the module fresh random seeds on
// every run, so a bar cannot hold only for one hand-picked seed list.
//
// Every test run draws a fresh run seed from the clock, or takes it from
// DMRGO_TESTSEED. A test derives its own stream from the run seed and its
// name, and each trial of a test its own stream from that, so one run seed
// reproduces every draw of the run. A failing test logs the run seed and the
// command that reproduces it.
//
// By default a test checks a few fresh draws. With DMRGO_HOLDOUT=1 it checks
// many and reports each bar as a failure rate with its Clopper-Pearson 95%
// upper bound. The seeds differ on every run, so they are a held-out set
// that no change can be fitted to.
package testseed

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Environment variables that select the draws.
const (
	EnvSeed    = "DMRGO_TESTSEED"
	EnvHoldout = "DMRGO_HOLDOUT"
	EnvTrial   = "DMRGO_TRIAL"
)

var runSeed = sync.OnceValue(func() uint64 {
	if v := os.Getenv(EnvSeed); v != "" {
		if s, err := strconv.ParseUint(v, 0, 64); err == nil {
			return s
		}
	}
	return Mix(uint64(time.Now().UnixNano()), uint64(os.Getpid()))
})

// RunSeed returns the seed of this test run.
func RunSeed() uint64 { return runSeed() }

// Holdout reports whether DMRGO_HOLDOUT=1 selects the held-out runs.
func Holdout() bool { return os.Getenv(EnvHoldout) == "1" }

// N returns holdout under DMRGO_HOLDOUT=1 and fast otherwise.
func N(fast, holdout int) int {
	if Holdout() {
		return holdout
	}
	return fast
}

// Mix returns a well mixed 64-bit value of a and b (the SplitMix64 finalizer
// of their sum).
func Mix(a, b uint64) uint64 {
	z := a ^ (b+0x9E3779B97F4A7C15)*0xBF58476D1CE4E5B9
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	return z ^ z>>31
}

// NameHash returns the 64-bit FNV-1a hash of s.
func NameHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// Gen draws the random values of one test or one trial. A Gen is not safe
// for use by more than one goroutine.
type Gen struct {
	seed uint64
	rng  *rand.Rand
}

// New returns the Gen of t: its stream comes from the run seed and the name
// of t. When t fails, New logs the run seed and the command that reproduces
// the failure.
func New(t testing.TB) *Gen {
	t.Helper()
	Repro(t)
	return Of(Mix(RunSeed(), NameHash(t.Name())))
}

// Of returns the Gen of seed.
func Of(seed uint64) *Gen {
	return &Gen{seed: seed, rng: rand.New(rand.NewPCG(seed, Mix(seed, 1)))}
}

// Seed returns the seed of g.
func (g *Gen) Seed() uint64 { return g.seed }

// Uint64 returns a random 64-bit value, for a noise seed.
func (g *Gen) Uint64() uint64 { return g.rng.Uint64() }

// IntN returns a random value in [0, n).
func (g *Gen) IntN(n int) int { return g.rng.IntN(n) }

// Float64 returns a random value in [0, 1).
func (g *Gen) Float64() float64 { return g.rng.Float64() }

// Seeds returns n random noise seeds.
func (g *Gen) Seeds(n int) []uint64 {
	s := make([]uint64, n)
	for i := range s {
		s[i] = g.rng.Uint64()
	}
	return s
}

// Bytes returns n random octets.
func (g *Gen) Bytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(g.rng.Uint32())
	}
	return b
}

// Trial returns the Gen of trial i of the test with Gen g.
func (g *Gen) Trial(i int) *Gen { return Of(Mix(g.seed, uint64(i)+2)) }

// Repro makes t log the run seed and the command that reproduces the run
// when t fails. New calls it.
func Repro(t testing.TB) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("run seed %d; reproduce: %s", RunSeed(), Command(t, -1))
		}
	})
}

// Command returns the shell command that reruns t with this run seed, only
// trial when trial is 0 or more.
func Command(t testing.TB, trial int) string {
	parts := strings.Split(t.Name(), "/")
	for i, p := range parts {
		parts[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	env := fmt.Sprintf("%s=%d", EnvSeed, RunSeed())
	if Holdout() {
		env += " " + EnvHoldout + "=1"
	}
	if trial >= 0 {
		env += fmt.Sprintf(" %s=%d", EnvTrial, trial)
	}
	dir, pkg := moduleDir()
	return fmt.Sprintf("(cd %s && %s go test -count=1 -run '%s' %s)", dir, env, strings.Join(parts, "/"), pkg)
}

func moduleDir() (string, string) {
	wd, err := os.Getwd()
	if err != nil {
		return ".", "."
	}
	for d := wd; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			rel, err := filepath.Rel(d, wd)
			if err != nil {
				return wd, "."
			}
			return d, "./" + filepath.ToSlash(rel)
		}
		up := filepath.Dir(d)
		if up == d {
			return wd, "."
		}
		d = up
	}
}

// Trials checks n trials of t, each with its own Gen from Trial, on up to
// GOMAXPROCS goroutines. check returns an empty string for a trial that
// meets the bar and the failure otherwise; it must be safe to call from
// several goroutines. Outside held-out runs every trial must pass. In a
// held-out run Trials logs the failure rate and its Clopper-Pearson 95%
// upper bound, and fails t when a trial fails and target is 0, or when the
// bound is above target. DMRGO_TRIAL=k checks only trial k.
func Trials(t testing.TB, fast, holdout int, target float64, check func(g *Gen) string) {
	t.Helper()
	g := New(t)
	n := N(fast, holdout)
	only := -1
	if v := os.Getenv(EnvTrial); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k >= 0 {
			only, n = k, 1
		}
	}
	trialOf := func(i int) int {
		if only >= 0 {
			return only
		}
		return i
	}
	msgs := make([]string, n)
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(n, runtime.GOMAXPROCS(0)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				msgs[i] = check(g.Trial(trialOf(i)))
			}
		}()
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
	if !Holdout() {
		for i, m := range msgs {
			if m != "" {
				t.Errorf("trial %d: %s; reproduce: %s", trialOf(i), m, Command(t, trialOf(i)))
			}
		}
		return
	}
	fails, first := 0, -1
	for i, m := range msgs {
		if m != "" {
			fails++
			if first < 0 {
				first = i
			}
		}
	}
	ub := UpperBound(fails, n)
	t.Logf("held-out: %d of %d trials fail, rate %.4g, 95%% upper bound %.4g, target %.4g", fails, n, float64(fails)/float64(n), ub, target)
	if fails == 0 {
		return
	}
	t.Logf("first failing trial %d: %s; reproduce: %s", trialOf(first), msgs[first], Command(t, trialOf(first)))
	if target == 0 || ub > target {
		t.Errorf("held-out failure rate %d/%d, 95%% upper bound %.4g above target %.4g", fails, n, ub, target)
	}
}

// UpperBound returns the Clopper-Pearson 95% (two-sided) upper bound of a
// failure rate with fails failures in n trials.
func UpperBound(fails, n int) float64 {
	if n <= 0 || fails >= n {
		return 1
	}
	lo, hi := float64(fails)/float64(n), 1.0
	for range 200 {
		mid := (lo + hi) / 2
		if binomCDF(fails, n, mid) > 0.025 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

// Alpha is the one-sided error level of the rate bars: a receiver whose true
// rate meets a bar fails RateBelow, RateAbove or RateHigher in at most this
// share of runs.
const Alpha = 1e-4

// RateBelow reports whether k successes in n trials show a success rate
// below bar: the one-sided Clopper-Pearson upper bound of the rate at
// confidence 1-Alpha lies below bar.
func RateBelow(k, n int, bar float64) bool {
	if n <= 0 || k >= n {
		return false
	}
	return binomCDF(k, n, bar) < Alpha
}

// RateAbove reports whether k events in n trials show an event rate above
// bar: the one-sided Clopper-Pearson lower bound of the rate at confidence
// 1-Alpha lies above bar.
func RateAbove(k, n int, bar float64) bool {
	if n <= 0 || k <= 0 {
		return false
	}
	if bar <= 0 {
		return true
	}
	return 1-binomCDF(k-1, n, bar) < Alpha
}

// RateHigher reports whether k1 events in n1 trials show a higher event rate
// than k2 events in n2 trials, by the exact conditional test: with equal
// rates, k1 of the k1+k2 events fall in the first set with probability
// n1/(n1+n2) each, and the chance of k1 or more is below Alpha.
func RateHigher(k1, n1, k2, n2 int) bool {
	if n1 <= 0 || n2 <= 0 || k1 <= 0 {
		return false
	}
	return 1-binomCDF(k1-1, k1+k2, float64(n1)/float64(n1+n2)) < Alpha
}

func binomCDF(k, n int, p float64) float64 {
	if k >= n || p <= 0 {
		return 1
	}
	if k < 0 {
		return 0
	}
	if p >= 1 {
		return 0
	}
	lp, lq := math.Log(p), math.Log1p(-p)
	ln := lgamma(n + 1)
	s := 0.0
	for i := 0; i <= k; i++ {
		s += math.Exp(ln - lgamma(i+1) - lgamma(n-i+1) + float64(i)*lp + float64(n-i)*lq)
	}
	return s
}

func lgamma(n int) float64 {
	v, _ := math.Lgamma(float64(n))
	return v
}

// The statistical bars of the suite share one false failure budget: a run
// of the suite on a receiver that meets every bar fails with probability at
// most SuiteAlpha, as each of its at most BarChecks bar checks fails with
// probability at most BarAlpha.
const (
	SuiteAlpha = 1e-6
	BarChecks  = 500
	BarAlpha   = SuiteAlpha / BarChecks
)

// BarZ is the standard normal quantile with upper tail BarAlpha.
var BarZ = math.Sqrt2 * math.Erfcinv(2*BarAlpha)

// CountAbove reports whether a count of k lies above the level that a
// count of the given mean and variance exceeds with probability BarAlpha.
// The count is negative binomial with that mean and variance when the
// variance is above the mean. Otherwise it is Poisson with that mean when
// the mean is below 20, and normal with that mean and variance above.
func CountAbove(k int, mean, variance float64) bool {
	switch {
	case k <= 0:
		return false
	case mean <= 0:
		return true
	case variance <= mean && mean >= 20:
		return float64(k) > mean+BarZ*math.Sqrt(variance)
	}
	return countTail(k, mean, variance) < BarAlpha
}

// CountBar returns the largest count that CountAbove accepts.
func CountBar(mean, variance float64) int {
	k := 0
	for !CountAbove(k+1, mean, variance) {
		k++
	}
	return k
}

func countTail(k int, mean, variance float64) float64 {
	lpmf := func(i int) float64 {
		x := float64(i)
		return x*math.Log(mean) - mean - lgammaF(x+1)
	}
	if variance > mean {
		r, p := mean*mean/(variance-mean), mean/variance
		lpmf = func(i int) float64 {
			x := float64(i)
			return lgammaF(x+r) - lgammaF(r) - lgammaF(x+1) + r*math.Log(p) + x*math.Log1p(-p)
		}
	}
	s := 0.0
	for i := k; ; i++ {
		v := math.Exp(lpmf(i))
		s += v
		if float64(i) > mean && v < s*1e-15 {
			return s
		}
	}
}

func lgammaF(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}
