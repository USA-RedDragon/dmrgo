package lip_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/lip"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

type field struct {
	n int
	v uint64
}

func pack(fs ...field) ([]byte, int) {
	var out []byte
	n := 0
	for _, f := range fs {
		for i := f.n - 1; i >= 0; i-- {
			if n%8 == 0 {
				out = append(out, 0)
			}
			if f.v>>i&1 == 1 {
				out[n/8] |= 0x80 >> (n % 8)
			}
			n++
		}
	}
	return out, n
}

func cat(groups ...[]field) []field {
	var out []field
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

const (
	lon25 = 0x0123456
	lat24 = 0xF6789A
)

var (
	lonDeg = float64(lon25) * 360 / (1 << 25)
	latDeg = float64(lat24-(1<<24)) * 180 / (1 << 24)
)

func TestLongReportShapes(t *testing.T) {
	t.Parallel()
	point := []field{{25, lon25}, {24, lat24}}
	alt := []field{{1, 1}, {11, 1201}}
	ellipse := []field{{6, 10}, {6, 7}, {8, 64}}
	tests := []struct {
		name   string
		loc    lip.Location
		fields []field
	}{
		{"none", lip.Location{Shape: lip.ShapeNone}, nil},
		{"point", lip.Location{Shape: lip.ShapePoint}, point},
		{"circle", lip.Location{Shape: lip.ShapeCircle, Uncertainty: 20}, cat(point, []field{{6, 20}})},
		{"ellipse", lip.Location{Shape: lip.ShapeEllipse, HalfMajor: 10, HalfMinor: 7, Angle: 64, Confidence: 4}, cat(point, ellipse, []field{{3, 4}})},
		{"point altitude", lip.Location{Shape: lip.ShapePointAltitude, AltitudeUserDefined: true, Altitude: 1201}, cat(point, alt)},
		{"circle altitude", lip.Location{Shape: lip.ShapeCircleAltitude, Uncertainty: 63, AltitudeUserDefined: true, Altitude: 1201}, cat(point, []field{{6, 63}}, alt)},
		{"ellipse altitude", lip.Location{Shape: lip.ShapeEllipseAltitude, HalfMajor: 10, HalfMinor: 7, Angle: 64, AltitudeUserDefined: true, Altitude: 1201, Confidence: 7}, cat(point, ellipse, alt, []field{{3, 7}})},
		{"circle altitude uncertainty", lip.Location{Shape: lip.ShapeCircleAltitudeUncertainty, Uncertainty: 1, AltitudeUserDefined: true, Altitude: 1201, AltitudeUncertainty: 5}, cat(point, []field{{6, 1}}, alt, []field{{3, 5}})},
		{"ellipse altitude uncertainty", lip.Location{Shape: lip.ShapeEllipseAltitudeUncertainty, HalfMajor: 10, HalfMinor: 7, Angle: 64, AltitudeUserDefined: true, Altitude: 1201, AltitudeUncertainty: 2, Confidence: 3}, cat(point, ellipse, alt, []field{{3, 2}, {3, 3}})},
		{"arc", lip.Location{Shape: lip.ShapeArc, InnerRadius: 100, OuterRadius: 65535, StartAngle: 32, StopAngle: 200, Confidence: 6}, cat(point, []field{{16, 100}, {16, 65535}, {8, 32}, {8, 200}, {3, 6}})},
		{"point position error", lip.Location{Shape: lip.ShapePointPositionError, PositionError: layer3.PositionAccuracy(5)}, cat(point, []field{{3, 5}})},
	}
	sizes := map[lip.Shape]int{0: 0, 1: 49, 2: 55, 3: 72, 4: 61, 5: 67, 6: 84, 7: 70, 8: 87, 9: 100, 10: 52}
	for _, tt := range tests {
		if tt.loc.Shape != lip.ShapeNone {
			tt.loc.Longitude, tt.loc.Latitude = lonDeg, latDeg
		}
		size := 0
		for _, f := range tt.fields {
			size += f.n
		}
		if size != sizes[tt.loc.Shape] {
			t.Fatalf("%s: vector has %d bits, Table 6.69 gives %d", tt.name, size, sizes[tt.loc.Shape])
		}
		r := lip.LongReport{Time: lip.Time{Type: lip.TimeElapsed, Elapsed: 2}, Location: tt.loc, AckRequested: true, AdditionalData: 32}
		want, wantBits := pack(cat([]field{{2, 1}, {4, 3}, {2, 1}, {2, 2}, {4, uint64(tt.loc.Shape)}}, tt.fields, []field{{3, 0}, {1, 1}, {1, 0}, {8, 32}})...)
		got, n, err := r.Encode(nil)
		if err != nil || n != wantBits || !bytes.Equal(got, want) {
			t.Errorf("%s: Encode = %x %d %v, want %x %d", tt.name, got, n, err, want, wantBits)
			continue
		}
		back, err := lip.DecodeLongReport(got)
		r.Elements = wantBits
		if err != nil || back != r {
			t.Errorf("%s: Decode = %+v %v, want %+v", tt.name, back, err, r)
		}
	}
}

func TestLongReportVelocityAndTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		v      lip.Velocity
		fields []field
	}{
		{"none", lip.Velocity{}, nil},
		{"horizontal", lip.Velocity{Type: lip.VelocityHorizontal, Horizontal: 127}, []field{{7, 127}}},
		{"uncertainty", lip.Velocity{Type: lip.VelocityHorizontalUncertainty, Horizontal: 28, HorizontalUncertainty: 3}, []field{{7, 28}, {3, 3}}},
		{"vertical", lip.Velocity{Type: lip.VelocityHorizontalVertical, Horizontal: 1, Down: true, Vertical: 56}, []field{{7, 1}, {1, 1}, {7, 56}}},
		{"vertical uncertain", lip.Velocity{Type: lip.VelocityHorizontalVerticalUncertain, Horizontal: 2, HorizontalUncertainty: 1, Vertical: 3, VerticalUncertainty: 4}, []field{{7, 2}, {3, 1}, {1, 0}, {7, 3}, {3, 4}}},
		{"direction", lip.Velocity{Type: lip.VelocityHorizontalDirection, Horizontal: 9, Direction: 192}, []field{{7, 9}, {8, 192}}},
		{"direction uncertain", lip.Velocity{Type: lip.VelocityHorizontalDirectionUncertain, Horizontal: 9, HorizontalUncertainty: 6, Direction: 192, DirectionUncertainty: 7}, []field{{7, 9}, {3, 6}, {8, 192}, {3, 7}}},
		{"all", lip.Velocity{Type: lip.VelocityAll, Horizontal: 5, HorizontalUncertainty: 1, Down: true, Vertical: 6, VerticalUncertainty: 2, Direction: 7, DirectionUncertainty: 3}, []field{{7, 5}, {3, 1}, {1, 1}, {7, 6}, {3, 2}, {8, 7}, {3, 3}}},
	}
	sizes := []int{0, 7, 10, 15, 21, 15, 21, 32}
	for _, tt := range tests {
		size := 0
		for _, f := range tt.fields {
			size += f.n
		}
		if size != sizes[tt.v.Type] {
			t.Fatalf("%s: vector has %d bits, Table 6.122 gives %d", tt.name, size, sizes[tt.v.Type])
		}
		r := lip.LongReport{
			Time:     lip.Time{Type: lip.TimePosition, Day: 31, Hour: 23, Minute: 59, Second: 58},
			Location: lip.Location{Shape: lip.ShapeNone}, Velocity: tt.v, UserDefined: true, AdditionalData: 0xA5,
		}
		want, wantBits := pack(cat([]field{{2, 1}, {4, 3}, {2, 2}, {5, 31}, {5, 23}, {6, 59}, {6, 58}, {4, 0}, {3, uint64(tt.v.Type)}}, tt.fields, []field{{1, 0}, {1, 1}, {8, 0xA5}})...)
		got, n, err := r.Encode(nil)
		if err != nil || n != wantBits || !bytes.Equal(got, want) {
			t.Errorf("%s: Encode = %x %d %v, want %x %d", tt.name, got, n, err, want, wantBits)
			continue
		}
		back, err := lip.DecodeLongReport(got)
		r.Elements = wantBits
		if err != nil || back != r {
			t.Errorf("%s: Decode = %+v %v, want %+v", tt.name, back, err, r)
		}
	}
	r := lip.LongReport{Location: lip.Location{Shape: lip.ShapeNone}}
	got, n, err := r.Encode(nil)
	if want, wantBits := pack(field{2, 1}, field{4, 3}, field{2, 0}, field{4, 0}, field{3, 0}, field{1, 0}, field{1, 0}, field{8, 0}); err != nil || n != wantBits || !bytes.Equal(got, want) {
		t.Errorf("no time: Encode = %x %d %v, want %x %d", got, n, err, want, wantBits)
	}
}

