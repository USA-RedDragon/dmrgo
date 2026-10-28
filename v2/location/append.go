package location

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

// Header holds the data header that AppendPacket builds.
type Header struct {
	// Data is the data header, for packet.NewPacket or a data sender.
	Data pdu.DataHeader
}

// AppendPacket appends the user data of the position f to dst, sets h to
// the data header to send it with, and returns dst. It sends f as an AnyTone
// radio sends its position: an NMEA RMC sentence from nmea.AppendRMC with
// talker GP, from UDP port 10110 to 10110, behind a compressed UDP/IPv4
// header (SAP UDP/IP header compression) with IPv4 Identification 1 and the
// radio network as SAID and DAID, in an unconfirmed packet (U_HEAD) from
// f.Source to f.Destination. A valid f gives status A, the position and mode
// A; an f that is not valid gives status V, no position and mode N, which
// Decode returns with ErrNoPosition. Speed is in meters per second and is
// sent in knots. The time and date come from Year to Second when HasTime is
// set. It returns dst unchanged and ErrFormat for a Format other than
// FormatNMEA, dmrgo.ErrNilArgument for a nil f or h, and nmea.ErrField or
// the errors of the packages that build the message. It allocates only to
// grow dst.
func AppendPacket(dst []byte, f *Fix, h *Header) ([]byte, error) {
	if f == nil || h == nil {
		return dst, dmrgo.ErrNilArgument
	}
	h.Data = pdu.DataHeader{}
	if f.Format != FormatNMEA {
		return dst, ErrFormat
	}
	r := nmea.RMC{Talker: [2]byte{'G', 'P'}, Valid: f.Valid, Mode: 'N'}
	if f.Valid {
		r.Latitude, r.Longitude, r.HasPosition, r.Mode = f.Latitude, f.Longitude, true, 'A'
	}
	r.Speed, r.HasSpeed = f.Speed/knot, f.HasSpeed
	r.Course, r.HasCourse = f.Course, f.HasCourse
	if f.HasTime {
		r.Hour, r.Minute, r.Second, r.HasTime = f.Hour, f.Minute, f.Second, true
		r.Day, r.Month, r.Year, r.HasDate = f.Day, f.Month, f.Year, true
	}
	n := len(dst)
	dst, err := nmea.AppendRMC(dst, &r)
	if err != nil {
		return dst[:n], err
	}
	d := ip.Datagram{
		SourcePort: ip.PortNMEA, DestinationPort: ip.PortNMEA, ID: 1, Compressed: true,
		SAID: enums.SAIDRadioNetwork, DAID: enums.DAIDRadioNetwork, Payload: dst[n:],
	}
	i0 := len(dst)
	if dst, err = ip.AppendCompressed(dst, &d); err != nil {
		return dst[:n], err
	}
	k := copy(dst[n:], dst[i0:])
	if err = h.Data.SetUnconfirmed(pdu.UnconfirmedDataHeader{SAP: enums.SAPUDPIPHeaderCompression,
		LLIDDestination: f.Destination, LLIDSource: f.Source, FullMessageFlag: true}); err != nil {
		return dst[:n], err
	}
	return dst[:n+k], nil
}
