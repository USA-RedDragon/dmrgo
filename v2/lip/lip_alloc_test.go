//go:build !tinygo

package lip_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/lip"
)

func TestAllocs(t *testing.T) {
	r := lip.LongReport{
		Time:     lip.Time{Type: lip.TimeElapsed},
		Location: lip.Location{Shape: lip.ShapeArc, Latitude: -1, Longitude: 2},
		Velocity: lip.Velocity{Type: lip.VelocityAll},
	}
	buf := make([]byte, 0, 64)
	data := make([]byte, 9)
	n := testing.AllocsPerRun(50, func() {
		p, bits, err := r.Encode(buf[:0])
		if err != nil {
			panic(err)
		}
		p, _, err = lip.AppendElement(p, bits, lip.ElementExtendedUserData, data, 70)
		if err != nil {
			panic(err)
		}
		l, err := lip.DecodeLongReport(p)
		if err != nil {
			panic(err)
		}
		e, ok, err := lip.NextElement(p, l.Elements)
		if !ok || err != nil {
			panic(err)
		}
		_ = e.AppendData(data[:0], p)
		if _, err := lip.DecodeHeader(p); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