func TestCoordinatesMatchShortReport(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]float64{{0, 0}, {-33.8688, 151.2093}, {51.5007, -0.1246}, {-90, -180}, {90, 180}, {89.9999999, 179.9999999}, {-12.345678, -98.7654321}} {
		s := trunk.LIPShortReport{Latitude: c[0], Longitude: c[1], Velocity: 127}
		sb, err := s.Encode()
		if err != nil {
			t.Fatal(err)
		}
		l := lip.LongReport{Location: lip.Location{Shape: lip.ShapePoint, Latitude: c[0], Longitude: c[1]}}
		lb, _, err := l.Encode(nil)
		if err != nil {
			t.Fatal(err)
		}
		short := trunk.DecodeLIPShortReport(sb)
		long, err := lip.DecodeLongReport(lb)
		if err != nil || long.Location.Latitude != short.Latitude || long.Location.Longitude != short.Longitude {
			t.Errorf("%v: long %v %v %v, short %v %v", c, long.Location.Latitude, long.Location.Longitude, err, short.Latitude, short.Longitude)
		}
	}
}

func TestElements(t *testing.T) {
	t.Parallel()
	r := lip.LongReport{Location: lip.Location{Shape: lip.ShapePoint, Latitude: latDeg, Longitude: lonDeg}, AdditionalData: 32}
	pdu, n, err := r.Encode(nil)
	if err != nil {
		t.Fatal(err)
	}
	head := n
	pdu, n, err = lip.AppendElement(pdu, n, lip.ElementMessageReference, []byte{0x2A}, 8)
	if err != nil {
		t.Fatal(err)
	}
	pdu, n, err = lip.AppendElement(pdu, n, lip.ElementResultCode, []byte{0x01}, 8)
	if err != nil {
		t.Fatal(err)
	}
	long := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	pdu, n, err = lip.AppendElement(pdu, n, lip.ElementExtendedUserData, long, 70)
	if err != nil {
		t.Fatal(err)
	}
	prefix, _ := pack(cat([]field{{2, 1}, {4, 3}, {2, 0}, {4, 1}, {25, lon25}, {24, lat24}, {3, 0}, {1, 0}, {1, 0}, {8, 32}},
		[]field{{5, 6}, {6, 8}, {8, 0x2A}, {5, 12}, {6, 8}, {8, 1}, {5, 1}, {6, 0}, {7, 2}},
		[]field{{32, 0x01020304}, {32, 0x05060708}, {6, 9 >> 2}, {2, 3}})...)
	if want := head + 2*(11+8) + 18 + 72; n != want || !bytes.Equal(pdu, prefix) {
		t.Fatalf("AppendElement = %x %d, want %x %d", pdu, n, prefix, want)
	}
	back, err := lip.DecodeLongReport(pdu)
	if err != nil || back.Elements != head {
		t.Fatalf("Decode %+v %v", back, err)
	}
	at := back.Elements
	var got []lip.Element
	for {
		e, ok, err := lip.NextElement(pdu, at)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		got = append(got, e)
		at = e.Next
	}
	if len(got) != 3 || got[0].ID != lip.ElementMessageReference || got[0].Uint(pdu) != 0x2A || got[1].ID != lip.ElementResultCode || got[1].Uint(pdu) != 1 || got[2].Bits != 72 {
		t.Fatalf("elements %+v", got)
	}
	data := got[2].AppendData(nil, pdu)
	if want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 0x0B}; !bytes.Equal(data, want) {
		t.Errorf("AppendData = %x, want %x (70 bits and 1 fill bits)", data, want)
	}
	if v := got[2].Uint(pdu); v != 0x0102030405060708 {
		t.Errorf("Uint of a long element = %x", v)
	}
	padded := append(append([]byte{}, pdu...), 0, 0)
	if _, ok, err := lip.NextElement(padded, n); ok || err != nil {
		t.Errorf("zero octets after the PDU: %t %v", ok, err)
	}
}

