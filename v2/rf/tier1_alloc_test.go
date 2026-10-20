package rf

import "testing"

func TestAllocsTier1Weak(t *testing.T) {
	slots, _ := benchStream(t, 3)
	x := noisyBaseband(slots, 1400, 4, 1)
	const chunk = 1 << 12
	r := mustNew(Config{Rate: basebandRate, Tier: Tier1, MaxWrite: chunk})
	pos, bursts := 0, 0
	n := testing.AllocsPerRun(len(x)/chunk-2, func() {
		bursts += len(r.WriteBaseband(x[pos : pos+chunk]))
		pos += chunk
	})
	if n != 0 || bursts == 0 {
		t.Errorf("I/Q: %v allocations per WriteBaseband, %d bursts", n, bursts)
	}
	a := discAudio(x, benchAudioHz)
	d, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: Tier1})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]Burst, 0, d.maxBursts(1<<10))
	pos, bursts = 0, 0
	n = testing.AllocsPerRun(len(a)/(1<<10)-2, func() {
		dst = d.AppendBursts(dst[:0], a[pos:pos+1<<10])
		bursts += len(dst)
		pos += 1 << 10
	})
	if n != 0 || bursts == 0 {
		t.Errorf("audio: %v allocations per AppendBursts, %d bursts", n, bursts)
	}
}
