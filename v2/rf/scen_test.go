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

// fixedPayloadTests compare with stored digests, counts or files, or write
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
	"TestMSKeyUpGuardHold":               "a payload whose second burst the loop moved over the guard",
	"TestSecondsLockBurstWideFit":        "a payload whose Tier1 lock fit on the narrow filter output",
	"TestSecondsLockBurstDeferredFit":    "a payload whose lock took the strobe when the sync fit deferred",
	"TestSecondsTalkerSwitchHuntFit":     "payloads whose second talker locked on the sync correlation peak",
	"TestRxLevelStepTiming":              "payloads where a level and DC step pulled the timing loop",
	"TestRxLevelStepOneDrop":             "payloads where one level rise dropped the audio gain twice",
	"TestRxTwoTalkerSyncLevelStepHead":   "payloads whose step burst head took the post-step phase unit",
	"TestRxAudioTwoTalkerLevelStepBlind": "payloads whose imbalance kept the blind refit of a stepped burst off",
	"TestDiscInvertedVoicePin":           "voice patterns that delayed the polarity flip past the fourth superframe",
	"TestLeanRCAnchorProof":              "a standalone RC SYNC with a bad EMB after a voice call",
	"TestRxJumpPin":                      "payloads whose first voice A after a 2 ms jump fell in a slot without a hunt",
	"TestShortLCSoftNotWrong":            "seeds where soft Short LCs passed the CRC-8 with wrong bits",
	"TestIQUnheldAdjacentWalk":           "an interferer payload whose residue walked an unheld converter onto the adjacent channel",
	"TestIQSyncTuneAdjacent":             "payloads whose lag-1 mean held the SYNC-fit tune back next to an adjacent channel",
	"TestIQSeekLevelsHoldTune":           "MS calls where the converter followed an EMB seek on noise in the empty slot",
	"TestIQHuntAfterLock":                "Tier 0 locks where the spectrum hunt tuned after the lock",
}

func fixedPayload(t testing.TB) bool {
	if _, pinned := t.(pinnedTB); pinned {
		return false
	}
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

func logRepro(t testing.TB) {
	if _, shared := t.(sharedTB); shared {
		return
	}
	if _, seen := reproLogged.LoadOrStore(t, true); !seen {
		testseed.Repro(t)
	}
}

func nameHash(s string) uint64 { return testseed.NameHash(s) }

func runMix(a, b uint64) uint64 { return testseed.Mix(a, b) }

// pinnedTB draws its payloads and noise from a fixed run seed, for a bug pin
// whose bug needs a payload that the fixed one does not carry.
type pinnedTB struct {
	testing.TB
	run uint64
}

func scenarioOf(t testing.TB) (s testscen.Scenario, ok bool) {
	if p, pinned := t.(pinnedTB); pinned {
		return testscen.Of(p.run), true
	}
	if fixedPayload(t) {
		return s, false
	}
	logRepro(t)
	return testscen.Of(runMix(testseed.RunSeed(), nameHash(t.Name()))), true
}

func randomized(t testing.TB, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return s.Apply(cfg)
	}
	return cfg
}

var runScenario = sync.OnceValue(func() testscen.Scenario {
	return testscen.Of(runMix(testseed.RunSeed(), 0x7266))
})

func sharedRandomized(cfg rftx.SeqConfig) rftx.SeqConfig { return runScenario().Apply(cfg) }

func noiseSeed(t testing.TB, seed uint64) uint64 {
	if p, pinned := t.(pinnedTB); pinned {
		return runMix(p.run, seed)
	}
	if fixedPayload(t) {
		return seed
	}
	logRepro(t)
	return runMix(runMix(testseed.RunSeed(), nameHash(t.Name())), seed)
}

func testConfig(t testing.TB) rftx.SeqConfig { return randomized(t, testConfigFixed()) }

func channel(t testing.TB, iq []byte, txRate, rxRate, offsetHz, snrDB float64, seed uint64) []byte {
	return rftx.Channel(iq, txRate, rxRate, offsetHz, snrDB, noiseSeed(t, seed))
}

func randomizedN(t testing.TB, salt uint64, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return testscen.Of(runMix(s.VoiceSeed, salt)).Apply(cfg)
	}
	return cfg
}

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
