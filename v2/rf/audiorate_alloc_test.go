package rf

import "testing"

func TestAllocsAudioRate(t *testing.T) {
	slots, _ := benchStream(t, 1)
	a := sincResample(discAudio(noisyBaseband(slots, 1400, 30, 4), 2*discRate), 2*discRate, 44100)
	var r AudioReceiver
	if err := r.Init(AudioRxConfig{Rate: 44100, OnBursts: func([]Burst) {}}); err != nil {
		t.Fatal(err)
	}
	pos := 0
	if n := testing.AllocsPerRun(len(a)/4096-2, func() {
		_, _ = r.Write(a[pos : pos+4096])
		pos += 4096
	}); n != 0 {
		t.Errorf("Init at 44.1 kHz: %v allocations per Write", n)
	}
}
