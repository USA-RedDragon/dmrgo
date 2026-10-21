package rf

import "testing"

func TestAllocsDiscTier2(t *testing.T) {
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 10, 1), 9600)
	r, err := NewDisc(&DiscConfig{Hz: 9600, Tier: Tier2})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]Burst, 0, r.maxBursts(1<<10))
	const chunk = 1 << 10
	pos, bursts := 0, 0
	n := testing.AllocsPerRun(len(a)/chunk-2, func() {
		dst = r.AppendBursts(dst[:0], a[pos:pos+chunk])
		bursts += len(dst)
		pos += chunk
	})
	if n != 0 || bursts == 0 {
		t.Errorf("%v allocations per AppendBursts, %d bursts", n, bursts)
	}
}
