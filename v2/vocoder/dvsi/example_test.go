package dvsi_test

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

// ExampleNew decodes the AMBE+2 silence frame on the chip of an AMBEserver.
func ExampleNew() {
	var d net.Dialer
	conn, err := d.DialContext(context.Background(), "udp", "127.0.0.1:2460")
	if err != nil {
		log.Fatal(err)
	}
	c, err := dvsi.New(conn, nil)
	if err != nil {
		_ = conn.Close()
		log.Fatal(err)
	}
	pcm, _, err := c.Decode(nil, vocoder.AMBESilenceSlot())
	fmt.Println(c.Product(), len(pcm), "samples", err)
	_ = c.Close()
	_ = conn.Close()
}

// ExampleAppendChannel builds the request to decode one DMR frame and reads
// a reply.
func ExampleAppendChannel() {
	slot := vocoder.AMBESilenceSlot()
	pkt, err := dvsi.AppendChannel(nil, 0, slot[:], dvsi.FrameBits, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("% x\n", pkt)

	reply := []byte{0x61, 0x00, 0x02, 0x00, 0x3F, 0x00}
	p, n, err := dvsi.Parse(reply, false)
	if err != nil {
		log.Fatal(err)
	}
	status, ok := p.Response(dvsi.FieldParityMode)
	fmt.Println(n, len(status), status[0], ok)
	// Output:
	// 61 00 0d 01 01 48 b9 e8 81 52 61 73 00 2a 6b 2f bb
	// 6 1 0 true
}
