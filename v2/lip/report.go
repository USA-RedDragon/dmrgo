package lip

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// TimeType is the kind of the time data of a long location report (ETSI
// TS 100 392-18-1 clause 6.3.81).
type TimeType uint8

// The TimeType values. 3 is reserved.
const (
	TimeNone     TimeType = 0 // no time
	TimeElapsed  TimeType = 1 // Time.Elapsed
	TimePosition TimeType = 2 // Time.Day to Time.Second
)

// String returns the name of the TimeType. It allocates only for a
// reserved value.
func (t TimeType) String() string {
	switch t {
	case TimeNone:
		return "None"
	case TimeElapsed:
		return "Time elapsed"
	case TimePosition:
		return "Time of position"
	}
	return "TimeType(" + text.Int(int64(t)) + ")"
}

// Time is the time data of a long location report: the time elapsed since
// the position was found, or the time of the position. Only the fields of
// Type are coded. (ETSI TS 100 392-18-1 clauses 6.3.78 to 6.3.80)
type Time struct {
	// Type tells which fields are coded.
	Type TimeType
	// Elapsed is the time elapsed code for TimeElapsed: 0 under 5 s, 1
	// under 5 min, 2 under 30 min, 3 not known.
	Elapsed uint8
	// Day is the day of the month, 1 to 31, for TimePosition.
	Day uint8
	// Hour is the UTC hour, 0 to 23, for TimePosition.
	Hour uint8
	// Minute is the minute, 0 to 59, for TimePosition.
	Minute uint8
	// Second is the second, 0 to 59, for TimePosition.
	Second uint8
}

func (t *Time) read(r *reader) {
	t.Type = TimeType(r.u8(2))
	switch t.Type {
	case TimeElapsed:
		t.Elapsed = r.u8(2)
	case TimePosition:
		t.Day, t.Hour, t.Minute, t.Second = r.u8(5), r.u8(5), r.u8(6), r.u8(6)
	case TimeNone:
	default:
		r.fail(ErrTimeType)
	}
}

func (t *Time) write(w *writer) {
	w.put(2, uint64(t.Type), ErrTimeType)
	switch t.Type {
	case TimeNone:
	case TimeElapsed:
		w.put(2, uint64(t.Elapsed), errTimeElapsed)
	case TimePosition:
		w.put(5, uint64(t.Day), errTimeDay)
		w.put(5, uint64(t.Hour), errTimeHour)
		w.put(6, uint64(t.Minute), errTimeMinute)
		w.put(6, uint64(t.Second), errTimeSecond)
	default:
		w.fail(ErrTimeType)
	}
}

// Shape is the location shape of the location data. Values that have no
// constant are reserved, and 15 is the location shape extension, which
// the spec does not define (ETSI TS 100 392-18-1 clause 6.3.49).
type Shape uint8

// The Shape values.
const (
	ShapeNone                       Shape = 0  // no position
	ShapePoint                      Shape = 1  // longitude and latitude
	ShapeCircle                     Shape = 2  // a point and Uncertainty
	ShapeEllipse                    Shape = 3  // a point, the axes, Angle and Confidence
	ShapePointAltitude              Shape = 4  // a point and the altitude
	ShapeCircleAltitude             Shape = 5  // a circle and the altitude
	ShapeEllipseAltitude            Shape = 6  // an ellipse and the altitude
	ShapeCircleAltitudeUncertainty  Shape = 7  // a circle, the altitude and AltitudeUncertainty
	ShapeEllipseAltitudeUncertainty Shape = 8  // an ellipse, the altitude and AltitudeUncertainty
	ShapeArc                        Shape = 9  // a point, the radii, the angles and Confidence
	ShapePointPositionError         Shape = 10 // a point and PositionError
)

