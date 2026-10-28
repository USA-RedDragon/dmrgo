package rf

import "testing"

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
	cfg := AudioRxConfig{Rate: hz, Tier: []Tier{Tier0, Tier1, Tier2, Tier3}[int(tier)%4]}
	r, err := NewAudioReceiver(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	cfg.OnBursts = func(bs []Burst) { seen += len(bs) }
	ro, err := NewAudioReceiver(&cfg)
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
		dst = r.AppendBursts(dst[:0], x[off:off+n])
		before := seen
		if _, err := ro.Write(x[off : off+n]); err != nil {
			t.Fatal(err)
		}
		if m := r.MaxOutput(n); len(dst) > m || seen-before > m || m > n/72+16 {
			t.Fatalf("%d Hz: %d samples gave %d and %d bursts, MaxOutput %d", hz, n, len(dst), seen-before, m)
		}
		total += len(dst)
		off += n
	}
	most := r.MaxOutput(0)
	dst = r.AppendFlush(dst[:0])
	before := seen
	if err := ro.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(dst) > most || seen-before > most || most > 26+r.unionBacklog() {
		t.Fatalf("%d Hz: flush gave %d and %d bursts, bound %d", hz, len(dst), seen-before, most)
	}
	if total += len(dst); seen != total {
		t.Fatalf("%d Hz: %d bursts appended, %d to OnBursts", hz, total, seen)
	}
	return total
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
