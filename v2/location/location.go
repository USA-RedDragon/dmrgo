package location

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

// Format is the format that a position came in.
type Format uint8

// The Format values.
const (
	// FormatNMEA is an NMEA RMC sentence to UDP port 10110, as AnyTone
	// radios send.
	FormatNMEA Format = iota + 1
	// FormatLRRP is a MOTOTRBO LRRP report to UDP port 4001, as MOTOTRBO
	// and Hytera radios send.
	FormatLRRP
)

// String returns the name of f. It allocates only for a value without a
// name.
func (f Format) String() string {
	switch f {
	case FormatNMEA:
		return "NMEA"
	case FormatLRRP:
		return "LRRP"
	}
	return "Format(" + text.Int(int64(f)) + ")"
}

// Fix is a position that a radio reported.
type Fix struct {
	// Format is the format of the report.
	Format Format
	// Source and Destination are the radio IDs of the data packet.
	Source, Destination uint32
	// Valid reports a position from a GNSS fix. An NMEA sentence with
	// status V gives false.
	Valid bool
	// Latitude and Longitude are in degrees, north and east positive.
	Latitude, Longitude float64
	// Altitude is in meters, when HasAltitude is set.
	Altitude    float64
	HasAltitude bool
	// Radius is the accuracy of the position in meters, when HasRadius is
	// set.
	Radius    float64
	HasRadius bool
	// Speed is the speed over ground in meters per second, when HasSpeed
	// is set.
	Speed    float64
	HasSpeed bool
	// Course is the course over ground in degrees from north, when
	// HasCourse is set.
	Course    float64
	HasCourse bool
	// Year to Second are the UTC time of the position, when HasTime is
	// set.
	Year                             uint16
	Month, Day, Hour, Minute, Second uint8
	HasTime                          bool
}

const knot = 1852.0 / 3600

// Decoder decodes positions. It holds the storage of an LRRP message, so
// keep a Decoder in a struct or a package variable rather than in a local
// variable on a small stack. The zero value is ready to use. A Decoder is
// not safe for concurrent use.
type Decoder struct {
	report lrrp.Message
}

// Decode returns the position in a data packet, with header h and user
// data data: an NMEA RMC sentence
// to UDP port 10110 or an LRRP report with a position to UDP port 4001. It
// returns ErrNoFix for a packet in neither format, and the errors of the
// ip, lrrp and nmea packages for a message that does not decode. It does
// not allocate.
func (d *Decoder) Decode(h *pdu.DataHeader, data []byte) (Fix, error) {
	var sap enums.ServiceAccessPointID
	switch {
	case h.UnconfirmedDataHeader != nil:
		sap = h.UnconfirmedDataHeader.SAP
	case h.ConfirmedDataHeader != nil:
		sap = h.ConfirmedDataHeader.SAP
	default:
		return Fix{}, ErrNoFix
	}
	dg, err := ip.Decode(sap, data)
	if err != nil {
		return Fix{}, err
	}
	var f Fix
	target, source, _ := h.Addresses()
	f.Source, f.Destination = source, target
	switch dg.DestinationPort {
	case ip.PortNMEA:
		r, err := nmea.ParseRMC(dg.Payload)
		if err != nil {
			return Fix{}, err
		}
		f.Format, f.Valid = FormatNMEA, r.Valid
		f.Latitude, f.Longitude = r.Latitude, r.Longitude
		f.Speed, f.HasSpeed = r.Speed*knot, true
		f.Course, f.HasCourse = r.Course, true
		if r.Day != 0 {
			f.Year, f.Month, f.Day = r.Year, r.Month, r.Day
			f.Hour, f.Minute, f.Second = r.Hour, r.Minute, r.Second
			f.HasTime = true
		}
		return f, nil
	case ip.PortLRRP:
		if err := lrrp.DecodeMessageInto(&d.report, dg.Payload); err != nil {
			return Fix{}, err
		}
		if !fromReport(&f, &d.report) {
			return Fix{}, ErrNoFix
		}
		return f, nil
	default:
		return Fix{}, ErrNoFix
	}
}

func fromReport(f *Fix, m *lrrp.Message) bool {
	switch m.Document {
	case lrrp.ImmediateRequest, lrrp.TriggeredRequest, lrrp.StopRequest, lrrp.ProtocolRequest:
		return false
	default:
	}
	r := &m.Report
	switch {
	case r.HasCircle3D:
		c := &r.Circle3D
		f.Latitude, f.Longitude, f.Radius, f.HasRadius, f.Altitude, f.HasAltitude = c.Latitude, c.Longitude, c.Radius, true, c.Altitude, true
	case r.HasPoint3DAccuracy:
		p := &r.Point3DAccuracy
		f.Latitude, f.Longitude, f.Altitude, f.HasAltitude = p.Latitude, p.Longitude, p.Altitude, true
	case r.HasPoint3D:
		p := &r.Point3D
		f.Latitude, f.Longitude, f.Altitude, f.HasAltitude = p.Latitude, p.Longitude, p.Altitude, true
	case r.HasCircle2D:
		c := &r.Circle2D
		f.Latitude, f.Longitude, f.Radius, f.HasRadius = c.Latitude, c.Longitude, c.Radius, true
	case r.HasPoint2D:
		f.Latitude, f.Longitude = r.Point2D.Latitude, r.Point2D.Longitude
	default:
		return false
	}
	f.Format, f.Valid = FormatLRRP, true
	f.Speed, f.HasSpeed = r.Speed, r.HasSpeed
	f.Course, f.HasCourse = float64(r.Direction)*2, r.HasDirection
	if r.HasTime {
		t := &r.Time
		f.Year, f.Month, f.Day, f.Hour, f.Minute, f.Second, f.HasTime = t.Year, t.Month, t.Day, t.Hour, t.Minute, t.Second, true
	}
	return true
}
