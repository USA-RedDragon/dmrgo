package sms_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func received(t testing.TB, name string) packet.Message {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	a := packet.NewAssembler(127)
	var b layer2.Burst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		raw, err := hex.DecodeString(fields[1])
		if err != nil || len(raw) != 33 {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		if err := b.DecodeFromBytes(layer2.RawBurst(raw)); err != nil {
			t.Fatal(err)
		}
		m, done, err := a.AddBurst(&b)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			return m
		}
	}
	t.Fatalf("%s: no packet", name)
	return packet.Message{}
}

func setHeader[P any](t testing.TB, set func(*pdu.DataHeader, P) error, p P) pdu.DataHeader {
	t.Helper()
	var h pdu.DataHeader
	if err := set(&h, p); err != nil {
		t.Fatal(err)
	}
	return h
}

func ipMessage(t testing.TB, s string) packet.Message {
	t.Helper()
	return packet.Message{
		Header: setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3140598, LLIDSource: 3191868, FullMessageFlag: true,
		}),
		Data: unhex(t, s),
	}
}

func TestReceivedTexts(t *testing.T) {
	t.Parallel()

	dmr := received(t, "anytone_dmr_text.txt")
	hsms := received(t, "anytone_hsms.txt")
	tms := ipMessage(t, appcapture.TMS)
	tests := []struct {
		name   string
		m      *packet.Message
		format sms.Format
		body   string
	}{
		{"DMR Standard on air", &dmr, sms.FormatAnyTone, "DMR TEST"},
		{"H-SMS on air", &hsms, sms.FormatHytera, "HYT TEST"},
		{"M-SMS over IPSC", &tms, sms.FormatTMS, "MOTO TEST"},
	}
	for _, tt := range tests {
		out, got, err := sms.Decode([]byte("x"), &tt.m.Header, tt.m.Data)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got.Format != tt.format || string(got.Body) != tt.body || string(out) != "x"+tt.body || got.Source != 3191868 || got.Destination != 3140598 || got.Group {
			t.Errorf("%s: %+v, %q", tt.name, got, got.Body)
		}
	}
}

