package lrrp_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func near(a, b, eps float64) bool { return math.Abs(a-b) < eps }

var specReport = "071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006"

func decode(t *testing.T, s string) *lrrp.Message {
	t.Helper()
	data := unhex(t, s)
	m := new(lrrp.Message)
	if err := lrrp.DecodeMessageInto(m, data); err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Errorf("%s encodes to %x, %v", s, out, err)
	}
	return m
}

func TestSpecReport(t *testing.T) {
	t.Parallel()

	m := decode(t, specReport)
	r := &m.Report
	if m.Document != lrrp.DocumentImmediateReport || !r.HasRequestID || hex.EncodeToString(r.RequestID) != "2468ace0" || !r.HasTime || !r.HasCircle2D || !r.HasSpeed {
		t.Fatalf("got %s", m.String())
	}
	if r.Time != (lrrp.Time{Year: 2003, Month: 6, Day: 30, Hour: 7, Minute: 30}) {
		t.Errorf("time %s", r.Time.String())
	}
	c := r.Circle2D
	if !near(c.Latitude, 12.345345, 1e-6) || !near(c.Longitude, 24.668866, 1e-6) || c.Radius != 99.0/128 || r.Speed != 6.0/128 {
		t.Errorf("circle %s speed %v", c.String(), r.Speed)
	}
}

func TestSpecDocuments(t *testing.T) {
	t.Parallel()

	m := decode(t, "070c22042468ace0390503515355")
	if !m.Report.HasError || m.Report.Error.Code != 5 || string(m.Report.Error.Info) != "QSU" {
		t.Errorf("error report %s", m.String())
	}
	m = decode(t, "090922042468ace034313c")
	if !m.Request.Periodic || !m.Request.HasInterval || m.Request.Interval != 60 {
		t.Errorf("triggered request %s", m.String())
	}
	m = decode(t, "0d0f22042468ace066118ecd8d118ad47b")
	if !m.Report.HasPoint2D || !near(m.Report.Point2D.Latitude, 12.345345, 1e-6) {
		t.Errorf("triggered report %s", m.String())
	}
	m = decode(t, "0f0622042468ace0")
	if m.Document != lrrp.DocumentStopRequest || !m.Request.HasRequestID {
		t.Errorf("stop request %s", m.String())
	}
	m = decode(t, "110722042468ace038")
	if !m.Report.Success {
		t.Errorf("stop answer %s", m.String())
	}
}

func TestTRBOnetRequests(t *testing.T) {
	t.Parallel()

	m := decode(t, "05092203000007"+"51546257")
	q := m.Request
	if !q.AccuracyAndTime || !q.Altitude || !q.Speed || !q.Direction || hex.EncodeToString(q.RequestID) != "000007" {
		t.Errorf("immediate request %s", m.String())
	}
	m = decode(t, "090c2203000003"+"3431817a788374")
	if !m.Request.Periodic || m.Request.Interval != 250 || m.Request.Distance != 500 {
		t.Errorf("distance request %s", m.String())
	}
	m = decode(t, "09082203000002"+"344a02")
	if m.Request.TriggerCondition != lrrp.TriggerEmergency {
		t.Errorf("emergency request %s", m.String())
	}
}

func TestBuildReport(t *testing.T) {
	t.Parallel()

	m := &lrrp.Message{Document: lrrp.DocumentTriggeredReport}
	r := &m.Report
	r.RequestID, r.HasRequestID = []byte{1}, true
	r.Point3D, r.HasPoint3D = lrrp.Point3D{Latitude: -33.5, Longitude: -70.25, Altitude: -12.5}, true
	r.VerticalSpeed, r.HasVerticalSpeed = -0.5, true
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	back := new(lrrp.Message)
	if err := lrrp.DecodeMessageInto(back, out); err != nil {
		t.Fatal(err)
	}
	p := back.Report.Point3D
	if !back.Report.HasPoint3D || !near(p.Latitude, -33.5, 1e-7) || !near(p.Longitude, -70.25, 1e-7) || p.Altitude != -12.5 || back.Report.VerticalSpeed != -0.5 {
		t.Errorf("round trip %s", back.String())
	}
	r.Point3D.Latitude = 91
	if _, err := m.AppendBinary(nil); err == nil {
		t.Error("latitude 91 encoded")
	}
}

