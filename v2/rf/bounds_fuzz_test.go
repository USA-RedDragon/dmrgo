package rf

import (
	"errors"
	"testing"
)

const discFlushBound = 26

func discBoundAudio(t testing.TB) map[int][]int16 {
	t.Helper()
	slots, _ := benchStream(t, 2)
	x := noisyBaseband(slots, 900, 35, 3)
	a := map[int][]int16{}
	for _, hz := range []int{24000, 12000, 9600} {
		a[hz] = discAudio(x, hz)
	}
	return a
}

func runDiscBounds(t *testing.T, audio map[int][]int16, sel, tier byte, cuts []byte) int {
	t.Helper()
	hz := []int{24000, 12000, 9600}[int(sel)%3]
	x := audio[hz]
	if sel&4 != 0 {
		x = x[len(x)/3:]
	}
	r, err := NewDisc(DiscConfig{Hz: hz, Tier: []Tier{Tier0, Tier1, Tier2}[int(tier)%3]})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	var dst []Burst
	for off, i := 0, 0; off < len(x); i++ {
		n := 1 + int(sel)*37
		if len(cuts) > 0 {
			n = 1 + int(cuts[i%len(cuts)])*int(cuts[i%len(cuts)])
		}
		n = min(n, len(x)-off)
		w, err := []Burst(nil), error(nil)
		if i%2 == 1 {
			w, err = r.Write(x[off : off+n])
		}
		switch {
		case i%2 == 0 || err != nil:
			if err != nil && (!errors.Is(err, ErrWriteTooLong) || n <= 1<<12 || w != nil) {
				t.Fatalf("%d Hz: a Write of %d samples: %v", hz, n, err)
			}
			dst = r.AppendBursts(dst[:0], x[off:off+n])
		default:
			dst = w
		}
		if len(dst) > n/72+16 {
			t.Fatalf("%d Hz: %d samples gave %d bursts", hz, n, len(dst))
		}
		total += len(dst)
		off += n
	}
	if sel&8 != 0 {
		dst = r.Flush()
	} else {
		dst = r.AppendFlush(dst[:0])
	}
	if len(dst) > discFlushBound {
		t.Fatalf("%d Hz: flush gave %d bursts", hz, len(dst))
	}
	return total + len(dst)
}

func TestDiscReceiverBounds(t *testing.T) {
	t.Parallel()
	audio := discBoundAudio(t)
	for sel := range byte(12) {
		if n := runDiscBounds(t, audio, sel, sel, []byte{1, 9, 60, 200}); n == 0 {
			t.Errorf("selector %d gave no bursts", sel)
		}
	}
}

func FuzzDiscReceiverBounds(f *testing.F) {
	audio := discBoundAudio(f)
	f.Add(byte(0), byte(1), []byte{3})
	f.Add(byte(2), byte(2), []byte{0, 255, 17})
	f.Fuzz(func(t *testing.T, sel, tier byte, cuts []byte) {
		_ = runDiscBounds(t, audio, sel, tier, cuts)
	})
}
