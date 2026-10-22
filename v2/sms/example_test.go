package sms_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

var header sms.Header

// Example builds a MOTOTRBO text message as an unconfirmed packet, segments
// it into rate 1/2 blocks and reads the text back.
func Example() {
	t := sms.Text{Format: sms.FormatTMS, Source: 3191868, Destination: 3140598, Body: []byte("Hello")}
	data, err := sms.AppendPacket(nil, &t, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header.Data, data, enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	m := packet.Message{Header: p.Header, Data: data}
	_, got, err := sms.Decode(nil, &m.Header, m.Data)
	fmt.Printf("%d blocks, %s from %d: %s %v\n", len(p.Blocks), got.Format, got.Source, got.Body, err)
	// Output:
	// 5 blocks, TMS from 3191868: Hello <nil>
}
