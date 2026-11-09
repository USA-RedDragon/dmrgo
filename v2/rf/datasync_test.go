//go:build !tinygo

package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataSyncRescue(t *testing.T) {
	t.Parallel()
	_, data, _ := sourceAt(0).SyncPatterns()
	var syms [syncSymbols]int8
	syncSyms(data, &syms)
	const cc = enums.ColorCode(7)
	for _, c := range []struct {
		name            string
		scale           float64
		flips           int
		cc              enums.ColorCode
		dt              enums.DataType
		unproven, takes bool
	}{
		{"weak sync", 1, 10, cc, enums.DataTypeCSBK, false, true},
		{"too many errors", 1, discAnchorBits + 1, cc, enums.DataTypeCSBK, false, false},
		{"flat carrier", 0.1, 10, cc, enums.DataTypeCSBK, false, false},
		{"other color code", 1, 10, cc + 1, enums.DataTypeCSBK, false, false},
		{"reserved data type", 1, 10, cc, enums.DataTypeReserved, false, false},
		{"unproven stream", 1, 10, cc, enums.DataTypeCSBK, true, false},
	} {
		r := &leanCore{ns: 600, sources: 1, devScale: 1, leanLock: leanLock{unproven: c.unproven}}
		r.votes[cc] = 3
		r.base = r.ns - slotSymbols
		for k := range syncSymbols {
			v := c.scale * deviationHz * float64(syms[k])
			if k%3 == 0 {
				v += 150
			}
			r.ring[ringPos(r.base+leanSyncEnd-syncSymbols+1+int64(k))] = int8(v / discStepHz)
		}
		b := &Burst{}
		if err := b.Data.SetSync(data); err != nil {
			t.Fatal(err)
		}
		for i := range c.flips {
			bit := framer.SyncOffset + 3*i + 1
			b.Data[bit/8] ^= 0x80 >> (bit % 8)
		}
		if err := b.Data.SetSlotType(pdu.SlotType{ColorCode: c.cc, DataType: c.dt}); err != nil {
			t.Fatal(err)
		}
		if got := r.dataSync(b); got != c.takes {
			t.Errorf("%s: rescue %t, want %t", c.name, got, c.takes)
		}
		if c.takes && (b.Sync != data || b.Distance != c.flips) {
			t.Errorf("%s: sync %v at distance %d, want %v at %d", c.name, b.Sync, b.Distance, data, c.flips)
		}
	}
}
