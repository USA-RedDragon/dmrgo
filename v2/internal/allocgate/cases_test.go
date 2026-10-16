//go:build !tinygo

package allocgate_test

import "testing"

// TestGateCases runs the analysis on testdata/cases, whose Alloc functions
// allocate in ways that the escape analysis of their own body does not show
// and whose NoAlloc functions do not allocate, and fails on each function
// that the gate classifies wrongly.
func TestGateCases(t *testing.T) {
	t.Parallel()

	gt := newGate(t, []string{"./internal/allocgate/testdata/cases"})
	n := 0
	for _, d := range gt.p.declaredFuncs() {
		name := d.fn.Name()
		want := hasPrefixWord(name, "Alloc")
		if !want && !hasPrefixWord(name, "NoAlloc") {
			continue
		}
		n++
		if got := gt.allocates(d.sym); got != want {
			t.Errorf("%s: allocates is %v, want %v%s", name, got, want, gt.chain(gt.next, d.sym))
		}
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}

// TestDstGrowthCases runs the dst growth rule on testdata/growth, whose Grow
// functions allocate only by growing dst and whose Bad functions can grow
// another slice, or call code that the gate cannot follow, and fails on
// each function that the rule classifies wrongly.
func TestDstGrowthCases(t *testing.T) {
	t.Parallel()

	gt := newGate(t, []string{"./internal/allocgate/testdata/growth"})
	n := 0
	for _, d := range gt.p.declaredFuncs() {
		name := d.fn.Name()
		want := hasPrefixWord(name, "Grow")
		if !want && !hasPrefixWord(name, "Bad") {
			continue
		}
		n++
		if !gt.allocates(d.sym) {
			t.Errorf("%s: does not allocate, so the case tests nothing", name)
			continue
		}
		got, why := gt.growsDst(root{d.sym, &d})
		switch {
		case got != want:
			t.Errorf("%s: dst growth is %v, want %v: %s", name, got, want, why)
		case !got && why == "":
			t.Errorf("%s: rejected without a reason", name)
		default:
			t.Logf("%s: %v %s", name, got, why)
		}
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}