// String returns the name of the Shape. It allocates only for a reserved
// value.
func (s Shape) String() string {
	switch s {
	case ShapeNone:
		return "No shape"
	case ShapePoint:
		return "Location point"
	case ShapeCircle:
		return "Location circle"
	case ShapeEllipse:
		return "Location ellipse"
	case ShapePointAltitude:
		return "Location point with altitude"
	case ShapeCircleAltitude:
		return "Location circle with altitude"
	case ShapeEllipseAltitude:
		return "Location ellipse with altitude"
	case ShapeCircleAltitudeUncertainty:
		return "Location circle with altitude and altitude uncertainty"
	case ShapeEllipseAltitudeUncertainty:
		return "Location ellipse with altitude and altitude uncertainty"
	case ShapeArc:
		return "Location arc"
	case ShapePointPositionError:
		return "Location point and position error"
	}
	return "Shape(" + text.Int(int64(s)) + ")"
}

// Location is the location data of a long location report: the shape and
// the fields that it codes. The other fields are zero after a decode and
// ignored by an encode. Uncertainty, HalfMajor and HalfMinor are
// horizontal position uncertainty codes, 0 under 1 m to 62 under 404 km
// and 63 best effort; Angle, StartAngle and StopAngle are in steps of
// 360/256 degrees clockwise from north. (ETSI TS 100 392-18-1 clauses
// 6.3.35 to 6.3.47)
type Location struct {
	// Shape tells which fields are coded.
	Shape Shape
	// Longitude is in degrees, -180 to 180, east positive, in steps of
	// 360/2^25 degrees. Every shape but ShapeNone codes it.
	Longitude float64
	// Latitude is in degrees, -90 to 90, north positive, in steps of
	// 180/2^24 degrees. Every shape but ShapeNone codes it.
	Latitude float64
	// Uncertainty is the horizontal position uncertainty of the circle
	// shapes.
	Uncertainty uint8
	// HalfMajor is half of the major axis of the ellipse shapes.
	HalfMajor uint8
	// HalfMinor is half of the minor axis of the ellipse shapes.
	HalfMinor uint8
	// Angle is the angle of the major axis of the ellipse shapes.
	Angle uint8
	// Confidence is the confidence level of the ellipse and arc shapes: 0
	// for 50 %, 1 68 %, 2 80 %, 3 90 %, 4 95 %, 5 99 %, 6 99.9 % and 7 not
	// known.
	Confidence uint8
	// AltitudeUserDefined is the location altitude type of the shapes with
	// an altitude: true for a user defined reference, false for the WGS84
	// ellipsoid.
	AltitudeUserDefined bool
	// Altitude is the altitude code of the shapes with an altitude: 1 is
	// -200 m, in steps of 1 m to 1201 (1000 m), 2 m to 1926 (2450 m) and
	// 75 m to 2047 (11525 m or more). 0 is reserved.
	Altitude uint16
	// AltitudeUncertainty is the location altitude uncertainty code of
	// the shapes with an altitude uncertainty.
	AltitudeUncertainty uint8
	// InnerRadius is the inner radius of ShapeArc in steps of 2 m.
	InnerRadius uint16
	// OuterRadius is the outer radius of ShapeArc in steps of 2 m.
	OuterRadius uint16
	// StartAngle is the start angle of ShapeArc.
	StartAngle uint8
	// StopAngle is the stop angle of ShapeArc.
	StopAngle uint8
	// PositionError is the position error of ShapePointPositionError.
	PositionError layer3.PositionAccuracy
}

