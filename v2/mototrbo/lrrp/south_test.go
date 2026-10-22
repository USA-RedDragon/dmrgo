package lrrp_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

func TestSouthernLatitude(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		msg      string
		lat, lon float64
	}{
		{"070f22040102030466b02b40f66b86d022", -33.8688, 151.2093},
		{"070f22040102030466b136d29bd67bf43d", -34.6037, -58.3816},
	} {
		var m lrrp.Message
		if err := lrrp.DecodeMessageInto(&m, unhex(t, tt.msg)); err != nil {
			t.Fatal(err)
		}
		p := m.Report.Point2D
		if !m.Report.HasPoint2D || !near(p.Latitude, tt.lat, 1e-6) || !near(p.Longitude, tt.lon, 1e-6) {
			t.Errorf("%s: %v %v, want %v %v", tt.msg, p.Latitude, p.Longitude, tt.lat, tt.lon)
		}
		out, err := m.AppendBinary(nil)
		if err != nil || hex.EncodeToString(out) != tt.msg {
			t.Errorf("encode %x %v, want %s", out, err, tt.msg)
		}
	}
}
