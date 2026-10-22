package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestStatusServiceOptions_Table7_57(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		o    layer3.StatusServiceOptions
		want uint8
	}{
		{layer3.StatusServiceOptions{}, 0},
		{layer3.StatusServiceOptions{Group: true}, 0b100_0000},
		{layer3.StatusServiceOptions{IsSuppData: true}, 0b010_0000},
		{layer3.StatusServiceOptions{StatusHigh: 0b1_1111}, 0b001_1111},
		{layer3.StatusServiceOptions{Group: true, StatusHigh: 125 >> 2}, 0b101_1111},
	} {
		got, err := c.o.Field()
		if err != nil || got != c.want {
			t.Errorf("%+v: Field %07b %v, want %07b", c.o, got, err, c.want)
		}
		if back := layer3.StatusServiceOptionsOf(got); back != c.o {
			t.Errorf("%07b decodes as %+v, want %+v", got, back, c.o)
		}
	}
	if _, err := (layer3.StatusServiceOptions{StatusHigh: 32}).Field(); err == nil {
		t.Error("StatusHigh 32: no error")
	}
}

func FuzzStatusServiceOptions(f *testing.F) {
	f.Add(byte(0xFA))
	f.Fuzz(func(t *testing.T, b byte) {
		o := layer3.DecodeStatusServiceOptions([1]byte{b})
		_ = o.String()
		out, err := o.Encode()
		if err != nil || out[0] != b&0xFE {
			t.Fatalf("%02x encodes as %02x %v", b, out[0], err)
		}
	})
}
