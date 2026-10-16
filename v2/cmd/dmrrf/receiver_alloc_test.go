package main

import (
	"testing"
)

// TestAllocsReceiver checks that Write and WriteBaseband of every tier
// allocate nothing in steady state, over a BS outbound voice call in
// chunks of 16 KiB, while the receiver locks, follows the stream and emits
// bursts.
func TestAllocsReceiver(t *testing.T) {
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 2
	built, err := Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := ModulateBS(built.Slots)
	iq := channel(ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 1500, 25, 0)
	const chunk = 1 << 14
	for _, tier := range []Tier{TierDisc, TierCoherent, TierExact, TierMax} {
		r := NewReceiver(ReceiverConfig{Rate: 2.048e6, Tier: tier, MaxWrite: chunk})
		pos, bursts := 0, 0
		n := testing.AllocsPerRun(len(iq)/chunk-2, func() {
			bursts += len(r.Write(iq[pos : pos+chunk]))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per Write, %d bursts", tier, n, bursts)
		}
		x := basebandStream(built.Slots, 1500)
		r = NewReceiver(ReceiverConfig{Rate: basebandRate, Tier: tier})
		pos, bursts = 0, 0
		n = testing.AllocsPerRun(len(x)/chunk-2, func() {
			bursts += len(r.WriteBaseband(x[pos : pos+chunk]))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per WriteBaseband, %d bursts", tier, n, bursts)
		}
	}
}
