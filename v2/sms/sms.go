package sms

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

const defaultGroupPrefix = 225

// Format is the format of a text message.
type Format uint8

// The Format values.
const (
	// FormatTMS is a MOTOTRBO text message to UDP port 4007, as AnyTone
	// radios send in M-SMS mode.
	FormatTMS Format = iota + 1
	// FormatIP is an ETSI text message to UDP port 5016 in UTF-16BE.
	FormatIP
	// FormatAnyTone is the text to UDP port 5016 that AnyTone radios send
	// in DMR Standard mode.
	FormatAnyTone
	// FormatDefinedData is a short data packet (DD_HEAD) whose DD format
	// gives the coding of the text.
	FormatDefinedData
	// FormatHytera is the short data packet that AnyTone radios send in
	// H-SMS mode: DD format 1 and the HyteraText layout.
	FormatHytera
)

// String returns the name of f. It allocates only for a value without a
// name.
func (f Format) String() string {
	switch f {
	case FormatTMS:
		return "TMS"
	case FormatIP:
		return "IP"
	case FormatAnyTone:
		return "AnyTone"
	case FormatDefinedData:
		return "Defined Data"
	case FormatHytera:
		return "Hytera"
	}
	return "Format(" + text.Int(int64(f)) + ")"
}

// Text is a text message.
type Text struct {
	// Format is the format of the message.
	Format Format
	// Source and Destination are the radio IDs, or the talkgroup for a
	// group message.
	Source, Destination uint32
	// Group reports a message to a talkgroup.
	Group bool
	// Network is the network ID of the IPv4 radio addresses of a FormatTMS,
	// FormatIP or FormatAnyTone message, Network.x.y.z. The zero value
	// means ip.DefaultNetwork. Decode sets it from the source address, and
	// leaves it 0 for a compressed header.
	Network uint8
	// GroupPrefix is the first octet of the IPv4 multicast address of a
	// group message in those formats. The zero value means 225. Decode sets
	// it from the destination address of a group message.
	GroupPrefix uint8
	// DD is the DD format of a FormatDefinedData message.
	DD enums.DDFormat
	// Sequence is the sequence number of a FormatTMS message.
	Sequence uint8
	// AckRequired reports that a FormatTMS message asks for an
	// acknowledgement.
	AckRequired bool
	// Body is the text in UTF-8, without the CR LF that MOTOTRBO radios put
	// before it and without a NUL after it.
	Body []byte
}

// Decode appends the text of a data packet, with header h and user data
// data, to dst as UTF-8, and returns
// dst and the Text, whose Body points into dst. It finds the format from
// the header and the UDP port: TMS on port 4007, IP or AnyTone text on
// port 5016, and defined data. It returns ErrNotText for a packet in none
// of these formats and for defined data with DD format binary, the errors
// of the ip, tms and shortdata packages for a message that does not
// decode, and ErrNil for a nil h. It allocates only to grow dst.
func Decode(dst []byte, h *pdu.DataHeader, data []byte) ([]byte, Text, error) {
	if h == nil {
		return dst, Text{}, ErrNil
	}
	n := len(dst)
	var t Text
	target, source, _ := h.Addresses()
	t.Source, t.Destination = source, target
	switch {
	case h.DefinedDataHeader != nil:
		d := h.DefinedDataHeader
		t.Group, t.DD = d.Group, d.DefinedData
		if d.DefinedData == enums.DDBinary {
			return dst, Text{}, ErrNotText
		}
		if pad := int(d.BitPadding / 8); pad <= len(data) {
			data = data[:len(data)-pad]
		}
		if d.DefinedData == enums.DDBCD {
			ht, err := shortdata.DecodeHyteraText(data)
			if err != nil {
				return dst, Text{}, err
			}
			t.Format = FormatHytera
			dst = ht.TextUTF8(dst)
			t.Body = dst[n:]
			return dst, t, nil
		}
		dst, err := shortdata.Text(dst, d.DefinedData, data)
		if err != nil {
			return dst, Text{}, err
		}
		t.Format = FormatDefinedData
		t.Body = dst[n:]
		return dst, t, nil
	case h.UnconfirmedDataHeader != nil:
		t.Group = h.UnconfirmedDataHeader.Group
		return decodeIP(dst, h.UnconfirmedDataHeader.SAP, data, t)
	case h.ConfirmedDataHeader != nil:
		t.Group = h.ConfirmedDataHeader.Group
		return decodeIP(dst, h.ConfirmedDataHeader.SAP, data, t)
	}
	return dst, Text{}, ErrNotText
}

func decodeIP(dst []byte, sap enums.ServiceAccessPointID, data []byte, t Text) ([]byte, Text, error) {
	n := len(dst)
	d, err := ip.Decode(sap, data)
	if err != nil {
		return dst, Text{}, err
	}
	t.Network = d.Source[0]
	if t.Group {
		t.GroupPrefix = d.Destination[0]
	}
	switch d.DestinationPort {
	case ip.PortTMS:
		msg, err := tms.DecodeMessage(d.Payload)
		if err != nil {
			return dst, Text{}, err
		}
		if msg.Header.Type != tms.TypeText {
			return dst, Text{}, ErrNotText
		}
		t.Format = FormatTMS
		t.Sequence, _ = msg.Sequence()
		t.AckRequired = msg.Header.AckRequired
		body := msg.Text
		if len(body) >= 4 && body[0] == '\r' && body[1] == 0 && body[2] == '\n' && body[3] == 0 {
			body = body[4:]
		}
		msg.Text = body
		dst = msg.TextUTF8(dst)
	case ip.PortText:
		p := d.Payload
		if len(p) >= 6 && p[0] == 0 && p[1] == 0x0D && p[2] == 0 && p[3] == 0x0A && p[4] != 0 && p[5] == 0 {
			a, err := DecodeAnyToneText(p)
			if err != nil {
				return dst, Text{}, err
			}
			t.Format = FormatAnyTone
			dst = a.TextUTF8(dst)
		} else {
			e, err := DecodeIPText(p)
			if err != nil {
				return dst, Text{}, err
			}
			t.Format = FormatIP
			dst = e.TextUTF8(dst)
		}
	default:
		return dst, Text{}, ErrNotText
	}
	t.Body = dst[n:]
	return dst, t, nil
}