func (l *Location) read(r *reader) {
	l.Shape = Shape(r.u8(4))
	if l.Shape > ShapePointPositionError {
		r.fail(ErrShape)
		return
	}
	if l.Shape == ShapeNone {
		return
	}
	l.Longitude = r.coordinate(25, 360)
	l.Latitude = r.coordinate(24, 180)
	switch {
	case l.Shape.circle():
		l.Uncertainty = r.u8(6)
	case l.Shape.ellipse():
		l.HalfMajor, l.HalfMinor, l.Angle = r.u8(6), r.u8(6), r.u8(8)
	case l.Shape == ShapeArc:
		l.InnerRadius, l.OuterRadius = r.u16(16), r.u16(16)
		l.StartAngle, l.StopAngle, l.Confidence = r.u8(8), r.u8(8), r.u8(3)
	case l.Shape == ShapePointPositionError:
		l.PositionError = layer3.PositionAccuracy(r.u8(3))
	}
	if l.Shape.altitude() {
		l.AltitudeUserDefined = r.flag()
		l.Altitude = r.u16(11)
	}
	if l.Shape.altitudeUncertainty() {
		l.AltitudeUncertainty = r.u8(3)
	}
	if l.Shape.ellipse() {
		l.Confidence = r.u8(3)
	}
}

func (l *Location) write(w *writer) {
	if l.Shape > ShapePointPositionError {
		w.fail(ErrShape)
		return
	}
	w.put(4, uint64(l.Shape), nil)
	if l.Shape == ShapeNone {
		return
	}
	if !(l.Longitude >= -180 && l.Longitude <= 180) {
		w.fail(errLongitude)
		return
	}
	if !(l.Latitude >= -90 && l.Latitude <= 90) {
		w.fail(errLatitude)
		return
	}
	w.put(25, twos(math.Round(l.Longitude*(1<<25)/360.0), 25), nil)
	w.put(24, twos(min(math.Round(l.Latitude*(1<<24)/180.0), 1<<23-1), 24), nil)
	switch {
	case l.Shape.circle():
		w.put(6, uint64(l.Uncertainty), errUncertainty)
	case l.Shape.ellipse():
		w.put(6, uint64(l.HalfMajor), errHalfMajor)
		w.put(6, uint64(l.HalfMinor), errHalfMinor)
		w.put(8, uint64(l.Angle), nil)
	case l.Shape == ShapeArc:
		w.put(16, uint64(l.InnerRadius), nil)
		w.put(16, uint64(l.OuterRadius), nil)
		w.put(8, uint64(l.StartAngle), nil)
		w.put(8, uint64(l.StopAngle), nil)
		w.put(3, uint64(l.Confidence), errConfidence)
	case l.Shape == ShapePointPositionError:
		w.put(3, uint64(l.PositionError), errPosition)
	}
	if l.Shape.altitude() {
		w.flag(l.AltitudeUserDefined)
		w.put(11, uint64(l.Altitude), errAltitude)
	}
	if l.Shape.altitudeUncertainty() {
		w.put(3, uint64(l.AltitudeUncertainty), errAltitudeUnc)
	}
	if l.Shape.ellipse() {
		w.put(3, uint64(l.Confidence), errConfidence)
	}
}

func (s Shape) circle() bool {
	return s == ShapeCircle || s == ShapeCircleAltitude || s == ShapeCircleAltitudeUncertainty
}

func (s Shape) ellipse() bool {
	return s == ShapeEllipse || s == ShapeEllipseAltitude || s == ShapeEllipseAltitudeUncertainty
}

func (s Shape) altitude() bool {
	return s == ShapePointAltitude || s == ShapeCircleAltitude || s == ShapeEllipseAltitude || s.altitudeUncertainty()
}

func (s Shape) altitudeUncertainty() bool {
	return s == ShapeCircleAltitudeUncertainty || s == ShapeEllipseAltitudeUncertainty
}

func twos(v float64, n int) uint64 {
	if v < 0 {
		v += float64(uint64(1) << n)
	}
	return uint64(v) & (1<<n - 1)
}

// VelocityType is the kind of the velocity data of a long location report
// (ETSI TS 100 392-18-1 clause 6.3.90).
type VelocityType uint8

