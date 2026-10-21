package trunk

import (
	"testing"
	"time"
)

func TestNextWake_Table6_14(t *testing.T) {
	t.Parallel()
	s := msSvc{psRQ: 2, psOffset: 2, counter: 65 * 4, counterAt: 0}
	block := func(b int) time.Duration { return time.Duration(b-65*4-1) * counterBlock }
	for _, c := range []struct {
		now  time.Duration
		want time.Duration
	}{
		{0, block(66 * 4)},
		{block(66*4) + psFrame - 1, block(66 * 4)},
		{block(66*4) + psFrame, block(70 * 4)},
		{block(70*4) + psFrame, block(74 * 4)},
	} {
		if got := s.nextWake(c.now); got != c.want {
			t.Errorf("nextWake(%v) = %v, want %v", c.now, got, c.want)
		}
	}
	for ps := uint32(65); ps <= 70; ps++ {
		wake := ps&3 == 2
		if want := ps == 66 || ps == 70; wake != want {
			t.Errorf("PS_Counter %d: wake %v, want %v", ps, wake, want)
		}
	}
	s = msSvc{psRQ: 7, psOffset: 127, counter: 511, counterAt: time.Second}
	if got, want := s.nextWake(time.Second), time.Second+time.Duration(127*4+512-511-1)*counterBlock; got != want {
		t.Errorf("wrap: nextWake = %v, want %v", got, want)
	}
}