func TestElementErrors(t *testing.T) {
	t.Parallel()
	ext, _ := pack(field{5, 31}, field{6, 1}, field{1, 0})
	if _, _, err := lip.NextElement(ext, 0); !errors.Is(err, lip.ErrElement) {
		t.Errorf("extended identifier: %v", err)
	}
	reserved, _ := pack(field{5, 2}, field{6, 0}, field{7, 0}, field{6, 0x3F})
	if _, _, err := lip.NextElement(reserved, 0); !errors.Is(err, lip.ErrElement) {
		t.Errorf("length extension 0: %v", err)
	}
	cut, _ := pack(field{5, 2}, field{6, 20}, field{5, 1})
	if _, _, err := lip.NextElement(cut, 0); !errors.Is(err, lip.ErrShort) {
		t.Errorf("element past the end: %v", err)
	}
	if _, _, err := lip.NextElement(cut, 99); !errors.Is(err, lip.ErrShort) {
		t.Errorf("at past the end: %v", err)
	}
	for _, c := range []struct {
		id   lip.ElementID
		data []byte
		n    int
	}{{31, []byte{1}, 1}, {1, []byte{1}, 0}, {1, []byte{1}, 9}, {1, make([]byte, 135), 1073}} {
		if _, _, err := lip.AppendElement(nil, 0, c.id, c.data, c.n); !errors.Is(err, lip.ErrElement) {
			t.Errorf("AppendElement(%d, %d): %v", c.id, c.n, err)
		}
	}
	if _, _, err := lip.AppendElement([]byte{0}, 9, 1, []byte{1}, 1); !errors.Is(err, lip.ErrShort) {
		t.Errorf("AppendElement past pdu: %v", err)
	}
	longest, n, err := lip.AppendElement(nil, 0, lip.ElementExtendedUserData, make([]byte, 134), 1072)
	if err != nil || n != 18+1072 || len(longest) != (n+7)/8 {
		t.Fatalf("1072 bits: %d %d %v", len(longest), n, err)
	}
	if e, ok, err := lip.NextElement(longest, 0); !ok || err != nil || e.Bits != 1072 {
		t.Errorf("1072 bits back: %+v %t %v", e, ok, err)
	}
}

