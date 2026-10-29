package ars_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
)

// ExampleDecodeMessage reads the registration of a radio and answers it
// with a refresh time of 30 minutes.
func ExampleDecodeMessage() {
	m, err := ars.DecodeMessage([]byte{0x00, 0x0c, 0xf0, 0x20, 0x07, '3', '1', '9', '1', '8', '6', '8', 0x00, 0x00})
	if err != nil {
		fmt.Println(err)
		return
	}
	id, _ := m.DeviceID()
	ack, _ := ars.NewAck(1)
	out, _ := ack.AppendBinary(nil)
	fmt.Printf("%s %s %d, answer %x\n", m.Header.Type, m.Registration.Event, id, out)
	// Output:
	// Device Registration Initial 3191868, answer 0002bf01
}

// ExampleNewAck takes the registration of radio 3191868 out of the IPv4
// packet of a DMR data message to the server at radio ID 3140598, and
// builds the IPv4 packet of the answer.
func ExampleNewAck() {
	reg := ars.NewRegistration(ars.EventInitial)
	if _, err := reg.SetDeviceID(nil, 3191868); err != nil {
		fmt.Println(err)
		return
	}
	payload, _ := reg.AppendBinary(nil)
	radio := ip.RadioAddress(ip.DefaultNetwork, 3191868)
	server := ip.RadioAddress(ip.DefaultNetwork, 3140598)
	packet, _ := ip.AppendIPv4(nil, &ip.Datagram{Source: radio, Destination: server, SourcePort: ip.PortARS, DestinationPort: ip.PortARS, Payload: payload})

	d, err := ip.Decode(enums.SAPIPBasedPacketData, packet)
	if err != nil || d.DestinationPort != ip.PortARS {
		fmt.Println(d.DestinationPort, err)
		return
	}
	m, err := ars.DecodeMessage(d.Payload)
	if err != nil {
		fmt.Println(err)
		return
	}
	id, _ := m.DeviceID()
	fmt.Println(m.Header.Type, m.Registration.Event, "from", id, "at", ip.RadioID(d.Source))

	ack, _ := ars.NewAck(1)
	answer, _ := ack.AppendBinary(nil)
	reply, _ := ip.AppendIPv4(nil, &ip.Datagram{Source: d.Destination, Destination: d.Source, SourcePort: d.DestinationPort, DestinationPort: d.SourcePort, Payload: answer})
	fmt.Printf("answer %x in %d octets to %d\n", answer, len(reply), ip.RadioID(d.Source))
	// Output:
	// Device Registration Initial from 3191868 at 3191868
	// answer 0002bf01 in 32 octets to 3191868
}
