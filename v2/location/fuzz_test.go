package location_test

import (
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
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
	m := hdap.Message{Header: hdap.Header{Service: hdap.ServiceLP}, Opcode: hdap.LPConditionReport, Payload: []byte("\x00\x00\x00\x09\x0a\x00\x00\x50" + hdapGPS)}
	m.Seal()
	p, _ := m.AppendBinary(nil)
	h, _ := ip.AppendIPv4(nil, &ip.Datagram{SourcePort: ip.PortHyteraLocation, DestinationPort: ip.PortHyteraLocation, Source: ip.RadioAddress(12, 1), Destination: ip.RadioAddress(13, 2), Payload: p})
	add(uint8(enums.SAPIPBasedPacketData), h)
}

var fuzzDecoder location.Decoder

func fuzzDecode(t *testing.T, sap uint8, data []byte) {
	m := packet.Message{Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAP(sap & 0xF), LLIDDestination: 2, LLIDSource: 1,
	}), Data: data}
	f, err := fuzzDecoder.Decode(&m.Header, m.Data)
	if err != nil {
		return
	}
	if f.Source != 1 || f.Destination != 2 || math.IsNaN(f.Latitude) || f.Latitude < -90 || f.Latitude > 90 || f.Longitude < -180 || f.Longitude > 180 {
		t.Fatalf("Decode(%d, %x) = %+v", sap, data, f)
	}
	if f.Format != location.FormatNMEA {
		return
	}
	var h location.Header
	out, err := location.AppendPacket(nil, &f, &h)
	if err != nil {
		if !errors.Is(err, nmea.ErrField) || f.Speed < 1e12 {
			t.Fatalf("AppendPacket(%+v): %v", f, err)
		}
		return
	}
	g, err := fuzzDecoder.Decode(&h.Data, out)
	if err != nil || g.Valid != f.Valid || !near(g.Latitude, f.Latitude, 1e-6) || !near(g.Longitude, f.Longitude, 1e-6) ||
		g.HasSpeed != f.HasSpeed || !near(g.Speed, f.Speed, 0.003+f.Speed*1e-12) || g.HasCourse != f.HasCourse || !near(g.Course, f.Course, 0.005) ||
		g.HasTime != f.HasTime || g.Year != f.Year || g.Month != f.Month || g.Day != f.Day || g.Hour != f.Hour || g.Minute != f.Minute || g.Second != f.Second {
		t.Fatalf("Decode(AppendPacket(%+v)) = %+v, %v", f, g, err)
	}
}
