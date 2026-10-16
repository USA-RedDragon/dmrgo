package rf

import (
	"testing"
	"unsafe"
)

// TestAllocsSoftDecoders checks that no decode allocates and logs the table sizes.
func TestAllocsSoftDecoders(t *testing.T) {
	d := newTestSoftDecoder(t)
	in := newSoftBenchInputs(t, 3)
	for _, c := range softBenchCases(d, in) {
		if n := testing.AllocsPerRun(20, c.run); n != 0 {
			t.Errorf("%s: %v allocations per decode, want 0", c.name, n)
		}
	}
	t.Logf("sizes in bytes: softDecoder %d, AMBE tables %d, BPTC code %d, Short LC code %d, embedded LC %d, trellis %d, small scratch %d, OSD scratch %d",
		unsafe.Sizeof(*d), unsafe.Sizeof(*d.ambe), unsafe.Sizeof(*d.bptc), unsafe.Sizeof(*d.shortLC), unsafe.Sizeof(*d.embLC.code),
		unsafe.Sizeof(*d.trellis), unsafe.Sizeof(d.small), unsafe.Sizeof(d.osd))
}
