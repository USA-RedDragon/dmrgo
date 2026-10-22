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
	"github.com/USA-RedDragon/dmrgo/v2/ip"
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
		out, err := sms.AppendPacket([]byte("x"), &tt.text, &h)
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
			in := sms.Text{Format: f, Source: 1234, Destination: 99, Group: confirmed, Confirmed: confirmed, DD: enums.DDUTF16LE, Sequence: 7, AckRequired: true, Body: []byte("Grüße €")}
			var h sms.Header
			out, err := sms.AppendPacket(nil, &in, &h)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			m := packet.Message{Header: h.Data, Data: out}
			_, got, err := sms.Decode(nil, &m.Header, m.Data)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if got.Format != f || string(got.Body) != string(in.Body) || got.Source != 1234 || got.Destination != 99 || got.Group != confirmed || got.Confirmed != confirmed {
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
	if _, err := sms.AppendPacket(nil, &sms.Text{Format: 99}, &h); !errors.Is(err, sms.ErrFormat) {
		t.Errorf("format 99: %v", err)
	}
	if b, err := sms.AppendPacket([]byte("x"), &sms.Text{Format: sms.FormatDefinedData, DD: enums.DDBinary, Body: []byte("1")}, &h); !errors.Is(err, sms.ErrFormat) || string(b) != "x" {
		t.Errorf("binary text built: %q, %v", b, err)
	}
	for _, tt := range []struct {
		format sms.Format
		body   string
	}{
		{sms.FormatHytera, ""}, {sms.FormatHytera, "\x00"}, {sms.FormatHytera, "\x00A"}, {sms.FormatHytera, "A\x00B"},
		{sms.FormatIP, "\r\n"}, {sms.FormatIP, "\r\nA"}, {sms.FormatIP, "\r\n\x00"}, {sms.FormatIP, "\r\nA\x00"}, {sms.FormatIP, "\r\n中\x00\x00"},
	} {
		if b, err := sms.AppendPacket([]byte("x"), &sms.Text{Format: tt.format, Body: []byte(tt.body)}, &h); !errors.Is(err, sms.ErrFormat) || string(b) != "x" {
			t.Errorf("%s %q built: %x, %v", tt.format, tt.body, b, err)
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

func TestBadChecksum(t *testing.T) {
	t.Parallel()

	for _, at := range []int{10, 26} {
		m := ipMessage(t, appcapture.TMS)
		m.Data[at] ^= 0x01
		_, got, err := sms.Decode(nil, &m.Header, m.Data)
		if !errors.Is(err, ip.ErrChecksum) || got.Format != sms.FormatTMS || string(got.Body) != "MOTO TEST" {
			t.Errorf("octet %d: %+v %q, %v", at, got, got.Body, err)
		}
	}
}

func TestBCDShortData(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		digits, data string
	}{
		{"5551234", "5551234f"},
		{"*31#", "a31b"},
		{"1234560000000000", "1234560000000000ff"},
		{"0000", "0000"},
	} {
		in := sms.Text{Format: sms.FormatDefinedData, DD: enums.DDBCD, Source: 3191868, Destination: 3140598, Body: []byte(tt.digits)}
		var h sms.Header
		out, err := sms.AppendPacket(nil, &in, &h)
		dd, ok := h.Data.AsDefinedData()
		if err != nil || hex.EncodeToString(out) != tt.data || !ok || dd.DefinedData != enums.DDBCD {
			t.Fatalf("%s: %x, %v", tt.digits, out, err)
		}
		_, got, err := sms.Decode(nil, &h.Data, out)
		if err != nil || got.Format != sms.FormatDefinedData || got.DD != enums.DDBCD || string(got.Body) != tt.digits {
			t.Errorf("%s: decoded %+v %q, %v", tt.digits, got, got.Body, err)
		}
	}
	hsms := unhex(t, appcapture.HSMS)
	h := setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{DefinedData: enums.DDBCD})
	for _, tt := range []struct {
		data   []byte
		format sms.Format
	}{
		{hsms, sms.FormatHytera},
		{append(append([]byte{}, hsms...), 0, 0), sms.FormatHytera},
		{append(append([]byte{}, hsms...), 1), sms.FormatDefinedData},
		{append(append([]byte{}, hsms...), 0, 1), sms.FormatDefinedData},
		{hsms[:len(hsms)-2], sms.FormatDefinedData},
		{unhex(t, "000041000000"), sms.FormatHytera},
		{unhex(t, "000000000000"), sms.FormatDefinedData},
	} {
		if _, got, err := sms.Decode(nil, &h, tt.data); err != nil || got.Format != tt.format {
			t.Errorf("%x: %v, %v, want %v", tt.data, got.Format, err, tt.format)
		}
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
		in.Confirmed = true
		out, err := sms.AppendPacket(nil, &in, &h)
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

func TestAppendPacketAfterDst(t *testing.T) {
	t.Parallel()

	for _, f := range []sms.Format{sms.FormatTMS, sms.FormatIP, sms.FormatAnyTone, sms.FormatDefinedData, sms.FormatHytera} {
		in := sms.Text{Format: f, Source: 1, Destination: 2, DD: enums.DDUTF16LE, Body: []byte("hi")}
		var h sms.Header
		out, err := sms.AppendPacket([]byte("xyz"), &in, &h)
		if err != nil || string(out[:3]) != "xyz" {
			t.Fatalf("%s: %x, %v", f, out, err)
		}
		if _, got, err := sms.Decode(nil, &h.Data, out[3:]); err != nil || got.Format != f || string(got.Body) != "hi" {
			t.Errorf("%s: decoded %+v %q, %v", f, got, got.Body, err)
		}
	}
}

func TestPort5016FirstCharacter(t *testing.T) {
	t.Parallel()

	bodies := []string{"", "A", "中文", "一", "€5", "Ā", "ÿ", "\x7fA", "😀", "\r\n", "\r\nA", "\r\n中", "\r\nA\x00B", "\x00", "\rA", "\n\r"}
	for _, f := range []sms.Format{sms.FormatIP, sms.FormatAnyTone} {
		for _, body := range bodies {
			if f == sms.FormatIP && strings.HasPrefix(body, "\r\n") {
				continue
			}
			in := sms.Text{Format: f, Source: 1, Destination: 2, Body: []byte(body)}
			var h sms.Header
			out, err := sms.AppendPacket(nil, &in, &h)
			if err != nil {
				t.Fatalf("%s %q: %v", f, body, err)
			}
			want, _, _ := strings.Cut(body, "\x00")
			if _, got, err := sms.Decode(nil, &h.Data, out); err != nil || got.Format != f || string(got.Body) != want {
				t.Errorf("%s %q: decoded %s %q, %v", f, body, got.Format, got.Body, err)
			}
		}
	}
}

func TestPort5016SamePayload(t *testing.T) {
	t.Parallel()

	var h sms.Header
	out, err := sms.AppendPacket(nil, &sms.Text{Format: sms.FormatAnyTone, Body: []byte("䄀")}, &h)
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "000d000a00410000"); !bytes.HasSuffix(out, want) {
		t.Fatalf("AnyTone 䄀: %x, want %x, the UTF-16BE of the IP text CR LF A NUL", out, want)
	}
	if _, err := sms.AppendPacket(nil, &sms.Text{Format: sms.FormatIP, Body: []byte("\r\nA\x00")}, &h); !errors.Is(err, sms.ErrFormat) {
		t.Fatalf("IP CR LF A NUL: %v", err)
	}
}

func TestPort5016Senders(t *testing.T) {
	t.Parallel()

	d878 := ipMessage(t, appcapture.D878Text)
	hytera := ipMessage(t, appcapture.HyteraIPText)
	u, _ := hytera.Header.AsUnconfirmed()
	u.SAP = enums.SAPUDPIPHeaderCompression
	if err := hytera.Header.SetUnconfirmed(u); err != nil {
		t.Fatal(err)
	}
	da7x2 := ipMessage(t, appcapture.DMRText)
	pro0 := ipMessage(t, appcapture.BTECH6X2ProTest)
	pro1 := ipMessage(t, appcapture.BTECH6X2ProHello1)
	pro2 := ipMessage(t, appcapture.BTECH6X2ProHello2)
	for _, tt := range []struct {
		name   string
		m      *packet.Message
		format sms.Format
		body   string
	}{
		{"AnyTone D878, no NUL", &d878, sms.FormatAnyTone, "TEST"},
		{"BTECH 6X2 Pro 1, no NUL", &pro0, sms.FormatAnyTone, "TEST"},
		{"BTECH 6X2 Pro 2, no NUL", &pro1, sms.FormatAnyTone, "HELLO"},
		{"BTECH 6X2 Pro 3, no NUL", &pro2, sms.FormatAnyTone, "HELLO"},
		{"AnyTone DA-7X2, NUL", &da7x2, sms.FormatAnyTone, "DMR TEST"},
		{"Hytera, UTF-16BE", &hytera, sms.FormatIP, "Test Message Hytera to DSDFME"},
	} {
		if _, got, err := sms.Decode(nil, &tt.m.Header, tt.m.Data); err != nil || got.Format != tt.format || string(got.Body) != tt.body {
			t.Errorf("%s: %s %q, %v", tt.name, got.Format, got.Body, err)
		}
	}
}

func TestPort5016Lead(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		payload, body string
		format        sms.Format
	}{
		{"000d000a", "", sms.FormatAnyTone},
		{"000d000a2d4e8765", "中文", sms.FormatAnyTone},
		{"000d000a41004200", "AB", sms.FormatAnyTone},
		{"000d000a410042000000", "AB", sms.FormatAnyTone},
		{"000d000a4100420000", "AB", sms.FormatAnyTone},
		{"000d000a00410042", "䄀䈀", sms.FormatAnyTone},
		{"000d000a480045004c004c004f0000", "HELLO", sms.FormatAnyTone},
		{"000d00", "\r", sms.FormatIP},
		{"000d0041", "\rA", sms.FormatIP},
		{"00410042", "AB", sms.FormatIP},
		{"0041004200000000", "AB", sms.FormatIP},
		{"", "", sms.FormatIP},
	} {
		var h sms.Header
		out, err := sms.AppendPacket(nil, &sms.Text{Format: sms.FormatIP, Source: 1, Destination: 2, Body: []byte("x")}, &h)
		if err != nil {
			t.Fatal(err)
		}
		p := unhex(t, tt.payload)
		out = append(out[:28:28], p...)
		out[2], out[3] = byte(len(out)>>8), byte(len(out))
		out[24], out[25] = byte((8+len(p))>>8), byte(8+len(p))
		out[10], out[11], out[26], out[27] = 0, 0, 0, 0
		if _, got, err := sms.Decode(nil, &h.Data, out); (err != nil && !errors.Is(err, ip.ErrChecksum)) || got.Format != tt.format || string(got.Body) != tt.body {
			t.Errorf("%s: %s %q, %v", tt.payload, got.Format, got.Body, err)
		}
	}
}
