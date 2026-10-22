package lrrp_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

type appender interface {
	AppendBinary([]byte) ([]byte, error)
}

func TestLongitude180(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		east, west    appender
		over, overNeg appender
	}{
		{"Point2D", &lrrp.Point2D{Longitude: 180}, &lrrp.Point2D{Longitude: -180}, &lrrp.Point2D{Longitude: 180.0001}, &lrrp.Point2D{Longitude: -180.0001}},
		{"Circle2D", &lrrp.Circle2D{Longitude: 180}, &lrrp.Circle2D{Longitude: -180}, &lrrp.Circle2D{Longitude: 180.0001}, &lrrp.Circle2D{Longitude: -180.0001}},
		{"Point3D", &lrrp.Point3D{Longitude: 180}, &lrrp.Point3D{Longitude: -180}, &lrrp.Point3D{Longitude: 180.0001}, &lrrp.Point3D{Longitude: -180.0001}},
		{"Point3DWhole", &lrrp.Point3DWhole{Longitude: 180}, &lrrp.Point3DWhole{Longitude: -180}, &lrrp.Point3DWhole{Longitude: 180.0001}, &lrrp.Point3DWhole{Longitude: -180.0001}},
		{"Circle3DWhole", &lrrp.Circle3DWhole{Longitude: 180}, &lrrp.Circle3DWhole{Longitude: -180}, &lrrp.Circle3DWhole{Longitude: 180.0001}, &lrrp.Circle3DWhole{Longitude: -180.0001}},
		{"Point3DAccuracy", &lrrp.Point3DAccuracy{Longitude: 180}, &lrrp.Point3DAccuracy{Longitude: -180}, &lrrp.Point3DAccuracy{Longitude: 180.0001}, &lrrp.Point3DAccuracy{Longitude: -180.0001}},
		{"Circle3D", &lrrp.Circle3D{Longitude: 180}, &lrrp.Circle3D{Longitude: -180}, &lrrp.Circle3D{Longitude: 180.0001}, &lrrp.Circle3D{Longitude: -180.0001}},
	} {
		e, err := tt.east.AppendBinary(nil)
		if err != nil {
			t.Errorf("%s +180: %v", tt.name, err)
			continue
		}
		w, err := tt.west.AppendBinary(nil)
		if err != nil || !bytes.Equal(e, w) || !bytes.Equal(e[4:8], []byte{0x80, 0, 0, 0}) {
			t.Errorf("%s: +180 %x, -180 %x %v", tt.name, e, w, err)
		}
		for _, o := range []appender{tt.over, tt.overNeg} {
			if out, err := o.AppendBinary([]byte{7}); err == nil || len(out) != 1 {
				t.Errorf("%s outside [-180, 180]: %x %v", tt.name, out, err)
			}
		}
	}
	var m lrrp.Message
	if err := lrrp.DecodeMessageInto(&m, unhex(t, "070f2204010203046600000000"+"80000000")); err != nil {
		t.Fatal(err)
	}
	if p := m.Report.Point2D; !m.Report.HasPoint2D || p.Longitude != -180 {
		t.Fatalf("decoded %v", p)
	}
}