// The VelocityType values.
const (
	VelocityNone                         VelocityType = 0 // no velocity
	VelocityHorizontal                   VelocityType = 1 // Horizontal
	VelocityHorizontalUncertainty        VelocityType = 2 // Horizontal and its uncertainty
	VelocityHorizontalVertical           VelocityType = 3 // Horizontal and Vertical
	VelocityHorizontalVerticalUncertain  VelocityType = 4 // Horizontal, Vertical and their uncertainties
	VelocityHorizontalDirection          VelocityType = 5 // Horizontal and Direction
	VelocityHorizontalDirectionUncertain VelocityType = 6 // Horizontal, Direction and their uncertainties
	VelocityAll                          VelocityType = 7 // Horizontal, Vertical, Direction and their uncertainties
)

// String returns the name of the VelocityType. It allocates only for a
// value over 7.
func (t VelocityType) String() string {
	switch t {
	case VelocityNone:
		return "No velocity information"
	case VelocityHorizontal:
		return "Horizontal velocity"
	case VelocityHorizontalUncertainty:
		return "Horizontal velocity with uncertainty"
	case VelocityHorizontalVertical:
		return "Horizontal velocity and vertical velocity"
	case VelocityHorizontalVerticalUncertain:
		return "Horizontal velocity and vertical velocity with uncertainty"
	case VelocityHorizontalDirection:
		return "Horizontal velocity with direction of travel extended"
	case VelocityHorizontalDirectionUncertain:
		return "Horizontal velocity with direction of travel extended and uncertainty"
	case VelocityAll:
		return "Horizontal velocity and vertical velocity with direction of travel extended and uncertainty"
	}
	return "VelocityType(" + text.Int(int64(t)) + ")"
}

// Velocity is the velocity data of a long location report: the velocity
// type and the fields that it codes. The other fields are zero after a
// decode and ignored by an encode. Horizontal and Vertical are velocity
// codes: 0 to 28 km/h in steps of 1 km/h, then 16 x 1.038^(code-13) km/h,
// 127 for not known. The uncertainties are codes 0 under 1.5 km/h (or
// degrees) to 6 under 96, 7 not known. (ETSI TS 100 392-18-1 clauses 6.3.17
// to 6.3.26, 6.3.89 and 6.3.91)
type Velocity struct {
	// Type tells which fields are coded.
	Type VelocityType
	// Horizontal is the horizontal velocity code. Every type but
	// VelocityNone codes it.
	Horizontal uint8
	// HorizontalUncertainty is the horizontal velocity uncertainty of the
	// types with uncertainties.
	HorizontalUncertainty uint8
	// Down is the vertical velocity sign, true for downwards, of the types
	// with a vertical velocity.
	Down bool
	// Vertical is the vertical velocity code of the types with a vertical
	// velocity.
	Vertical uint8
	// VerticalUncertainty is the vertical velocity uncertainty of
	// VelocityHorizontalVerticalUncertain and VelocityAll.
	VerticalUncertainty uint8
	// Direction is the direction of travel extended, in steps of 360/256
	// degrees clockwise from north, of the types with a direction.
	Direction uint8
	// DirectionUncertainty is the direction of travel uncertainty of
	// VelocityHorizontalDirectionUncertain and VelocityAll.
	DirectionUncertainty uint8
}

func (v *Velocity) uncertain() bool {
	return v.Type == VelocityHorizontalUncertainty || v.Type == VelocityHorizontalVerticalUncertain || v.Type == VelocityHorizontalDirectionUncertain || v.Type == VelocityAll
}

func (v *Velocity) vertical() bool {
	return v.Type == VelocityHorizontalVertical || v.Type == VelocityHorizontalVerticalUncertain || v.Type == VelocityAll
}

func (v *Velocity) direction() bool {
	return v.Type == VelocityHorizontalDirection || v.Type == VelocityHorizontalDirectionUncertain || v.Type == VelocityAll
}

