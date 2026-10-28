package rf

import "testing"

func TestAllocsDiscReceiver(t *testing.T) {
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 20, 1), 9600)
	var r AudioReceiver
	dst := make([]Burst, 0, 16)
	const chunk = 1 << 10
	pos, bursts := 0, 0
	if n := testing.AllocsPerRun(1, func() {
		if err := r.Init(AudioRxConfig{Rate: 9600}); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("Init: %v allocations", n)
	}
	n := testing.AllocsPerRun(len(a)/chunk-2, func() {
		dst = r.AppendBursts(dst[:0], a[pos:pos+chunk])
		bursts += len(dst)
		pos += chunk
	})
	if n != 0 || bursts == 0 {
		t.Errorf("%v allocations per AppendBursts, %d bursts", n, bursts)
	}
	var pe RxPreEmphasis
	if n := testing.AllocsPerRun(1, func() {
		if err := r.Init(AudioRxConfig{Rate: 9600, PreEmphasis: pe.Init()}); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Errorf("Init with PreEmphasis: %v allocations", n)
	}
	a = radioAudio(noisyBaseband(slots, 1400, 20, 1), 9600, true)
	pos, bursts = 0, 0
	n = testing.AllocsPerRun(len(a)/chunk-2, func() {
		dst = r.AppendBursts(dst[:0], a[pos:pos+chunk])
		bursts += len(dst)
		pos += chunk
	})
	if n != 0 || bursts == 0 {
		t.Errorf("PreEmphasis: %v allocations per AppendBursts, %d bursts", n, bursts)
	}
}
