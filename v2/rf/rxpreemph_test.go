package rf

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
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

// TestRxPreEmphasisRecovers checks that RxPreEmphasis recovers de-emphasized
// audio at 9 dB: over preEmphasisDraws payload draws, de-emphasis alone must
// lose at least half of the AMBE frames of flat audio, and pre-emphasis must
// get back all but a tenth of that loss. One draw varies by about 1.8 frames
// against a margin of about 3, so the test pools 16 draws.
func TestRxPreEmphasisRecovers(t *testing.T) {
	t.Parallel()
	const preEmphasisDraws = 16
	type sums struct{ flat, off, on int }
	got := map[int]*sums{8000: {}, 9600: {}, 48000: {}}
	for d := range uint64(preEmphasisDraws) {
		slots, truth := benchStreamOf(t, randomizedN(t, d+1, rftx.SeqConfig{Source: 3009101, BS: 990913, Dest: 3009201, Message: benchMessage(d)}), 3)
		x := noisyBaseband(t, slots, 0, 9, 1+d)
		for hz, s := range got {
			exact := func(a []int16, pe *RxPreEmphasis) int {
				var r AudioReceiver
				if err := r.Init(AudioRxConfig{Rate: hz, PreEmphasis: pe}); err != nil {
					t.Fatal(err)
				}
				return scoreBench(truth, r.AppendFlush(r.AppendBursts(nil, a)))["ambe"].Exact
			}
			de := radioAudio(x, hz, true)
			s.flat += exact(radioAudio(x, hz, false), nil)
			s.off += exact(de, nil)
			s.on += exact(de, NewRxPreEmphasis())
		}
	}
	for _, hz := range []int{8000, 9600, 48000} {
		s := got[hz]
		t.Logf("%d Hz over %d draws: flat %d, de-emphasized %d, with pre-emphasis %d", hz, preEmphasisDraws, s.flat, s.off, s.on)
		if s.off >= s.flat/2 || s.flat-s.on > (s.flat-s.off)/10 {
			t.Errorf("%d Hz over %d draws: flat %d, de-emphasized %d, with pre-emphasis %d", hz, preEmphasisDraws, s.flat, s.off, s.on)
		}
	}
}
