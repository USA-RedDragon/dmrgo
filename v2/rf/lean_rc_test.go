package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestLeanRCSigns(t *testing.T) {
	t.Parallel()
	var s [syncSymbols]int8
	syncSyms(enums.SyncMSSourcedRC, &s)
	var w uint32
	for k, v := range s {
		if v > 0 {
			w |= 1 << (syncSymbols - 1 - k)
		}
	}
	if w != leanRCSigns {
		t.Fatalf("RC signs %#x, want %#x", leanRCSigns, w)
	}
}
