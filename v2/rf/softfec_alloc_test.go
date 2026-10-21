package rf

import (
	"testing"
	"unsafe"
)

func TestAllocsSoftDecoders(t *testing.T) {
	d := newSoftDecoder()
	in := newSoftBenchInputs(t, 3)
	for _, c := range softBenchCases(d, in) {
		if n := testing.AllocsPerRun(20, c.run); n != 0 {
			t.Errorf("%s: %v allocations per decode, want 0", c.name, n)
		}
	}
	t.Logf("sizes in bytes: softDecoder %d, small codes %d, OSD codes %d, trellis %d, small scratch %d, OSD scratch %d",
		unsafe.Sizeof(*d), unsafe.Sizeof(softSmallCodes)+unsafe.Sizeof(softAMBESmallCodes), unsafe.Sizeof(softOSDCodes), unsafe.Sizeof(softTrellisTables),
		unsafe.Sizeof(d.small), unsafe.Sizeof(d.osd))
}
