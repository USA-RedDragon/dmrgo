package location_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera"
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
	m := packet.Message{
		Header: pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: 3140598, LLIDSource: 3191868,
		}},
		Data: data,
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
	hdap := hytera.Message{Header: hytera.Header{Service: hytera.ServiceLP}, Opcode: hytera.LPConditionReport, Payload: payload}
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
	h := pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: 1, LLIDSource: 80,
	}}
	f, err := decoder.Decode(&h, data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s from %d: %.5f %.5f %d-%02d-%02d\n", f.Format, f.Source, f.Latitude, f.Longitude, f.Year, f.Month, f.Day)
	// Output:
	// HDAP from 80: 34.21217 108.83333 2025-12-02
}
