package ip_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

// ExampleDecode decodes the IPv4 packet of a MOTOTRBO text message that an
// AnyTone radio sent, and builds it again.
func ExampleDecode() {
	data, _ := hex.DecodeString(appcapture.TMS)
	d, err := ip.Decode(enums.SAPIPBasedPacketData, data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(ip.ID(d.Source), "to", ip.ID(d.Destination), "port", d.DestinationPort, len(d.Payload), "octets")
	out, _ := ip.AppendIPv4(nil, &d)
	fmt.Println(hex.EncodeToString(out) == appcapture.TMS)
	// Output:
	// 3191868 to 3140598 port TMS 30 octets
	// true
}

// ExampleAppendICMPPacket builds the Host Unreachable that the radio that
// got a text message sends back when it cannot deliver it, and decodes it.
func ExampleAppendICMPPacket() {
	tms, _ := hex.DecodeString(appcapture.TMS)
	c := ip.ICMPPacket{
		Source:      ip.RadioAddress(ip.DefaultNetwork, 3140598),
		Destination: ip.RadioAddress(ip.DefaultNetwork, 3191868),
		Message:     ip.ICMPHostUnreachable,
		Original:    tms,
	}
	out, _ := ip.AppendICMPPacket(nil, &c)
	fmt.Println(len(out), "octets")
	back, err := ip.DecodeICMPPacket(out)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(back.Message, "to", ip.ID(back.Destination), "about", len(back.Original), "octets")
	m, pointer, ok := ip.CheckIPv4(tms, 576)
	fmt.Println(m, pointer, ok)
	// Output:
	// 56 octets
	// Host Unreachable to 3191868 about 28 octets
	// ICMPMessage(type 0, code 0) 0 false
}
