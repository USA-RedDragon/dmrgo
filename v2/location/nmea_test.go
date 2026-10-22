package location_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

func nmeaPacket(t *testing.T, body string) packet.Message {
	t.Helper()
	var sum byte
	for i := range len(body) {
		sum ^= body[i]
	}
	d := ip.Datagram{SourcePort: ip.PortNMEA, DestinationPort: ip.PortNMEA, Source: ip.RadioAddress(12, 5), Destination: ip.RadioAddress(12, 7),
		Payload: fmt.Appendf(nil, "$%s*%02X\r\n", body, sum)}
	data, err := ip.AppendIPv4(nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	return packet.Message{
		Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 7, LLIDSource: 5,
		}),
		Data: data,
	}
}

func TestNMEAEmptyFields(t *testing.T) {
	t.Parallel()

	var d location.Decoder
	m := nmeaPacket(t, "GPRMC,123519,A,4807.038,N,01131.000,E,,,230394,,")
	f, err := d.Decode(&m.Header, m.Data)
	if err != nil || f.Format != location.FormatNMEA || f.HasSpeed || f.HasCourse || !f.HasTime || f.Hour != 12 || f.Day != 23 {
		t.Errorf("no speed or course: %+v, %v", f, err)
	}
	m = nmeaPacket(t, "GPRMC,,A,4807.038,N,01131.000,E,1.0,90.0,230394,,")
	f, err = d.Decode(&m.Header, m.Data)
	if err != nil || f.HasTime || !f.HasSpeed || !f.HasCourse || f.Course != 90 {
		t.Errorf("no time: %+v, %v", f, err)
	}
	m = nmeaPacket(t, "GPRMC,,V,,,,,,,230394,,")
	f, err = d.Decode(&m.Header, m.Data)
	if !errors.Is(err, location.ErrNoPosition) || f.Format != location.FormatNMEA || f.Source != 5 || f.Valid || f.HasTime || f.HasSpeed {
		t.Errorf("no position: %+v, %v", f, err)
	}
}
