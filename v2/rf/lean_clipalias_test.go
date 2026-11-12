package rf

import "testing"

func TestClipHeadNeverClippedFar(t *testing.T) {
	t.Parallel()
	for _, ns := range []int64{1040, 65576, 3*65536 + 40} {
		r := &leanCore{ns: ns}
		base := ns - 60
		if r.clipHead(base, 5) {
			t.Errorf("ns=%d: clipHead true with no clip ever", ns)
		}
		if r.levelCut(base, 0) {
			t.Errorf("ns=%d: levelCut true with no clip ever", ns)
		}
	}
}

func TestClipHeadAgedClipFar(t *testing.T) {
	t.Parallel()
	r := &leanCore{ns: 200000, clipNs: 200000 - 65536 - 30}
	base := r.ns - 60
	if r.clipHead(base, 5) || r.levelCut(base, 0) {
		t.Error("a clip a full counter wrap ago reads as recent")
	}
}

func TestClipHeadRecentClip(t *testing.T) {
	t.Parallel()
	r := &leanCore{ns: 70000}
	base := r.ns - 60
	r.clipNs = base + cachSymbols + 5
	if !r.clipHead(base, 5) {
		t.Error("clip in the SYNC window with a split not seen")
	}
	if r.clipHead(base, 0) {
		t.Error("clipHead without a split")
	}
	if !r.levelCut(base, 0) {
		t.Error("clip inside the burst not marked cut")
	}
}
