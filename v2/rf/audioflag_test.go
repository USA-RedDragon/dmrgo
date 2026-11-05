//go:build !tinygo

package rf

import (
	"reflect"
	"testing"
)

func TestAudioFlagLeavesIQ(t *testing.T) { //nolint:paralleltest // it flips the package audio flag
	slots, _ := benchStream(t, 2)
	x := cleanBaseband(slots, 1400).noisy(t, 12, 1)
	was := audioOn()
	defer audioRx.Store(was)
	for _, e := range benchEntries() {
		if e.mode == benchAudio {
			continue
		}
		audioRx.Store(false)
		off := benchReceive(t, x, e)
		audioRx.Store(true)
		on := benchReceive(t, x, e)
		if len(off) == 0 || !reflect.DeepEqual(off, on) {
			t.Errorf("%s: %d bursts with no audio receiver, %d after one, or they differ", e.name(), len(off), len(on))
		}
	}
}
