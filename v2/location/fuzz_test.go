package location_test

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(sap uint8, data []byte) { f.Add(sap, data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(sap uint8, data []byte)) {
	g, _ := hex.DecodeString(appcapture.GPS)
	add(uint8(enums.SAPUDPIPHeaderCompression), g)
	l, _ := hex.DecodeString("00010000000fa10fa1071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006")
	add(uint8(enums.SAPUDPIPHeaderCompression), l)
}

var fuzzDecoder location.Decoder

func fuzzDecode(t *testing.T, sap uint8, data []byte) {
	m := packet.Message{Header: pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		SAP: enums.ServiceAccessPointID(sap & 0xF), LLIDDestination: 2, LLIDSource: 1,
	}}, Data: data}
	f, err := fuzzDecoder.Decode(&m.Header, m.Data)
	if err != nil {
		return
	}
	if f.Source != 1 || f.Destination != 2 || math.IsNaN(f.Latitude) || f.Latitude < -90 || f.Latitude > 90 || f.Longitude < -180 || f.Longitude > 180 {
		t.Fatalf("Decode(%d, %x) = %+v", sap, data, f)
	}
}
