package location_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) < eps }

func aprsMessages(t *testing.T) []packet.Message {
	t.Helper()
	f, err := os.Open("../layer2/testdata/anytone_aprs.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a := packet.NewAssembler(127)
	var b layer2.Burst
	var out []packet.Message
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		raw, err := hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := b.DecodeFromBytes(layer2.RawBurst(raw)); err != nil {
			t.Fatal(err)
		}
		m, done, err := a.AddBurst(&b)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			m.Data = append([]byte(nil), m.Data...)
			out = append(out, m)
		}
	}
	return out
}

func TestAnyTonePositions(t *testing.T) {
	t.Parallel()

	ms := aprsMessages(t)
	if len(ms) != 10 {
		t.Fatalf("%d packets", len(ms))
	}
	var d location.Decoder
	for i := range ms {
		f, err := d.Decode(&ms[i].Header, ms[i].Data)
		if err != nil {
			t.Fatal(err)
		}
		if f.Format != location.FormatNMEA || !f.Valid || f.Source != 3191868 || f.Destination != 3140598 ||
			!near(f.Latitude, 34+12.73/60, 1e-9) || !near(f.Longitude, 108+50.0/60, 1e-9) || !f.HasTime || f.Year != 2025 || f.Month != 12 || f.Day != 2 ||
			!f.HasCourse || f.Course != 121 || !f.HasSpeed || f.Speed != 0 || f.HasAltitude {
			t.Errorf("packet %d: %+v", i, f)
		}
	}
}

func lrrpPacket(t *testing.T, payload string) packet.Message {
	t.Helper()
	p, err := hex.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	d := ip.Datagram{SourcePort: ip.PortLRRP, DestinationPort: ip.PortLRRP, Source: ip.RadioAddress(12, 5), Destination: ip.RadioAddress(13, 7), Payload: p}
	data, err := ip.AppendIPv4(nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	return packet.Message{
		Header: pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 7, LLIDSource: 5,
		}},
		Data: data,
	}
}

