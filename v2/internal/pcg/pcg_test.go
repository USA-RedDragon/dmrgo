package pcg

import (
	"testing"
	"time"
)

func TestReferenceVector(t *testing.T) {
	t.Parallel()
	s := NewStream(42, 54)
	want := []uint32{0xa15c02b7, 0x7b47f409, 0xba1d3330, 0x83d2f293, 0xbfa4784b, 0xcbed606e}
	for i, w := range want {
		if got := s.Uint32(); got != w {
			t.Fatalf("value %d: %#x, want %#x", i, got, w)
		}
	}
}

func TestSeeds(t *testing.T) {
	t.Parallel()
	a, b, z := New(7), New(7), New(0)
	other := New(8)
	same, zeros := true, 0
	for range 64 {
		va := a.Uint32()
		if va != b.Uint32() {
			t.Fatal("seed 7 gave two sequences")
		}
		if va != other.Uint32() {
			same = false
		}
		if z.Uint32() == 0 {
			zeros++
		}
	}
	if same {
		t.Error("seeds 7 and 8 gave the same values")
	}
	if zeros > 1 {
		t.Errorf("seed 0 gave %d zeros in 64 values", zeros)
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()
	s := New(1)
	if s.Duration(0) != 0 || s.Duration(-time.Second) != 0 {
		t.Fatal("Duration of 0 or less is not 0")
	}
	var seen [4]bool
	for range 1000 {
		d := s.Duration(3)
		if d < 0 || d > 3 {
			t.Fatalf("Duration(3) = %v", d)
		}
		seen[d] = true
	}
	for i, ok := range seen {
		if !ok {
			t.Errorf("Duration(3) never gave %d", i)
		}
	}
	for range 1000 {
		if d := s.Duration(2 * time.Second); d < 0 || d > 2*time.Second {
			t.Fatalf("Duration(2s) = %v", d)
		}
	}
}
