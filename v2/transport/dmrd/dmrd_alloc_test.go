package dmrd_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestAllocsData(t *testing.T) {
	p, _ := hex.DecodeString(dmrdVectors()[1].hex)
	buf := make([]byte, 0, 80)
	n := allocstest.PerRun(t, 100, func() {
		d, _ := dmrd.DecodeData(p)
		buf, _ = d.AppendBinary(buf[:0])
		_ = d.Hint(0)
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