func (v *Velocity) read(r *reader) {
	v.Type = VelocityType(r.u8(3))
	if v.Type == VelocityNone {
		return
	}
	v.Horizontal = r.u8(7)
	if v.uncertain() {
		v.HorizontalUncertainty = r.u8(3)
	}
	if v.vertical() {
		v.Down = r.flag()
		v.Vertical = r.u8(7)
		if v.uncertain() {
			v.VerticalUncertainty = r.u8(3)
		}
	}
	if v.direction() {
		v.Direction = r.u8(8)
		if v.uncertain() {
			v.DirectionUncertainty = r.u8(3)
		}
	}
}

func (v *Velocity) write(w *writer) {
	w.put(3, uint64(v.Type), errVelocityType)
	if v.Type == VelocityNone {
		return
	}
	w.put(7, uint64(v.Horizontal), errHorizontal)
	if v.uncertain() {
		w.put(3, uint64(v.HorizontalUncertainty), errHorizUnc)
	}
	if v.vertical() {
		w.flag(v.Down)
		w.put(7, uint64(v.Vertical), errVertical)
		if v.uncertain() {
			w.put(3, uint64(v.VerticalUncertainty), errVerticalUnc)
		}
	}
	if v.direction() {
		w.put(8, uint64(v.Direction), nil)
		if v.uncertain() {
			w.put(3, uint64(v.DirectionUncertainty), errDirectionUnc)
		}
	}
}

// LongReport is the LONG LOCATION REPORT PDU: the time, location and
// velocity data and the additional data. Its type 5 elements, such as the
// location message reference and the result code, follow from bit
// Elements; read them with NextElement and write them with AppendElement.
// (ETSI TS 100 392-18-1 clause 6.2.2)
type LongReport struct {
	// Time is the time data.
	Time Time
	// Location is the location data.
	Location Location
	// Velocity is the velocity data.
	Velocity Velocity
	// AckRequested asks for a location report acknowledgement.
	AckRequested bool
	// UserDefined is the type of additional data: true when
	// AdditionalData is user defined data, false for the reason for
	// sending.
	UserDefined bool
	// AdditionalData is the reason for sending or the user defined data.
	AdditionalData uint8
	// Elements is the bit offset of the first type 5 element.
	// DecodeLongReport sets it; Encode ignores it.
	Elements int
}

// DecodeLongReport decodes the LONG LOCATION REPORT PDU pdu. It returns
// ErrPDUType for another PDU, ErrShort for a PDU that ends early, and
// ErrTimeType and ErrShape for a reserved time type or location shape.
// (ETSI TS 100 392-18-1 clause 6.2.2)
func DecodeLongReport(pdu []byte) (LongReport, error) {
	r := reader{b: pdu}
	r.start(ExtensionLongReport, -1)
	var l LongReport
	l.Time.read(&r)
	l.Location.read(&r)
	l.Velocity.read(&r)
	l.AckRequested = r.flag()
	l.UserDefined = r.flag()
	l.AdditionalData = r.u8(8)
	if r.err != nil {
		return LongReport{}, r.err
	}
	l.Elements = r.at
	return l, nil
}

// Encode appends the PDU of l to dst, with 0 bits after its last bit, and
// returns dst and the number of bits of the PDU. It returns a
// *dmrgo.FieldRangeError for a field that does not fit, ErrTimeType and
// ErrShape for a reserved time type or location shape, and
// dmrgo.ErrNilReceiver for a nil l. It allocates only to grow dst. (ETSI
// TS 100 392-18-1 clause 6.2.2)
func (l *LongReport) Encode(dst []byte) ([]byte, int, error) {
	if l == nil {
		return dst, 0, dmrgo.ErrNilReceiver
	}
	var w writer
	w.start(ExtensionLongReport, -1)
	l.Time.write(&w)
	l.Location.write(&w)
	l.Velocity.write(&w)
	w.flag(l.AckRequested)
	w.flag(l.UserDefined)
	w.put(8, uint64(l.AdditionalData), nil)
	return w.finish(dst)
}
