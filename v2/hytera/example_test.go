package hytera_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/hytera"
)

// ExampleMessage_Seal builds the registration of radio 10.0.0.80 and reads
// it back.
func ExampleMessage_Seal() {
	r := hytera.RRSRadio{Radio: [4]byte{10, 0, 0, 80}}
	payload, _ := r.AppendBinary(nil)
	m := hytera.Message{Header: hytera.Header{Service: hytera.ServiceRRS}, Opcode: hytera.RRSRegistration, Payload: payload, End: 3}
	m.Seal()
	out, _ := m.AppendBinary(nil)
	fmt.Printf("%x\n", out)

	back, _ := hytera.DecodeMessage(out)
	radio, _ := hytera.DecodeRRSRadio(back.Payload)
	fmt.Println(back.Header.Service, back.ChecksumOK(), radio.Radio)
	// Output:
	// 11000300040a000050d103
	// RRS true [10 0 0 80]
}
