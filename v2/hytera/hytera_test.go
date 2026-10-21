package hytera_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera"
)

const gps = "A200037021225N3412.7300E10850.00000.0121"

func TestRRSRegistration(t *testing.T) {
	t.Parallel()

	r := hytera.RRSRadio{Radio: [4]byte{10, 0, 0, 80}}
	payload, err := r.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := hytera.Message{Header: hytera.Header{Service: hytera.ServiceRRS}, Opcode: hytera.RRSRegistration, Payload: payload, End: 3}
	m.Seal()
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(out); got != "1100030004"+"0a000050"+"d103" {
		t.Fatalf("message %s", got)
	}
	back, err := hytera.DecodeMessage(out)
	if err != nil || !back.ChecksumOK() || back.Header.Service != hytera.ServiceRRS || back.Opcode != hytera.RRSRegistration {
		t.Fatalf("decoded %s, %v", back.String(), err)
	}
	rr, err := hytera.DecodeRRSRadio(back.Payload)
	if err != nil || rr.Radio != r.Radio {
		t.Errorf("radio %v, %v", rr.Radio, err)
	}
	out[6] ^= 1
	if bad, err := hytera.DecodeMessage(out); err != nil || bad.ChecksumOK() {
		t.Errorf("a changed payload passes the checksum: %v", err)
	}
}

func TestLocationAnswer(t *testing.T) {
	t.Parallel()

	data := append([]byte{0, 0, 0, 7, 10, 0, 0, 80, 0, 0}, gps...)
	a, err := hytera.DecodeLPAnswer(data)
	if err != nil {
		t.Fatal(err)
	}
	g := &a.GPS
	if a.RequestID != 7 || a.Result != 0 || !g.Valid() || g.Hour != 20 || g.Second != 37 || g.Day != 2 || g.Month != 12 || g.Year != 25 ||
		math.Abs(g.Latitude()-(34+12.73/60)) > 1e-9 || math.Abs(g.Longitude()-(108+50.0/60)) > 1e-9 || string(g.Speed[:]) != "0.0" || g.Direction != 121 {
		t.Fatalf("got %s", a.String())
	}
	out, err := a.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Errorf("AppendBinary = %q, %v", out, err)
	}
	south := append([]byte{}, data...)
	south[10+13], south[10+23] = 'S', 'W'
	b, err := hytera.DecodeLPAnswer(south)
	if err != nil || b.GPS.Latitude() > 0 || b.GPS.Longitude() > 0 {
		t.Errorf("south west %v %v, %v", b.GPS.Latitude(), b.GPS.Longitude(), err)
	}
	broken := append([]byte{}, data...)
	broken[10+18] = ','
	if _, err := hytera.DecodeLPAnswer(broken); !errors.Is(err, hytera.ErrSignature) {
		t.Errorf("no point: %v", err)
	}
	broken[10+18], broken[10+2] = '.', 'x'
	if _, err := hytera.DecodeLPAnswer(broken); !errors.Is(err, hytera.ErrText) {
		t.Errorf("letter in the hour: %v", err)
	}
}

func TestTextMessage(t *testing.T) {
	t.Parallel()

	var m hytera.TMPMessage
	m.RequestID, m.Destination, m.Source = 1, [4]byte{10, 0, 0, 2}, [4]byte{10, 0, 0, 1}
	m.SetTextUTF8(nil, []byte("Hello"))
	payload, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	flags, err := (&hytera.TMPFlags{}).AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := hytera.Message{Header: hytera.Header{Reliable: true, Service: hytera.ServiceTMP}, Opcode: uint16(flags[0])<<8 | uint16(hytera.TMPPrivateMessage), Payload: payload, End: 3}
	msg.Seal()
	out, err := msg.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0x89 || out[1] != 0x00 || out[2] != 0xA1 {
		t.Errorf("header %x", out[:3])
	}
	back, err := hytera.DecodeMessage(out)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := hytera.DecodeTMPMessage(back.Payload)
	if err != nil || string(tm.TextUTF8(nil)) != "Hello" || tm.Source != m.Source {
		t.Errorf("text %q, %v", tm.TextUTF8(nil), err)
	}
	ack := hytera.TMPAnswer{RequestID: 1, Destination: m.Destination, Source: m.Source, Result: 4}
	b, _ := ack.AppendBinary(nil)
	if back, err := hytera.DecodeTMPAnswer(b); err != nil || back.Result != 4 {
		t.Errorf("answer %v", err)
	}
	g := hytera.TMPGroupAnswer{RequestID: 2, Destination: [4]byte{225, 0, 0, 9}}
	b, _ = g.AppendBinary(nil)
	if back, err := hytera.DecodeTMPGroupAnswer(b); err != nil || back.RequestID != 2 {
		t.Errorf("group answer %v", err)
	}
}

func TestMessageErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		data []byte
		want error
	}{
		{nil, hytera.ErrShort},
		{[]byte{0x11, 0, 3, 0, 4, 1, 2}, hytera.ErrShort},
		{[]byte{0x11, 0, 3, 0, 0, 0xd1, 4}, hytera.ErrSignature},
		{[]byte{0x11, 0, 3, 0, 0, 0xd1, 3, 0}, hytera.ErrLength},
	} {
		if _, err := hytera.DecodeMessage(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%x: %v, want %v", tt.data, err, tt.want)
		}
	}
	if hytera.ServiceLP.String() != "LP" || hytera.Service(0x7F).String() != "Service(127)" {
		t.Error("names")
	}
}
