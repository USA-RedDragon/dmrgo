package tms_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
)

// ExampleNewText builds a text message that asks for an acknowledgement,
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
