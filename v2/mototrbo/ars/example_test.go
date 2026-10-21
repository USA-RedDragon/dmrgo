package ars_test

import (
	"fmt"

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
