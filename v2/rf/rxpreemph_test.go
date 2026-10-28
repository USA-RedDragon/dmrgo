package rf

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

func TestRxPreEmphasisInit(t *testing.T) {
	t.Parallel()
	var nilPE *RxPreEmphasis
	if nilPE.Init() != nil || nilPE.set() || (&RxPreEmphasis{}).set() || !NewRxPreEmphasis().set() {
		t.Error("RxPreEmphasis.Init or set")
	}
	var r AudioReceiver
	var ce *dmrgo.ConfigError
	if err := r.Init(AudioRxConfig{PreEmphasis: &RxPreEmphasis{}}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.PreEmphasis" {
		t.Fatalf("PreEmphasis not set up: %v", err)
	}
	if _, err := NewAudioReceiver(&AudioRxConfig{PreEmphasis: &RxPreEmphasis{}}); !errors.As(err, &ce) {
		t.Fatalf("NewAudioReceiver, PreEmphasis not set up: %v", err)
	}
	var pe RxPreEmphasis
	if err := r.Init(AudioRxConfig{Rate: 9600, PreEmphasis: pe.Init()}); err != nil || r.pe != &pe {
		t.Fatalf("Init with PreEmphasis: %v", err)
	}
	if err := r.Init(AudioRxConfig{Rate: 9600}); err != nil || r.pe != nil {
		t.Fatalf("Init without PreEmphasis: %v", err)
	}
}

func TestRxPreEmphasisFilter(t *testing.T) {
	t.Parallel()
	for _, hz := range []int{2 * symbolRate, discRate / 2, discRate} {
		var f rxPE
		rxPEStart(&f, hz)
		x := make([]int16, hz)
		for i := range x {
			x[i] = 3000
		}
		rxPEFilter(&f, x)
		if v := x[len(x)-1]; v != 3000 {
			t.Errorf("%d Hz: DC gain, %d for 3000", hz, v)
		}
		for i := range x {
			x[i] = int16(32767 * (1 - 2*(i&1)))
		}
		rxPEFilter(&f, x)
		if v := x[len(x)-1]; v != -32768 && v != 32767 {
			t.Errorf("%d Hz: no saturation, %d", hz, v)
		}
	}
}

func TestRxPreEmphasisRecovers(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 3)
	x := noisyBaseband(slots, 0, 9, 1)
	for _, hz := range []int{8000, 9600, 48000} {
		exact := func(a []int16, pe *RxPreEmphasis) int {
			var r AudioReceiver
			if err := r.Init(AudioRxConfig{Rate: hz, PreEmphasis: pe}); err != nil {
				t.Fatal(err)
			}
			got := r.AppendFlush(r.AppendBursts(nil, a))
			return scoreBench(truth, got)["ambe"].Exact
		}
		flat := exact(radioAudio(x, hz, false), nil)
		de := radioAudio(x, hz, true)
		off, on := exact(de, nil), exact(de, NewRxPreEmphasis())
		t.Logf("%d Hz: flat %d, de-emphasized %d, with pre-emphasis %d", hz, flat, off, on)
		if off >= flat/2 || flat-on > (flat-off)/10 {
			t.Errorf("%d Hz: flat %d, de-emphasized %d, with pre-emphasis %d", hz, flat, off, on)
		}
	}
}