// Header holds the data header that AppendPacket builds. Data points into
// the Header, so a Header must not be copied after AppendPacket; go vet
// reports a copy.
type Header struct {
	_ noCopy
	// Data is the data header, for packet.NewPacket or a data sender.
	Data        pdu.DataHeader
	unconfirmed pdu.UnconfirmedDataHeader
	confirmed   pdu.ConfirmedDataHeader
	defined     pdu.DefinedDataHeader
}

// AppendPacket appends the user data of t in its format to dst, sets h to
// the data header to send it with, and returns dst. An IP format uses an
// IPv4 header (SAP IP based packet data) with the radio addresses of
// network t.Network and a group address with prefix t.GroupPrefix, and a
// confirmed packet when confirmed is set. A TMS text starts with a CR LF.
// It returns dst unchanged and ErrFormat for an unknown format and for
// FormatDefinedData with DD format binary, which holds no text, or BCD,
// which Decode reads as FormatHytera, ErrNil for a nil t or h, or the
// errors of the packages that build the message. It allocates only to
// grow dst.
func AppendPacket(dst []byte, t *Text, confirmed bool, h *Header) ([]byte, error) {
	if t == nil || h == nil {
		return dst, ErrNil
	}
	n := len(dst)
	h.Data = pdu.DataHeader{}
	var err error
	switch t.Format {
	case FormatDefinedData:
		if t.DD == enums.DDBCD || t.DD == enums.DDBinary {
			return dst[:n], ErrFormat
		}
		if dst, err = shortdata.AppendText(dst, t.DD, t.Body); err != nil {
			return dst[:n], err
		}
		h.defined = definedHeader(t, t.DD, confirmed)
		h.Data = pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &h.defined}
		return dst, nil
	case FormatHytera:
		var ht shortdata.HyteraText
		dst = ht.SetTextUTF8(dst, t.Body)
		p0 := len(dst)
		if dst, err = ht.AppendBinary(dst); err != nil {
			return dst[:n], err
		}
		k := copy(dst[n:], dst[p0:])
		h.defined = definedHeader(t, enums.DDBCD, confirmed)
		h.Data = pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &h.defined}
		return dst[:n+k], nil
	case FormatTMS, FormatIP, FormatAnyTone:
	default:
		return dst, ErrFormat
	}
	network := t.Network
	if network == 0 {
		network = ip.DefaultNetwork
	}
	d := ip.Datagram{
		Source:      ip.RadioAddress(network, t.Source),
		Destination: ip.RadioAddress(network, t.Destination),
		SourcePort:  ip.PortText, DestinationPort: ip.PortText,
	}
	if t.Group {
		prefix := t.GroupPrefix
		if prefix == 0 {
			prefix = defaultGroupPrefix
		}
		d.Destination = ip.GroupAddress(prefix, t.Destination)
	}
	var p0 int
	switch t.Format {
	case FormatTMS:
		d.SourcePort, d.DestinationPort, d.TTL = ip.PortTMS, ip.PortTMS, 64
		m, err := tms.NewText(t.Sequence, t.AckRequired)
		if err != nil {
			return dst[:n], err
		}
		dst = append(dst, '\r', 0, '\n', 0)
		dst = m.SetTextUTF8(dst, t.Body)
		m.Text = dst[n:]
		p0 = len(dst)
		dst, err = m.AppendBinary(dst)
		if err != nil {
			return dst[:n], err
		}
	case FormatAnyTone:
		d.TTL = 1
		var a AnyToneText
		dst = a.SetTextUTF8(dst, t.Body)
		p0 = len(dst)
		if dst, err = a.AppendBinary(dst); err != nil {
			return dst[:n], err
		}
	default:
		var e IPText
		dst = e.SetTextUTF8(dst, t.Body)
	}
	d.Payload = dst[p0:]
	i0 := len(dst)
	if dst, err = ip.AppendIPv4(dst, &d); err != nil {
		return dst[:n], err
	}
	k := copy(dst[n:], dst[i0:])
	dst = dst[:n+k]
	if confirmed {
		h.confirmed = pdu.ConfirmedDataHeader{Group: t.Group, ResponseRequested: true, SAP: enums.SAPIPBasedPacketData,
			LLIDDestination: t.Destination, LLIDSource: t.Source, FullMessageFlag: true}
		h.Data = pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &h.confirmed}
	} else {
		h.unconfirmed = pdu.UnconfirmedDataHeader{Group: t.Group, SAP: enums.SAPIPBasedPacketData,
			LLIDDestination: t.Destination, LLIDSource: t.Source, FullMessageFlag: true}
		h.Data = pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &h.unconfirmed}
	}
	return dst, nil
}

func definedHeader(t *Text, dd enums.DDFormat, confirmed bool) pdu.DefinedDataHeader {
	return pdu.DefinedDataHeader{
		Group: t.Group, ResponseRequested: confirmed, SAP: enums.SAPShortData,
		LLIDDestination: t.Destination, LLIDSource: t.Source, DefinedData: dd, FullMessageFlag: true,
	}
}
