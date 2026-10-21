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
