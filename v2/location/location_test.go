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
