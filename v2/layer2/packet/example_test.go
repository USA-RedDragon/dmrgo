package packet_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func ExampleNewPacket() {
	var header pdu.DataHeader
	err := header.SetUnconfirmedDataHeader(pdu.UnconfirmedDataHeader{
		SAP:             enums.SAPShortData,
		LLIDDestination: 91,
		LLIDSource:      3120000,
		FullMessageFlag: true,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header, []byte("hello, DMR"), enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	h, _ := p.Header.AsUnconfirmedDataHeader()
	fmt.Println(len(p.Blocks), h.BlocksToFollow, h.PadOctetCount)

	a := packet.NewAssembler(127)
	if _, _, err := a.Add(&p.Header); err != nil {
		fmt.Println(err)
		return
	}
	for _, b := range p.Blocks {
		msg, done, err := a.Add(b)
		if err != nil {
			fmt.Println(err)
			return
		}
		if done {
			fmt.Printf("%q\n", msg.Data)
		}
	}
	// Output:
	// 2 2 10
	// "hello, DMR"
}

func ExampleMessage_UDPIPv4() {
	udp := pdu.UDPIPv4CompressedHeader{IPv4Identification: 1, ExtendedHeader1: 10110, ExtendedHeader2: 10110}
	raw, err := udp.Encode()
	if err != nil {
		fmt.Println(err)
		return
	}
	sentence := "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"
	var header pdu.DataHeader
	err = header.SetUnconfirmedDataHeader(pdu.UnconfirmedDataHeader{
		SAP:             enums.SAPUDPIPHeaderCompression,
		LLIDDestination: 3140598,
		LLIDSource:      3191868,
		FullMessageFlag: true,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header, append(raw[:], sentence...), enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}

	a := packet.NewAssembler(127)
	var msg packet.Message
	for _, d := range append([]layer2.Data{&p.Header}, p.Blocks...) {
		if msg, _, err = a.Add(d); err != nil {
			fmt.Println(err)
			return
		}
	}
	h, payload, ok := msg.UDPIPv4()
	src, dst := ip.CompressedPorts(&h)
	fmt.Println(ok, h.Octets(), uint16(src), uint16(dst))
	r, err := nmea.ParseRMC(payload)
	fmt.Printf("%.4f %.4f %t %v\n", r.Latitude, r.Longitude, r.Valid, err)
	// Output:
	// true 9 10110 10110
	// 34.2122 108.8333 true <nil>
}
