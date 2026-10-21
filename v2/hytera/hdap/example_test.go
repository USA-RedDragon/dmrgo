package hdap_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
)

// ExampleMessage_Seal builds the registration of radio 10.0.0.80 and reads
// it back.
func ExampleMessage_Seal() {
	r := hdap.RRSRadio{Radio: [4]byte{10, 0, 0, 80}}
	payload, _ := r.AppendBinary(nil)
	m := hdap.Message{Header: hdap.Header{Service: hdap.ServiceRRS}, Opcode: hdap.RRSRegistration, Payload: payload, End: 3}
	m.Seal()
	out, _ := m.AppendBinary(nil)
	fmt.Printf("%x\n", out)

	back, _ := hdap.DecodeMessage(out)
	radio, _ := hdap.DecodeRRSRadio(back.Payload)
	fmt.Println(back.Header.Service, back.ChecksumOK(), radio.Radio)
	// Output:
	// 11000300040a000050d103
	// RRS true [10 0 0 80]
}
