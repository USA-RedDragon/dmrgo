package lrrp_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

// The reports of TestWholeShapes are the two that VIGIA
// (github.com/frandaless92/VIGIA at 8dc426d, serverws.js) replays from its
// deployment in Buenos Aires.
func TestWholeShapes(t *testing.T) {
	t.Parallel()

	m := decode(t, "0d19220431323334341fa5fb128053b13a24bdd67ac6c202804403")
	r := &m.Report
	c := r.Circle3DWhole
	if m.Document != lrrp.DocumentTriggeredReport || string(r.RequestID) != "1234" || !r.HasTime || r.Time.Year != 2025 || r.Time.Day != 29 ||
		!r.HasCircle3DWhole || !near(c.Latitude, -34.612821, 1e-6) || !near(c.Longitude, -58.388069, 1e-6) ||
		c.Radius != 2 || c.Altitude != 68 || c.AltitudeAccuracy != 3 {
		t.Errorf("circle-3d report %s", m.String())
	}
	m = decode(t, "0711341fa5ca6d3467b13a5784d67ab88a8042")
	p := m.Report.Point3DWhole
	if m.Document != lrrp.DocumentImmediateReport || !m.Report.HasPoint3DWhole ||
		!near(p.Latitude, -34.613366, 1e-6) || !near(p.Longitude, -58.388374, 1e-6) || p.Altitude != 66 {
		t.Errorf("point-3d report %s", m.String())
	}
}