func TestAppendPacketMatchesRadios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text sms.Text
		want string
	}{
		{sms.Text{Format: sms.FormatTMS, Source: 3191868, Destination: 3140598, Sequence: 1, Body: []byte("MOTO TEST\x00")}, appcapture.TMS},
		{sms.Text{Format: sms.FormatAnyTone, Source: 3191868, Destination: 3140598, Body: []byte("DMR TEST")}, appcapture.DMRText},
	}
	for _, tt := range tests {
		var h sms.Header
		out, err := sms.AppendPacket([]byte("x"), &tt.text, false, &h)
		if err != nil {
			t.Fatal(err)
		}
		want := unhex(t, tt.want)
		if u, ok := h.Data.AsUnconfirmed(); out[0] != 'x' || !ok || u.SAP != enums.SAPIPBasedPacketData ||
			u.LLIDSource != 3191868 {
			t.Fatalf("%s: header %s", tt.text.Format, h.Data.String())
		}
		got := out[1:]
		if len(got) != len(want) || !bytes.Equal(got[:4], want[:4]) || !bytes.Equal(got[6:10], want[6:10]) || !bytes.Equal(got[12:], want[12:]) {
			t.Errorf("%s: %x, want %x", tt.text.Format, got, want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	for _, f := range []sms.Format{sms.FormatTMS, sms.FormatIP, sms.FormatAnyTone, sms.FormatDefinedData, sms.FormatHytera} {
		for _, confirmed := range []bool{false, true} {
			in := sms.Text{Format: f, Source: 1234, Destination: 99, Group: confirmed, DD: enums.DDUTF16LE, Sequence: 7, AckRequired: true, Body: []byte("Grüße €")}
			var h sms.Header
			out, err := sms.AppendPacket(nil, &in, confirmed, &h)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			m := packet.Message{Header: h.Data, Data: out}
			_, got, err := sms.Decode(nil, &m.Header, m.Data)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if got.Format != f || string(got.Body) != string(in.Body) || got.Source != 1234 || got.Destination != 99 || got.Group != confirmed {
				t.Errorf("%s: %+v %q", f, got, got.Body)
			}
			if f == sms.FormatTMS && (got.Sequence != 7 || !got.AckRequired) {
				t.Errorf("TMS: %+v", got)
			}
		}
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	m := ipMessage(t, appcapture.GPS)
	u, ok := m.Header.AsUnconfirmed()
	if !ok {
		t.Fatal("GPS header is not a U_HEAD")
	}
	u.SAP = enums.SAPUDPIPHeaderCompression
	if err := m.Header.SetUnconfirmed(u); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sms.Decode(nil, &m.Header, m.Data); !errors.Is(err, sms.ErrNotText) {
		t.Errorf("GPS: %v", err)
	}
	ack := ipMessage(t, appcapture.TMS)
	resp := setHeader(t, (*pdu.DataHeader).SetResponse, pdu.ResponsePacketHeader{})
	if _, _, err := sms.Decode(nil, &resp, nil); !errors.Is(err, sms.ErrNotText) {
		t.Errorf("response: %v", err)
	}
	ack.Data = ack.Data[:30]
	if _, _, err := sms.Decode(nil, &ack.Header, ack.Data); err == nil {
		t.Error("cut packet decoded")
	}
	var h sms.Header
	if _, err := sms.AppendPacket(nil, &sms.Text{Format: 99}, false, &h); !errors.Is(err, sms.ErrFormat) {
		t.Errorf("format 99: %v", err)
	}
	for _, dd := range []enums.DDFormat{enums.DDBCD, enums.DDBinary} {
		if b, err := sms.AppendPacket([]byte("x"), &sms.Text{Format: sms.FormatDefinedData, DD: dd, Body: []byte("1")}, false, &h); !errors.Is(err, sms.ErrFormat) || string(b) != "x" {
			t.Errorf("%s text built: %q, %v", dd, b, err)
		}
	}
	bin := setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{DefinedData: enums.DDBinary})
	if _, _, err := sms.Decode(nil, &bin, []byte("Hi")); !errors.Is(err, sms.ErrNotText) {
		t.Errorf("binary: %v", err)
	}
	if sms.FormatHytera.String() != "Hytera" || sms.Format(9).String() != "Format(9)" {
		t.Error("names")
	}
}

func TestAppendPacketAnyToneNUL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body, udp string
		ipLen     int
	}{
		{"C4 one", "13981398001a46df000d000a4300340020006f006e0065000000", 46},
		{"C4 x", "139813980016", 42},
	}
	for _, tt := range tests {
		in := sms.Text{Format: sms.FormatAnyTone, Source: 3140598, Destination: 3191868, Body: []byte(tt.body)}
		var h sms.Header
		out, err := sms.AppendPacket(nil, &in, true, &h)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != tt.ipLen || int(out[2])<<8|int(out[3]) != tt.ipLen {
			t.Errorf("%q: IP length %d, want %d", tt.body, len(out), tt.ipLen)
		}
		if want := unhex(t, tt.udp); len(out) < 20+len(want) || !bytes.Equal(out[20:20+len(want)], want) {
			t.Errorf("%q: UDP %x, want %x", tt.body, out[20:], want)
		}
		if !bytes.HasSuffix(out, []byte{0, 0}) || bytes.HasSuffix(out, []byte{0, 0, 0, 0}) {
			t.Errorf("%q: not one NUL: %x", tt.body, out)
		}
		_, got, err := sms.Decode(nil, &h.Data, out)
		if err != nil || got.Format != sms.FormatAnyTone || string(got.Body) != tt.body {
			t.Errorf("%q: decoded %+v %q, %v", tt.body, got, got.Body, err)
		}
	}
}
