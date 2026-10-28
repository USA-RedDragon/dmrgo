package location_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

var decoder location.Decoder

// ExampleDecoder_Decode reads the position report of an AnyTone radio.
func ExampleDecoder_Decode() {
	data, _ := hex.DecodeString(appcapture.GPS)
	m := packet.Message{Data: data}
	if err := m.Header.SetUnconfirmed(pdu.UnconfirmedDataHeader{
		SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: 3140598, LLIDSource: 3191868,
	}); err != nil {
		fmt.Println(err)
		return
	}
	f, err := decoder.Decode(&m.Header, m.Data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s from %d: %.5f %.5f %.0f deg\n", f.Format, f.Source, f.Latitude, f.Longitude, f.Course)
	// Output:
	// NMEA from 3191868: 34.21217 108.83333 121 deg
}

// ExampleDecoder_Decode_hytera reads the condition report of a Hytera
// radio, an HDAP location protocol message to UDP port 3003.
func ExampleDecoder_Decode_hytera() {
	payload := append([]byte{0, 0, 0, 9, 10, 0, 0, 80}, "A200037021225N3412.7300E10850.00000.0121"...)
	hdap := hdap.Message{Header: hdap.Header{Service: hdap.ServiceLP}, Opcode: hdap.LPConditionReport, Payload: payload}
	hdap.Seal()
	udp, err := hdap.AppendBinary(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	d := ip.Datagram{SourcePort: ip.PortHyteraLocation, DestinationPort: ip.PortHyteraLocation, Source: ip.RadioAddress(12, 80), Destination: ip.RadioAddress(13, 1), Payload: udp}
	data, err := ip.AppendIPv4(nil, &d)
	if err != nil {
		fmt.Println(err)
		return
	}
	var h pdu.DataHeader
	if err := h.SetUnconfirmed(pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: 1, LLIDSource: 80,
	}); err != nil {
		fmt.Println(err)
		return
	}
	f, err := decoder.Decode(&h, data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s from %d: %.5f %.5f %d-%02d-%02d\n", f.Format, f.Source, f.Latitude, f.Longitude, f.Year, f.Month, f.Day)
	// Output:
	// HDAP from 80: 34.21217 108.83333 2025-12-02
}

var header location.Header

// ExampleAppendPacket sends a position as an AnyTone radio does and reads
// it back.
func ExampleAppendPacket() {
	f := location.Fix{
		Format: location.FormatNMEA, Source: 3191868, Destination: 3140598, Valid: true,
		Latitude: 34.212166, Longitude: 108.833333, Course: 121, HasCourse: true, HasSpeed: true,
		Year: 2025, Month: 12, Day: 2, Hour: 20, Minute: 0, Second: 37, HasTime: true,
	}
	data, err := location.AppendPacket(nil, &f, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header.Data, data, enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	got, err := decoder.Decode(&p.Header, data)
	fmt.Printf("%d blocks: %q\n%s from %d: %.5f %.5f %v\n", len(p.Blocks), data[9:], got.Format, got.Source, got.Latitude, got.Longitude, err)
	// Output:
	// 8 blocks: "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"
	// NMEA from 3191868: 34.21217 108.83333 <nil>
}
