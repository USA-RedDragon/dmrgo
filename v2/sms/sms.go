package sms

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
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
	// radios send in M-SMS mode. AppendPacket sends no NUL after the text.
	FormatTMS Format = iota + 1
	// FormatIP is an ETSI text message to UDP port 5016: the text in
	// UTF-16BE, with no prefix and no NUL, as ETSI TS 102 361-3 clause
	// 5.7.1 gives it and as one Hytera radio sends it. AppendPacket sends
	// no NUL after the text. A DA-7X2 in DMR Standard mode shows the text
	// of this format as UTF-16LE. Use FormatAnyTone to text an AnyTone
	// radio.
	FormatIP
	// FormatAnyTone is the text to UDP port 5016 that AnyTone radios send
	// in DMR Standard mode: the octets 00 0d 00 0a, then the text in
	// UTF-16LE. A DA-7X2 with D890UV firmware ends the text with a UTF-16
	// NUL; a 6X2 Pro and a D878 do not. AppendPacket sends one UTF-16 NUL
	// after the text, as a DA-7X2 shows old characters after a text
	// without it.
	FormatAnyTone
	// FormatDefinedData is a short data packet (DD_HEAD) whose DD format
	// gives the coding of the text. AppendPacket sends no NUL after the
	// text.
	FormatDefinedData
	// FormatHytera is the short data packet that AnyTone radios send in
	// H-SMS mode: DD format 1 (BCD) and the HyteraText layout. AppendPacket
	// sends one UTF-16 NUL after the text, as AnyTone radios do in H-SMS
	// mode.
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
	// Confirmed reports a message that asks the data link layer for a
	// response: a C_HEAD packet for the formats on IP, and a DD_HEAD with
	// the Response Requested flag (A) for FormatDefinedData and
	// FormatHytera (ETSI TS 102 361-3 clause 6.4, Table 6.5). Decode sets
	// it from the header and AppendPacket builds that header.
	Confirmed bool
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
	// before it. Decode drops a NUL after the text in every format.
	// AppendPacket adds one only for FormatAnyTone and FormatHytera, and
	// sends a NUL that Body ends with.
	Body []byte
}

// Decode appends the text of a data packet, with header h and user data
// data, to dst as UTF-8, and returns dst and the Text, whose Body points
// into dst. data is the Data of a packet.Message, without the pad octets
// that the Bit Padding of a DD_HEAD counts. It finds the format from the
// header and the UDP port: TMS on port 4007, IP or AnyTone text on port
// 5016, and defined data. A text on port 5016 is FormatAnyTone when its
// payload starts with 00 0d 00 0a, the prefix of AnyToneText, whether or
// not a NUL ends the text. Other text on port 5016 is FormatIP. An
// ETSI text that starts with CR LF starts with the same octets, and Decode
// reads it as FormatAnyTone. Defined data with DD format BCD is
// FormatHytera when it has the HyteraText layout: an even length of at
// least 6 octets, 2 octets and then a UTF-16LE text of at least one
// character that ends in 00 00, with only 00 octets after it. Other BCD
// data is FormatDefinedData; BCD digits that end in six or more 0 digits
// can have that layout too. It returns ErrNotText for a packet in none of
// these formats and for defined data with DD format binary, the errors of
// the ip, tms and shortdata packages for a message that does not decode,
// the Text with ip.ErrChecksum for a text whose IPv4 or UDP checksum does
// not match, as the block CRCs of the packet already checked its octets,
// and dmrgo.ErrNilArgument for a nil h. It allocates only to grow dst.
func Decode(dst []byte, h *pdu.DataHeader, data []byte) ([]byte, Text, error) {
	if h == nil {
		return dst, Text{}, dmrgo.ErrNilArgument
	}
	n := len(dst)
	var t Text
	target, source, _ := h.Addresses()
	t.Source, t.Destination = source, target
	if d, ok := h.AsDefinedData(); ok {
		t.Group, t.DD, t.Confirmed = d.Group, d.DefinedData, d.ResponseRequested
		if d.DefinedData == enums.DDBinary {
			return dst, Text{}, ErrNotText
		}
		if d.DefinedData == enums.DDBCD && hyteraLayout(data) {
			ht, err := shortdata.DecodeHyteraText(data)
			if err != nil {
				return dst, Text{}, err
			}
			t.Format = FormatHytera
			dst = ht.TextUTF8(dst)
			t.Body = dst[n:]
			return dst, t, nil
		}
		dst, err := shortdata.DecodeText(dst, d.DefinedData, data)
		if err != nil {
			return dst, Text{}, err
		}
		t.Format = FormatDefinedData
		t.Body = dst[n:]
		return dst, t, nil
	}
	if u, ok := h.AsUnconfirmed(); ok {
		t.Group = u.Group
		return decodeIP(dst, u.SAP, data, t)
	}
	if c, ok := h.AsConfirmed(); ok {
		t.Group, t.Confirmed = c.Group, true
		return decodeIP(dst, c.SAP, data, t)
	}
	return dst, Text{}, ErrNotText
}

