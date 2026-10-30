//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestCohFreqSlotType(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 8)
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 3140598, BS: 314926, Message: "DMRGO RXBENCH", BSSourced: true}
	hz := map[enums.DataType]float64{}
	for _, name := range []string{"data12", "data34"} {
		b, err := rftx.Sequence(name, cfg)
		if err != nil {
			t.Fatal(err)
		}
		var burst layer2.RawBurst
		var dt enums.DataType
		for _, s := range b.Slots {
			if x, err := layer2.NewBurstFromBytes(s.Burst); err == nil && x.Kind == layer2.KindData &&
				(x.SlotType.DataType == enums.DataTypeRate12 || x.SlotType.DataType == enums.DataTypeRate34) {
				burst, dt = s.Burst, x.SlotType.DataType
				break
			}
		}
		one := make([]rftx.BSSlot, len(slots))
		for i := range one {
			one[i] = rftx.BSSlot{CACH: slots[i].CACH, Burst: burst}
		}
		r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier2, Format: IQFormatCU8})
		x := cleanBaseband(one, 1400).noisy(40, 1)
		for k := 0; k < len(x); k += 4096 {
			must(r.writeBB(x[k:min(k+4096, len(x))]))
		}
		h, _ := r.full.(*rxHost)
		if h == nil || h.iqd == nil {
			t.Fatal("Tier 2 I/Q runs without the I/Q core")
		}
		e, _ := h.iqd.d.ext.(*softExt)
		if e == nil || e.coh == nil || !e.coh.freqOK {
			t.Fatalf("%s: no coherent frequency", name)
		}
		hz[dt] = e.coh.freq
	}
	for dt, f := range hz {
		if math.Abs(f-1400) > 0.5 {
			t.Errorf("%v bursts: coherent frequency %.2f Hz, carrier at 1400 Hz", dt, f)
		}
	}
}
