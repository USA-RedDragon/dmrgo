package rf

import (
	"reflect"
	"testing"
)

func TestFront96Same(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	for _, cn := range []float64{30, 8, 4, 2, 0} {
		a := discAudio(noisyBaseband(slots, 1400, cn, 3), 9600)
		for _, chunk := range []int{1, 7, 1024} {
			var got [2][]Burst
			for i, on := range []bool{false, true} {
				opt := discOptions()
				opt.MidTiming, opt.Front96 = true, on
				r := mustAudioOpt(AudioRxConfig{Rate: 9600, OnBursts: func(bs []Burst) { got[i] = append(got[i], bs...) }}, opt)
				for k := 0; k < len(a); k += chunk {
					must(r.writeAll(a[k:min(k+chunk, len(a))]))
				}
				r.flushAll()
			}
			t.Logf("C/N %v, chunk %d: %d bursts", cn, chunk, len(got[0]))
			if len(got[0]) == 0 && cn > 4 {
				t.Errorf("C/N %v, chunk %d: no bursts", cn, chunk)
			}
			if !reflect.DeepEqual(got[0], got[1]) {
				t.Errorf("C/N %v, chunk %d: %d bursts with run, %d with run96, or they differ", cn, chunk, len(got[0]), len(got[1]))
			}
		}
	}
}
