package rf

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

// fixedPayloadTests keep the fixed payloads and noise seeds of the
// helpers: they compare with stored digests, counts or files, or they write
// the measurements that later runs compare with.
var fixedPayloadTests = map[string]bool{
	"TestTierDigests":         true,
	"TestTier3Digest":         true,
	"TestExactTrellisDigest":  true,
	"TestReceiverDirectSeeds": true,
	"TestRxBench":             true,
	"TestRxBenchShard":        true,
	"TestRxBenchRuleCheck":    true,
	"TestRxShardCheck":        true,
	"TestRxBenchRecordings":   true,
	"TestRxBenchMerge":        true,
	"TestRxFloor":             true,
	"TestRxAware":             true,
	"TestRxFloorShard":        true,
	"TestRxFloorMerge":        true,
	"TestFloorProbe":          true,
	"TestFloorTurbo":          true,
	"TestRxACS":               true,
	"TestExtCompare":          true,
	"TestMCUBenchSignals":     true,
	"TestMMDVMCompare":        true,
	"TestRxSheet":             true,
	"TestShortLCBench":        true,
	"TestShortLCCost":         true,
	"TestSweepWorker":         true,
	"TestTablesGenerated":     true,
	"TestRxMatrix":            true,
}

// fixedRun reports whether a measurement or generator variable is set: such
// a run measures the fixed benchmark payloads.
var fixedRun = sync.OnceValue(func() bool {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "DMRRF_") || k == "DMRGO_MCUBENCH_OUT" || k == "DMRGO_MMDVM" {
			return true
		}
	}
	return false
})

// bugPinTests keep the fixed payloads and noise seeds because their seeds
// reproduce a traced bug.
var bugPinTests = map[string]string{
	"TestLeanClockRateNullMS":            "seeds that learned a false MS clock rate",
	"TestRateLineNullMS":                 "cases that stepped a false rate on the rate line",
	"TestCoreTier3WarmNullClock":         "first locks at -521, -473 and +473 ppm on 0 ppm streams (11fd46a52)",
	"TestTier2LockError":                 "seeds that catch the lock frequency and timing faults (5670bc2a3)",
	"TestDiscTier2FalseLockBeforeStream": "a seed whose noise makes a false lock before the stream",
	"TestRxReplayNoisePad":               "starts and seeds where a replay placed a burst before the call",
	"TestRxEmbeddedLCTalkerChange":       "seeds that failed every cell under the old ELC combiner (f63c851a5)",
}

func fixedPayload(t testing.TB) bool {
	if t == nil || fixedRun() {
		return true
	}
	top, _, _ := strings.Cut(t.Name(), "/")
	for _, p := range []string{"Example", "Fuzz", "Benchmark", "TestAllocs"} {
		if strings.HasPrefix(top, p) {
			return true
		}
	}
	_, pin := bugPinTests[top]
	return pin || fixedPayloadTests[top]
}

var reproLogged sync.Map

func nameHash(s string) uint64 { return testseed.NameHash(s) }

func runMix(a, b uint64) uint64 { return testseed.Mix(a, b) }

// scenarioOf returns the random scenario of t for this run, drawn from the
// run seed and the name of t; ok is false for a test that keeps the fixed
// payloads.
func scenarioOf(t testing.TB) (s testscen.Scenario, ok bool) {
	if fixedPayload(t) {
		return s, false
	}
	if _, seen := reproLogged.LoadOrStore(t, true); !seen {
		testseed.Repro(t)
	}
	return testscen.Of(runMix(testseed.RunSeed(), nameHash(t.Name()))), true
}

// randomized returns cfg with the addresses, voice and message of the
// scenario of t.
func randomized(t testing.TB, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return s.Apply(cfg)
	}
	return cfg
}

// runScenario is the scenario of this run for the payloads that the tests
// share through a cache.
var runScenario = sync.OnceValue(func() testscen.Scenario {
	return testscen.Of(runMix(testseed.RunSeed(), 0x7266))
})

func sharedRandomized(cfg rftx.SeqConfig) rftx.SeqConfig { return runScenario().Apply(cfg) }

// noiseSeed returns the noise seed of t for seed: seed itself for a test
// that keeps the fixed payloads, and a draw from the run seed and seed
// otherwise, so the seeds of a test differ on every run.
func noiseSeed(t testing.TB, seed uint64) uint64 {
	if fixedPayload(t) {
		return seed
	}
	return runMix(runMix(testseed.RunSeed(), nameHash(t.Name())), seed)
}

func testConfig(t testing.TB) rftx.SeqConfig { return randomized(t, testConfigFixed()) }

func channel(t testing.TB, iq []byte, txRate, rxRate, offsetHz, snrDB float64, seed uint64) []byte {
	return rftx.Channel(iq, txRate, rxRate, offsetHz, snrDB, noiseSeed(t, seed))
}

// randomizedN returns cfg with the addresses, voice and message of draw salt
// of the scenario of t, so the talkers of one test differ.
func randomizedN(t testing.TB, salt uint64, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return testscen.Of(runMix(s.VoiceSeed, salt)).Apply(cfg)
	}
	return cfg
}

// benchTalkgroup returns the talkgroup of the voice call of a stream whose
// data calls go to ids.Dest: 9 for the fixed bench IDs and an address drawn
// from ids.Dest for a random scenario.
func benchTalkgroup(ids rftx.SeqConfig) uint32 {
	if ids.Dest == 3009201 {
		return 9
	}
	return 1 + uint32(runMix(uint64(ids.Dest), 9)%testscen.MaxAddress)
}

type sharedTB struct{ testing.TB }

func (sharedTB) Name() string { return "shared" }

// sharedRun stands for the tests that share a cached random input: its
// noise seeds come from the run seed only.
var sharedRun testing.TB = sharedTB{}

// mxDraw returns the noise draw of matrix row for t: 0 keeps the fixed noise
// of the row.
func mxDraw(t testing.TB, row mxRow) uint64 {
	if t == nil || fixedPayload(t) {
		return 0
	}
	return runMix(noiseSeed(t, nameHash(row.name())), 1)
}