func TestLRRP(t *testing.T) {
	t.Parallel()

	dec := new(location.Decoder)
	m := lrrpPacket(t, "071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006")
	f, err := dec.Decode(&m.Header, m.Data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != location.FormatLRRP || f.Source != 5 || !near(f.Latitude, 12.345345, 1e-6) || !near(f.Longitude, 24.668866, 1e-6) ||
		!f.HasRadius || f.Radius != 99.0/128 || !f.HasSpeed || f.Speed != 6.0/128 || !f.HasTime || f.Year != 2003 || f.HasCourse {
		t.Errorf("got %+v", f)
	}
	m = lrrpPacket(t, "0d0e6910000000f00000008101205647")
	f, err = dec.Decode(&m.Header, m.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasAltitude || f.Altitude != 129.25 || !f.HasCourse || f.Course != 142 || !near(f.Longitude, -22.5, 1e-9) {
		t.Errorf("point 3D %+v", f)
	}
	for _, s := range []string{"090922042468ace034313c", "110722042468ace038"} {
		m = lrrpPacket(t, s)
		if _, err := dec.Decode(&m.Header, m.Data); !errors.Is(err, location.ErrNoFix) {
			t.Errorf("%s: %v", s, err)
		}
	}
}

const hdapGPS = "A200037021225N3412.7300E10850.00000.0121"

func hdapPacket(t testing.TB, service hdap.Service, opcode hdap.Opcode, payload []byte) packet.Message {
	t.Helper()
	m := hdap.Message{Header: hdap.Header{Service: service}, Opcode: opcode, Payload: payload}
	m.Seal()
	p, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	d := ip.Datagram{SourcePort: ip.PortHyteraLocation, DestinationPort: ip.PortHyteraLocation, Source: ip.RadioAddress(12, 5), Destination: ip.RadioAddress(13, 7), Payload: p}
	data, err := ip.AppendIPv4(nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	return packet.Message{
		Header: pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 7, LLIDSource: 5,
		}},
		Data: data,
	}
}

func TestHDAP(t *testing.T) {
	t.Parallel()

	radio := []byte{10, 0, 0, 80}
	answer := append(append([]byte{0, 0, 0, 7}, radio...), append([]byte{0, 0}, hdapGPS...)...)
	report := append(append([]byte{0, 0, 0, 9}, radio...), hdapGPS...)
	emergency := append(append([]byte{}, radio...), append([]byte{1}, "V200037021225S3412.7300W10850.00001.5121"...)...)
	var d location.Decoder
	for _, tt := range []struct {
		name    string
		opcode  hdap.Opcode
		payload []byte
		valid   bool
		sign    float64
		speed   float64
	}{
		{"standard answer", hdap.LPStandardAnswer, answer, true, 1, 0},
		{"condition report", hdap.LPConditionReport, report, true, 1, 0},
		{"emergency report", hdap.LPEmergencyReport, emergency, false, -1, 1.5 * 1852 / 3600},
	} {
		m := hdapPacket(t, hdap.ServiceLP, tt.opcode, tt.payload)
		f, err := d.Decode(&m.Header, m.Data)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if f.Format != location.FormatHDAP || f.Valid != tt.valid || f.Source != 5 || f.Destination != 7 ||
			!near(f.Latitude, tt.sign*(34+12.73/60), 1e-9) || !near(f.Longitude, tt.sign*(108+50.0/60), 1e-9) ||
			!f.HasTime || f.Year != 2025 || f.Month != 12 || f.Day != 2 || f.Hour != 20 || f.Minute != 0 || f.Second != 37 ||
			!f.HasCourse || f.Course != 121 || !f.HasSpeed || !near(f.Speed, tt.speed, 1e-9) || f.HasAltitude || f.HasRadius {
			t.Errorf("%s: %+v", tt.name, f)
		}
	}
	failed := append([]byte{}, answer...)
	failed[9] = 1
	east := append([]byte{}, report...)
	copy(east[8+24:], "308")
	nospeed := append([]byte{}, report...)
	copy(nospeed[8+34:], "x.0")
	for _, tt := range []struct {
		name    string
		service hdap.Service
		opcode  hdap.Opcode
		payload []byte
		want    error
	}{
		{"failed answer", hdap.ServiceLP, hdap.LPStandardAnswer, failed, location.ErrNoFix},
		{"request", hdap.ServiceLP, hdap.LPStandardRequest, answer[:8], location.ErrNoFix},
		{"RRS", hdap.ServiceRRS, hdap.LPStandardAnswer, answer, location.ErrNoFix},
		{"short", hdap.ServiceLP, hdap.LPConditionReport, report[:20], hdap.ErrShort},
		{"longitude 308", hdap.ServiceLP, hdap.LPConditionReport, east, location.ErrNoFix},
	} {
		m := hdapPacket(t, tt.service, tt.opcode, tt.payload)
		if _, err := d.Decode(&m.Header, m.Data); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	m := hdapPacket(t, hdap.ServiceLP, hdap.LPConditionReport, nospeed)
	if f, err := d.Decode(&m.Header, m.Data); err != nil || f.HasSpeed {
		t.Errorf("bad speed: %+v, %v", f, err)
	}
	if location.FormatHDAP.String() != "HDAP" {
		t.Error("FormatHDAP name")
	}
}

func TestNotLocation(t *testing.T) {
	t.Parallel()

	var d location.Decoder
	m := packet.Message{Header: pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{}}}
	if _, err := d.Decode(&m.Header, m.Data); !errors.Is(err, location.ErrNoFix) {
		t.Errorf("DD_HEAD: %v", err)
	}
	m = lrrpPacket(t, "00")
	m.Data[22], m.Data[23] = 0x0f, 0xa7
	if _, err := d.Decode(&m.Header, m.Data); err == nil {
		t.Error("a datagram with a bad checksum decoded")
	}
	if location.FormatLRRP.String() != "LRRP" || location.Format(7).String() != "Format(7)" {
		t.Error("names")
	}
}
