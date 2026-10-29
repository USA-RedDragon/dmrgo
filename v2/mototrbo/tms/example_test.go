package tms_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
)

// ExampleNewText builds a text message that requests an acknowledgement,
// and the acknowledgement that the receiver sends back.
func ExampleNewText() {
	m, _ := tms.NewText(5, true)
	m.SetTextUTF8(nil, []byte("\r\nHello"))
	out, _ := m.AppendBinary(nil)
	fmt.Println(hex.EncodeToString(out))

	got, _ := tms.DecodeMessage(out)
	seq, _ := got.Sequence()
	ack, _ := tms.NewAck(seq)
	out, _ = ack.AppendBinary(nil)
	fmt.Printf("%q ack %x\n", got.TextUTF8(nil), out)
	// Output:
	// 0012e00085040d000a00480065006c006c006f00
	// "\r\nHello" ack 00039f0005
}

// ExampleDecodeMessage reads a text message that an AnyTone radio sent to
// radio 3140598 in its M-SMS mode, as an IPv4 packet of a DMR data message.
// The radio requests no acknowledgement; when a text requests one, the
// receiver sends it back in an IPv4 packet to the port it came from.
func ExampleDecodeMessage() {
	packet, _ := hex.DecodeString(appcapture.TMS)
	d, err := ip.Decode(enums.SAPIPBasedPacketData, packet)
	if err != nil || d.DestinationPort != ip.PortTMS {
		fmt.Println(d.DestinationPort, err)
		return
	}
	m, err := tms.DecodeMessage(d.Payload)
	if err != nil {
		fmt.Println(err)
		return
	}
	seq, _ := m.Sequence()
	fmt.Printf("%s %d from %d to %d: %q\n", m.Header.Type, seq, ip.RadioID(d.Source), ip.RadioID(d.Destination), m.TextUTF8(nil))
	if !m.Header.AckRequired {
		fmt.Println("no acknowledgement requested")
		return
	}
	ack, _ := tms.NewAck(seq)
	payload, _ := ack.AppendBinary(nil)
	reply, _ := ip.AppendIPv4(nil, &ip.Datagram{Source: d.Destination, Destination: d.Source, SourcePort: d.DestinationPort, DestinationPort: d.SourcePort, Payload: payload})
	fmt.Printf("ack %x in %d octets\n", payload, len(reply))
	// Output:
	// Text 1 from 3191868 to 3140598: "\r\nMOTO TEST"
	// no acknowledgement requested
}
