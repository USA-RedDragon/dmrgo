package location_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
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

func setHeader[P any](t testing.TB, set func(*pdu.DataHeader, P) error, p P) pdu.DataHeader {
	t.Helper()
	var h pdu.DataHeader
	if err := set(&h, p); err != nil {
		t.Fatal(err)
	}
	return h
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
		Header: setHeader(t, (*pdu.DataHeader).SetConfirmed, pdu.ConfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 7, LLIDSource: 5,
		}),
		Data: data,
	}
}

func TestBadChecksum(t *testing.T) {
	t.Parallel()

	var dec location.Decoder
	for _, at := range []int{10, 26} {
		m := lrrpPacket(t, "071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006")
		m.Data[at] ^= 0x01
		f, err := dec.Decode(&m.Header, m.Data)
		if !errors.Is(err, ip.ErrChecksum) || f.Format != location.FormatLRRP || !near(f.Latitude, 12.345345, 1e-6) {
			t.Errorf("octet %d: %+v, %v", at, f, err)
		}
	}
}

func TestLRRPWholeSpeed(t *testing.T) {
	t.Parallel()

	m := lrrpPacket(t, "1315232f341f4ad07b2e66474326660a4d56e46b0b5620")
	f, err := new(location.Decoder).Decode(&m.Header, m.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasSpeed || f.Speed != 11 || !near(f.Latitude, 50.106308, 1e-6) {
		t.Errorf("whole speed %+v", f)
	}
}

func TestLRRPWholeShapes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		payload string
		lat     float64
		alt     float64
		radius  bool
	}{
		{"0d19220431323334341fa5fb128053b13a24bdd67ac6c202804403", -34.612821, 68, true},
		{"0711341fa5ca6d3467b13a5784d67ab88a8042", -34.613366, 66, false},
	} {
		m := lrrpPacket(t, c.payload)
		f, err := new(location.Decoder).Decode(&m.Header, m.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !f.Valid || !near(f.Latitude, c.lat, 1e-6) || !f.HasAltitude || f.Altitude != c.alt || f.HasRadius != c.radius {
			t.Errorf("%s: %+v", c.payload, f)
		}
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
		!f.HasRadius || f.Radius != 99.0/128 || !f.HasSpeed || f.Speed != 6.0/128 || !f.HasTime || f.Year != 2003 || f.HasCourse ||
		f.HasResult || hex.EncodeToString(f.RequestID[:f.RequestIDLength]) != "2468ace0" {
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
	m = lrrpPacket(t, "090922042468ace034313c")
	if _, err := dec.Decode(&m.Header, m.Data); !errors.Is(err, location.ErrNoFix) {
		t.Errorf("request: %v", err)
	}
	for _, tt := range []struct {
		data   string
		result uint32
		id     string
	}{
		{"110722042468ace038", 0, "2468ace0"},
		{"070c22042468ace0390503515355", 5, "2468ace0"},
		{"07082204000000073710", uint32(lrrp.ResultNoGPS), "00000007"},
		{"07052307378400", uint32(lrrp.ResultGPSInitializing), "07"},
	} {
		m = lrrpPacket(t, tt.data)
		f, err := dec.Decode(&m.Header, m.Data)
		if !errors.Is(err, location.ErrNoPosition) || f.Format != location.FormatLRRP || f.Source != 5 || f.Valid ||
			!f.HasResult || f.Result != tt.result || hex.EncodeToString(f.RequestID[:f.RequestIDLength]) != tt.id {
			t.Errorf("%s: %+v, %v", tt.data, f, err)
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
		Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 7, LLIDSource: 5,
		}),
		Data: data,
	}
}

func TestHDAPFailedAnswer(t *testing.T) {
	t.Parallel()

	failed := append([]byte{0, 0, 0, 7, 10, 0, 0, 80, 0, 1}, hdapGPS...)
	var d location.Decoder
	m := hdapPacket(t, hdap.ServiceLP, hdap.LPStandardAnswer, failed)
	f, err := d.Decode(&m.Header, m.Data)
	if !errors.Is(err, location.ErrNoPosition) || f.Format != location.FormatHDAP || f.Valid || !f.HasResult || f.Result != 1 ||
		f.RequestIDLength != 4 || [4]byte(f.RequestID[:4]) != [4]byte{0, 0, 0, 7} {
		t.Errorf("%+v, %v", f, err)
	}
}

func TestHDAPNulFields(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString("08a002003200000003002337fb0000410000000000000000000000004e353030332e383737314530313432362e353330320000000000007003")
	if err != nil {
		t.Fatal(err)
	}
	m, err := hdap.DecodeMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	p := hdapPacket(t, m.Header.Service, m.Opcode, m.Payload)
	var d location.Decoder
	f, err := d.Decode(&p.Header, p.Data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != location.FormatHDAP || !f.Valid || !near(f.Latitude, 50+3.8771/60, 1e-9) || !near(f.Longitude, 14+26.5302/60, 1e-9) ||
		f.HasTime || f.HasSpeed || f.HasCourse || !f.HasResult || f.Result != 0 {
		t.Errorf("%+v", f)
	}
	half := append([]byte{}, m.Payload...)
	copy(half[10+7:], "221026")
	p = hdapPacket(t, m.Header.Service, m.Opcode, half)
	if f, err := d.Decode(&p.Header, p.Data); err != nil || f.HasTime {
		t.Errorf("date without a time: %+v, %v", f, err)
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
		{"failed answer", hdap.ServiceLP, hdap.LPStandardAnswer, failed, location.ErrNoPosition},
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
	m := packet.Message{Header: setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{})}
	if _, err := d.Decode(&m.Header, m.Data); !errors.Is(err, location.ErrNoFix) {
		t.Errorf("DD_HEAD: %v", err)
	}
	m = lrrpPacket(t, "00")
	m.Data[22], m.Data[23] = 0x0f, 0xa7
	if _, err := d.Decode(&m.Header, m.Data); err == nil {
		t.Error("a datagram with a bad checksum decoded")
	}
	if _, err := d.Decode(nil, m.Data); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil header: %v", err)
	}
	if location.FormatLRRP.String() != "LRRP" || location.Format(7).String() != "Format(7)" {
		t.Error("names")
	}
}
