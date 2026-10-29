package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestShortLCNotWorse(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the rxbench source directory")
	}
	d, err := load(filepath.Join(filepath.Dir(thisFile), "..", "..", "v2/rf/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.shortLC.Tiers) == 0 {
		t.Fatal("no Short LC tiers")
	}
	for _, tr := range d.shortLC.Tiers {
		h, l := tr.at90(tr.Hard), tr.at90(tr.LC)
		if !h.ok || !l.ok || l.ebn0 > h.ebn0 {
			t.Errorf("%s tier %d: OnShortLC 90%% at %v, hard decode at %v", tr.Mode, tr.Tier, l, h)
		}
	}
}
