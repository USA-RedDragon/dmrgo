package rf

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestAllocsReceiver checks that Write and WriteBaseband of every tier
// allocate nothing in steady state, over a BS outbound voice call in
// chunks of 16 KiB, while the receiver locks, follows the stream and emits
// bursts.
func TestAllocsReceiver(t *testing.T) {
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 2
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.ModulateBS(built.Slots)
	iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 1500, 25, 0)
	const chunk = 1 << 14
	for _, tier := range []Tier{TierDisc, TierCoherent, TierExact, TierMax} {
		r := mustNew(Config{Rate: 2.048e6, Tier: tier, MaxWrite: chunk})
		pos, bursts := 0, 0
		n := testing.AllocsPerRun(len(iq)/chunk-2, func() {
			bursts += len(r.Write(iq[pos : pos+chunk]))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per Write, %d bursts", tier, n, bursts)
		}
		x := basebandStream(built.Slots, 1500)
		r = mustNew(Config{Rate: basebandRate, Tier: tier})
		pos, bursts = 0, 0
		n = testing.AllocsPerRun(len(x)/chunk-2, func() {
			bursts += len(r.WriteBaseband(x[pos : pos+chunk]))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per WriteBaseband, %d bursts", tier, n, bursts)
		}
		bursts = 0
		r = mustNew(Config{Rate: basebandRate, Tier: tier, OnBurst: func(*Burst) { bursts++ }})
		pos = 0
		n = testing.AllocsPerRun(len(x)/chunk-2, func() {
			r.WriteBaseband(x[pos : pos+chunk])
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per WriteBaseband with OnBurst, %d bursts", tier, n, bursts)
		}
	}
}

// TestAllocsReceiverRAM checks that New of each tier in
// rxbenchPath keeps at most 10 percent more heap than the baseline. It
// reads the heap of the whole process, so it does not run in parallel.
func TestAllocsReceiverRAM(t *testing.T) {
	b, err := os.ReadFile(rxbenchPath)
	if err != nil {
		t.Fatal(err)
	}
	var base benchData
	if err := json.Unmarshal(b, &base); err != nil {
		t.Fatal(err)
	}
	for _, want := range base.Tiers {
		if got := receiverRAM(Tier(want.Tier)); got > want.RAMBytes*11/10 {
			t.Errorf("tier %d: New keeps %d bytes, baseline %d", want.Tier, got, want.RAMBytes)
		}
	}
}