func TestControlPDUs(t *testing.T) {
	t.Parallel()
	ack := lip.ReportAck{}
	got, n, err := ack.Encode(nil)
	if want, wantBits := pack(field{2, 1}, field{4, 4}, field{8, 0}); err != nil || n != wantBits || !bytes.Equal(got, want) {
		t.Errorf("ReportAck = %x %d %v, want %x", got, n, err, want)
	}
	if a, err := lip.DecodeReportAck(got); err != nil || a.Elements != 14 {
		t.Errorf("DecodeReportAck %+v %v", a, err)
	}
	q := lip.ImmediateRequest{ReportType: lip.ReportShort}
	got, n, err = q.Encode(nil)
	if want, wantBits := pack(field{2, 1}, field{4, 1}, field{1, 0}, field{2, 3}); err != nil || n != wantBits || !bytes.Equal(got, want) {
		t.Errorf("ImmediateRequest = %x %d %v, want %x", got, n, err, want)
	}
	got, n, _ = lip.AppendElement(got, n, lip.ElementMaxInformationAge, []byte{0x7F << 1}, 7)
	back, err := lip.DecodeImmediateRequest(got)
	if err != nil || back.ReportType != lip.ReportShort || back.Elements != 9 {
		t.Fatalf("DecodeImmediateRequest %+v %v", back, err)
	}
	if e, ok, err := lip.NextElement(got, back.Elements); !ok || err != nil || e.ID != lip.ElementMaxInformationAge || e.Uint(got) != 0x7F || e.Next != n {
		t.Errorf("maximum information age %+v %t %v", e, ok, err)
	}
	if _, _, err := (&lip.ImmediateRequest{ReportType: 4}).Encode(nil); !errors.As(err, new(*dmrgo.FieldRangeError)) {
		t.Errorf("ReportType 4: %v", err)
	}
	for _, e := range []lip.EnableDisable{{AckRequested: true, Flags: 0xC0}, {Response: true, Flags: 0x80}} {
		got, n, err := e.Encode(nil)
		fs := []field{{2, 1}, {4, 10}, {1, 0}, {1, 1}, {8, 0xC0}}
		if e.Response {
			fs = []field{{2, 1}, {4, 10}, {1, 1}, {8, 0x80}}
		}
		want, wantBits := pack(fs...)
		if err != nil || n != wantBits || !bytes.Equal(got, want) {
			t.Errorf("EnableDisable %+v = %x %d %v, want %x", e, got, n, err, want)
		}
		back, err := lip.DecodeEnableDisable(got)
		e.Elements = wantBits
		if err != nil || back != e {
			t.Errorf("DecodeEnableDisable %+v %v, want %+v", back, err, e)
		}
		h, err := lip.DecodeHeader(got)
		if err != nil || h != (lip.Header{Extension: lip.ExtensionEnableDisable, Response: e.Response}) {
			t.Errorf("DecodeHeader %+v %v", h, err)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	t.Parallel()
	if h, err := lip.DecodeHeader([]byte{0x00}); err != nil || !h.Short {
		t.Errorf("short report header %+v %v", h, err)
	}
	for _, b := range []byte{0x80, 0xC0} {
		if _, err := lip.DecodeHeader([]byte{b}); !errors.Is(err, lip.ErrPDUType) {
			t.Errorf("PDU type %d: %v", b>>6, err)
		}
	}
	if _, err := lip.DecodeHeader(nil); !errors.Is(err, lip.ErrShort) {
		t.Errorf("empty header: %v", err)
	}
	if h, err := lip.DecodeHeader([]byte{0x4C}); err != nil || h != (lip.Header{Extension: lip.ExtensionLongReport}) {
		t.Errorf("long report header %+v %v", h, err)
	}
	ack, _, _ := (&lip.ReportAck{}).Encode(nil)
	if _, err := lip.DecodeLongReport(ack); !errors.Is(err, lip.ErrPDUType) {
		t.Errorf("ack as long report: %v", err)
	}
	resp, _ := pack(field{2, 1}, field{4, 1}, field{1, 1}, field{2, 0})
	if _, err := lip.DecodeImmediateRequest(resp); !errors.Is(err, lip.ErrPDUType) {
		t.Errorf("response as request: %v", err)
	}
	full, _, _ := (&lip.LongReport{Location: lip.Location{Shape: lip.ShapeArc}}).Encode(nil)
	for i := range len(full) - 1 {
		if _, err := lip.DecodeLongReport(full[:i]); !errors.Is(err, lip.ErrShort) {
			t.Errorf("%d octets: %v", i, err)
		}
	}
	shape, _ := pack(field{2, 1}, field{4, 3}, field{2, 0}, field{4, 11}, field{40, 0})
	if _, err := lip.DecodeLongReport(shape); !errors.Is(err, lip.ErrShape) {
		t.Errorf("shape 11: %v", err)
	}
	tt, _ := pack(field{2, 1}, field{4, 3}, field{2, 3}, field{40, 0})
	if _, err := lip.DecodeLongReport(tt); !errors.Is(err, lip.ErrTimeType) {
		t.Errorf("time type 3: %v", err)
	}
}

func TestEncodeErrors(t *testing.T) {
	t.Parallel()
	var nilReport *lip.LongReport
	if _, _, err := nilReport.Encode(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil: %v", err)
	}
	bad := []lip.LongReport{
		{Time: lip.Time{Type: 3}},
		{Time: lip.Time{Type: lip.TimeElapsed, Elapsed: 4}},
		{Time: lip.Time{Type: lip.TimePosition, Second: 64}},
		{Location: lip.Location{Shape: 11}},
		{Location: lip.Location{Shape: lip.ShapePoint, Latitude: 91}},
		{Location: lip.Location{Shape: lip.ShapePoint, Longitude: -181}},
		{Location: lip.Location{Shape: lip.ShapeCircleAltitude, Altitude: 2048}},
		{Velocity: lip.Velocity{Type: 8}},
		{Velocity: lip.Velocity{Type: lip.VelocityHorizontal, Horizontal: 128}},
	}
	for i, r := range bad {
		if _, n, err := r.Encode([]byte{9}); err == nil || n != 0 {
			t.Errorf("%d: %+v encodes, %d", i, r, n)
		}
	}
	if !errors.Is(func() error { _, _, err := bad[3].Encode(nil); return err }(), lip.ErrShape) {
		t.Error("shape 11 is not ErrShape")
	}
}

func TestStrings(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]string{
		lip.ExtensionLongReport.String():             "Long location report",
		lip.Extension(13).String():                   "Extension(13)",
		lip.ShapeArc.String():                        "Location arc",
		lip.Shape(15).String():                       "Shape(15)",
		lip.VelocityAll.String():                     "Horizontal velocity and vertical velocity with direction of travel extended and uncertainty",
		lip.VelocityType(8).String():                 "VelocityType(8)",
		lip.TimeElapsed.String():                     "Time elapsed",
		lip.TimeType(3).String():                     "TimeType(3)",
		lip.ReportShort.String():                     "Short location report",
		lip.ReportType(4).String():                   "ReportType(4)",
		lip.ElementMessageReference.String():         "Location message reference",
		lip.ElementID(10).String():                   "ElementID(10)",
		lip.ElementID(31).String():                   "ElementID(31)",
		lip.ElementBacklogContinuation.String():      "Backlog continuation state",
		lip.ElementDirectionAccuracy.String():        "Direction of travel and direction of travel accuracy",
		lip.ExtensionImmediateReport.String():        "Immediate location report request",
		lip.Extension(0).String():                    "Extension(0)",
		lip.ElementVerticalAccuracy.String():         "Vertical velocity and vertical velocity accuracy",
		lip.ElementTemporaryControl.String():         "Temporary control parameter definition",
		lip.ExtensionBacklog.String():                "Backlog",
		lip.ElementPositionAccuracy.String():         "Horizontal position and horizontal position accuracy",
		lip.ShapeNone.String():                       "No shape",
		lip.TimeNone.String():                        "None",
		lip.TimePosition.String():                    "Time of position",
		lip.ReportLongNoTime.String():                "Long location report with no time",
		lip.ReportLongElapsed.String():               "Long location report with time elapsed",
		lip.ReportLongPosition.String():              "Long location report with time of position",
		lip.ElementExtendedUserData.String():         "Extended user defined data",
		lip.ExtensionTemporaryControl.String():       "Location reporting temporary control",
		lip.ShapePointPositionError.String():         "Location point and position error",
		lip.VelocityNone.String():                    "No velocity information",
		lip.ExtensionReportAck.String():              "Location report acknowledgement",
		lip.ElementStopTime.String():                 "Stop time",
		lip.Extension(2).String():                    "Extension(2)",
		lip.ElementBacklogAvailable.String():         "Backlog information available",
		lip.ElementTriggerDefinition.String():        "Trigger definition",
		lip.ExtensionReportBasicParams.String():      "Report basic location parameters",
		lip.ElementIdentification.String():           "Terminal or location identification",
		lip.ElementID(18).String():                   "ElementID(18)",
		lip.ExtensionAddModifyTrigger.String():       "Add/modify trigger",
		lip.ExtensionRemoveTrigger.String():          "Remove trigger",
		lip.ExtensionReportTrigger.String():          "Report trigger",
		lip.ExtensionBasicParameters.String():        "Basic location parameters",
		lip.ExtensionEnableDisable.String():          "Location reporting enable/disable",
		lip.ShapeEllipseAltitudeUncertainty.String(): "Location ellipse with altitude and altitude uncertainty",
	} {
		if s != want {
			t.Errorf("String = %q, want %q", s, want)
		}
	}
	for e := range lip.Extension(16) {
		want := e == lip.ExtensionImmediateReport || e >= lip.ExtensionBasicParameters && e <= lip.ExtensionBacklog
		if e.HasResponseBit() != want {
			t.Errorf("%v HasResponseBit = %t", e, !want)
		}
	}
}

func TestElementOutOfRange(t *testing.T) {
	t.Parallel()
	pdu := []byte{0xFF, 0xFF}
	for _, e := range []lip.Element{
		{Offset: 1 << 60, Bits: 8}, {Offset: 8, Bits: 1 << 60}, {Offset: 1<<63 - 1, Bits: 1<<63 - 1}, {Offset: -1, Bits: 8}, {Offset: 12, Bits: 5},
	} {
		if v := e.Uint(pdu); v != 0 {
			t.Errorf("%+v: Uint = %x", e, v)
		}
		if d := e.AppendData(nil, pdu); len(d) != 0 {
			t.Errorf("%+v: AppendData = %x", e, d)
		}
	}
}