func TestSouthernLatitudeEncode(t *testing.T) {
	t.Parallel()

	m := decode(t, "0d0f22042468ace066"+"cfd4bf0a"+"6b86d022")
	if p := m.Report.Point2D; !near(p.Latitude, -56.1312, 1e-4) || !near(p.Longitude, 151.2093, 1e-6) {
		t.Errorf("sign and magnitude %s", m.String())
	}
	m = &lrrp.Message{Document: lrrp.DocumentTriggeredReport}
	m.Report.Point2D, m.Report.HasPoint2D = lrrp.Point2D{Latitude: -33.8688, Longitude: -43.2}, true
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "0d0966"+"b02b40f6"+"e147ae14"); !bytes.Equal(out, want) {
		t.Errorf("-33.8688,-43.2 encodes to %x, want %x", out, want)
	}
	for _, lat := range []float64{90, -90} {
		m.Report.Point2D.Latitude = lat
		out, err := m.AppendBinary(nil)
		if err != nil {
			t.Fatalf("latitude %v: %v", lat, err)
		}
		back := new(lrrp.Message)
		if err := lrrp.DecodeMessageInto(back, out); err != nil || !near(back.Report.Point2D.Latitude, lat, 1e-7) {
			t.Errorf("latitude %v comes back as %v, %v", lat, back.Report.Point2D.Latitude, err)
		}
	}
}

func TestToken6B(t *testing.T) {
	t.Parallel()

	m := new(lrrp.Message)
	if err := lrrp.DecodeMessageInto(m, unhex(t, "1315232f341f4ad07b2e66474326660a4d56e46b0b5620")); err != nil {
		t.Fatal(err)
	}
	r := &m.Report
	if !r.HasPoint2D || !near(r.Point2D.Latitude, 50.106308, 1e-6) || !r.HasSpeedWhole || r.SpeedWhole != 11 || !r.HasDirection || r.Direction != 0x20 {
		t.Errorf("report %s", m.String())
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	data := unhex(t, specReport)
	for _, tt := range []struct {
		data []byte
		want error
	}{
		{nil, lrrp.ErrShort},
		{data[:10], lrrp.ErrShort},
		{append(append([]byte{}, data...), 0), lrrp.ErrLength},
		{[]byte{0x07, 0x01, 0x99}, lrrp.ErrToken},
		{[]byte{0x07, 0x02, 0x36, 0xFF}, lrrp.ErrShort},
		{[]byte{0x07, 0x07, 0x36, 0x8F, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F}, lrrp.ErrVarint},
	} {
		m := lrrp.Message{Document: 3}
		if err := lrrp.DecodeMessageInto(&m, tt.data); !errors.Is(err, tt.want) || m.Document != 0 {
			t.Errorf("%x: %v, want %v", tt.data, err, tt.want)
		}
	}
	if err := lrrp.DecodeMessageInto(nil, data); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil m: %v", err)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if lrrp.DocumentTriggeredReport.String() != "Triggered Location Report" || lrrp.DocumentID(2).String() != "DocumentID(2)" {
		t.Error("names")
	}
	for r, want := range map[lrrp.ResultCode]string{
		lrrp.ResultSuccess: "Success", lrrp.ResultInvalidCommand: "Invalid Command", lrrp.ResultNoGPS: "No GPS",
		lrrp.ResultDuplicateRequest: "Duplicate Request", lrrp.ResultGPSInitializing: "GPS Initializing", 5: "ResultCode(5)",
	} {
		if r.String() != want {
			t.Errorf("%d: %s", r, r.String())
		}
	}
	if lrrp.TriggerEmergency.String() != "Emergency" || lrrp.TriggerCondition(1).String() != "TriggerCondition(1)" {
		t.Error("TriggerCondition names")
	}
}
