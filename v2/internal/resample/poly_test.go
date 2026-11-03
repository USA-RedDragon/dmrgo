package resample

import "testing"

func TestPolyLinear(t *testing.T) {
	t.Parallel()
	var p Poly64
	h := []float64{1, 0, 2.0 / 3, 1.0 / 3, 1.0 / 3, 2.0 / 3}
	if p.Init(h, 2, 3, 2, make([]float64, 3)) || p.Init(h, 2, 3, 4, make([]float64, 4)) || p.Init(h[:5], 2, 3, 2, make([]float64, 4)) {
		t.Fatal("Init took a short ring, m above l or a short table")
	}
	if !p.Init(h, 2, 3, 2, make([]float64, 4)) {
		t.Fatal("Init refused a valid table")
	}
	if p.MaxOut() != 2 || p.Push(1, make([]float64, 1)) != 0 {
		t.Fatal("Push took a dst shorter than MaxOut")
	}
	var got []float64
	dst := make([]float64, p.MaxOut())
	for i := range 6 {
		n := p.Push(float64(i), dst)
		got = append(got, dst[:n]...)
	}
	want := []float64{0, 0, 2.0 / 3, 2, 4.0 / 3, 8.0 / 3, 4, 10.0 / 3, 14.0 / 3}
	if len(got) != len(want) {
		t.Fatalf("got %d outputs, want %d", len(got), len(want))
	}
	for i := range want {
		if d := got[i] - want[i]; d > 1e-12 || d < -1e-12 {
			t.Fatalf("output %d: %v, want %v", i, got[i], want[i])
		}
	}
}
