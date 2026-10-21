package location_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

var decoder location.Decoder

// ExampleDecoder_Decode reads the position report of an AnyTone radio.
func ExampleDecoder_Decode() {
	data, _ := hex.DecodeString(appcapture.GPS)
	m := packet.Message{
		Header: pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: 3140598, LLIDSource: 3191868,
		}},
		Data: data,
	}
	f, err := decoder.Decode(&m.Header, m.Data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s from %d: %.5f %.5f %.0f deg\n", f.Format, f.Source, f.Latitude, f.Longitude, f.Course)
	// Output:
	// NMEA from 3191868: 34.21217 108.83333 121 deg
}