func hyteraLayout(data []byte) bool {
	if len(data) < 6 || len(data)%2 != 0 || data[2] == 0 && data[3] == 0 {
		return false
	}
	end := -1
	for i := 4; i < len(data); i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			end = i
			break
		}
	}
	if end < 0 {
		return false
	}
	for _, c := range data[end+2:] {
		if c != 0 {
			return false
		}
	}
	return true
}

func decodeIP(dst []byte, sap enums.SAP, data []byte, t Text) ([]byte, Text, error) {
	n := len(dst)
	d, sumErr := ip.Decode(sap, data)
	if sumErr != nil && !errors.Is(sumErr, ip.ErrChecksum) {
		return dst, Text{}, sumErr
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
		if anyToneLayout(p) {
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
	return dst, t, sumErr
}

// Header holds the data header that AppendPacket builds.
type Header struct {
	// Data is the data header, for packet.NewPacket or a data sender.
	Data pdu.DataHeader
}

// AppendPacket appends the user data of t in its format to dst, sets h to
// the data header to send it with, and returns dst. An IP format uses an
// IPv4 header (SAP IP based packet data) with the radio addresses of
// network t.Network and a group address with prefix t.GroupPrefix, and a
// C_HEAD when t.Confirmed is set. Defined data sets the Response Requested
// flag (A) when t.Confirmed is set. A TMS text starts with a CR LF. Only
// FormatAnyTone and FormatHytera end the text with a NUL; the Format values
// say why. BCD digits that would have the HyteraText layout get an octet
// FF, the end of the digits, so that Decode reads them as
// FormatDefinedData. It returns dst unchanged and ErrFormat for an unknown
// format, for FormatDefinedData with DD format binary, which holds no text,
// and for a FormatHytera text without the HyteraText layout, which Decode
// reads as FormatDefinedData: a Body that is empty, starts with a NUL, or
// has a character other than NUL after a NUL, and for a FormatIP text with
// a Body that starts with CR LF, which Decode reads as FormatAnyTone. Such
// a text and an AnyTone text can give the same octets: the IP text CR LF A
// NUL and the AnyTone text U+4100 are both 00 0d 00 0a 00 41 00 00. It
// returns dmrgo.ErrNilArgument for a nil t or h, or the errors of the
// packages that build the message. It allocates only to grow dst.
func AppendPacket(dst []byte, t *Text, h *Header) ([]byte, error) {
	if t == nil || h == nil {
		return dst, dmrgo.ErrNilArgument
	}
	n := len(dst)
	h.Data = pdu.DataHeader{}
	var err error
	switch t.Format {
	case FormatDefinedData:
		if t.DD == enums.DDBinary {
			return dst[:n], ErrFormat
		}
		if dst, err = shortdata.AppendText(dst, t.DD, t.Body); err != nil {
			return dst[:n], err
		}
		if t.DD == enums.DDBCD && hyteraLayout(dst[n:]) {
			dst = append(dst, 0xFF)
		}
		if err = h.Data.SetDefinedData(definedHeader(t, t.DD)); err != nil {
			return dst[:n], err
		}
		return dst, nil
	case FormatHytera:
		var ht shortdata.HyteraText
		dst = ht.SetTextUTF8(dst, t.Body)
		p0 := len(dst)
		if dst, err = ht.AppendBinary(dst); err != nil {
			return dst[:n], err
		}
		k := copy(dst[n:], dst[p0:])
		if !hyteraLayout(dst[n : n+k]) {
			return dst[:n], ErrFormat
		}
		if err = h.Data.SetDefinedData(definedHeader(t, enums.DDBCD)); err != nil {
			return dst[:n], err
		}
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
		p0 = len(dst)
		dst = e.SetTextUTF8(dst, t.Body)
		if anyToneLayout(dst[p0:]) {
			return dst[:n], ErrFormat
		}
	}
	d.Payload = dst[p0:]
	i0 := len(dst)
	if dst, err = ip.AppendIPv4(dst, &d); err != nil {
		return dst[:n], err
	}
	k := copy(dst[n:], dst[i0:])
	if t.Confirmed {
		err = h.Data.SetConfirmed(pdu.ConfirmedDataHeader{Group: t.Group, ResponseRequested: true, SAP: enums.SAPIPBasedPacketData,
			LLIDDestination: t.Destination, LLIDSource: t.Source, FullMessageFlag: true})
	} else {
		err = h.Data.SetUnconfirmed(pdu.UnconfirmedDataHeader{Group: t.Group, SAP: enums.SAPIPBasedPacketData,
			LLIDDestination: t.Destination, LLIDSource: t.Source, FullMessageFlag: true})
	}
	if err != nil {
		return dst[:n], err
	}
	return dst[:n+k], nil
}

// AppendAck appends to dst the TMS acknowledgement of the FormatTMS text
// t, as Decode returns it, sets h to the data header to send it with, and
// returns dst. Send it when t.AckRequired is set. The acknowledgement goes
// from t.Destination back to t.Source in network t.Network, from UDP port
// 4007 to 4007, with the sequence number t.Sequence, in an unconfirmed
// packet (U_HEAD). It returns dst unchanged and ErrFormat for another
// format and for a group text, which gets no acknowledgement,
// dmrgo.ErrNilArgument for a nil t or h, or the errors of the packages
// that build the message. It allocates only to grow dst.
func AppendAck(dst []byte, t *Text, h *Header) ([]byte, error) {
	if t == nil || h == nil {
		return dst, dmrgo.ErrNilArgument
	}
	if t.Format != FormatTMS || t.Group {
		return dst, ErrFormat
	}
	m, err := tms.NewAck(t.Sequence)
	if err != nil {
		return dst, err
	}
	n := len(dst)
	if dst, err = m.AppendBinary(dst); err != nil {
		return dst[:n], err
	}
	network := t.Network
	if network == 0 {
		network = ip.DefaultNetwork
	}
	d := ip.Datagram{
		Source:      ip.RadioAddress(network, t.Destination),
		Destination: ip.RadioAddress(network, t.Source),
		SourcePort:  ip.PortTMS, DestinationPort: ip.PortTMS, TTL: 64,
		Payload: dst[n:],
	}
	i0 := len(dst)
	if dst, err = ip.AppendIPv4(dst, &d); err != nil {
		return dst[:n], err
	}
	k := copy(dst[n:], dst[i0:])
	if err = h.Data.SetUnconfirmed(pdu.UnconfirmedDataHeader{SAP: enums.SAPIPBasedPacketData,
		LLIDDestination: t.Source, LLIDSource: t.Destination, FullMessageFlag: true}); err != nil {
		return dst[:n], err
	}
	return dst[:n+k], nil
}

func anyToneLayout(p []byte) bool {
	return len(p) >= 4 && p[0] == 0 && p[1] == 0x0D && p[2] == 0 && p[3] == 0x0A
}

func definedHeader(t *Text, dd enums.DDFormat) pdu.DefinedDataHeader {
	return pdu.DefinedDataHeader{
		Group: t.Group, ResponseRequested: t.Confirmed, SAP: enums.SAPShortData,
		LLIDDestination: t.Destination, LLIDSource: t.Source, DefinedData: dd, FullMessageFlag: true,
	}
}
