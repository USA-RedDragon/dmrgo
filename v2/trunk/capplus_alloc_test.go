//go:build !tinygo

package trunk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsCapacityPlusFollower(t *testing.T) {
	caps := readCapPlusCapture(t, "testdata/capplus_busy.txt")
	f, err := trunk.NewCapacityPlusFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	i := 0
	n := testing.AllocsPerRun(len(caps), func() {
		c := &caps[i%len(caps)]
		out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		i++
	})
	if n != 0 {
		t.Errorf("%v allocations per burst", n)
	}
}
