package hdap_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
)

const gps = "A200037021225N3412.7300E10850.00000.0121"

func TestRRSRegistration(t *testing.T) {
	t.Parallel()

	r := hdap.RRSRadio{Radio: [4]byte{10, 0, 0, 80}}
	payload, err := r.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := hdap.Message{Header: hdap.Header{Service: hdap.ServiceRRS}, Opcode: hdap.RRSRegistration, Payload: payload, End: 3}
	m.Seal()
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(out); got != "1100030004"+"0a000050"+"d103" {
		t.Fatalf("message %s", got)
	}
	back, err := hdap.DecodeMessage(out)
	if err != nil || !back.ChecksumOK() || back.Header.Service != hdap.ServiceRRS || back.Opcode != hdap.RRSRegistration {
		t.Fatalf("decoded %s, %v", back.String(), err)
	}
	rr, err := hdap.DecodeRRSRadio(back.Payload)
	if err != nil || rr.Radio != r.Radio {
		t.Errorf("radio %v, %v", rr.Radio, err)
	}
	out[6] ^= 1
	if bad, err := hdap.DecodeMessage(out); err != nil || bad.ChecksumOK() {
		t.Errorf("a changed payload passes the checksum: %v", err)
	}
}

func TestLocationAnswer(t *testing.T) {
	t.Parallel()

	data := append([]byte{0, 0, 0, 7, 10, 0, 0, 80, 0, 0}, gps...)
	a, err := hdap.DecodeLPAnswer(data)
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
	b, err := hdap.DecodeLPAnswer(south)
	if err != nil || b.GPS.Latitude() > 0 || b.GPS.Longitude() > 0 {
		t.Errorf("south west %v %v, %v", b.GPS.Latitude(), b.GPS.Longitude(), err)
	}
	broken := append([]byte{}, data...)
	broken[10+18] = ','
	if _, err := hdap.DecodeLPAnswer(broken); !errors.Is(err, hdap.ErrSignature) {
		t.Errorf("no point: %v", err)
	}
	broken[10+18], broken[10+2] = '.', 'x'
	if _, err := hdap.DecodeLPAnswer(broken); !errors.Is(err, hdap.ErrText) {
		t.Errorf("letter in the hour: %v", err)
	}
}

func TestTextMessage(t *testing.T) {
	t.Parallel()

	var m hdap.TMPMessage
	m.RequestID, m.Destination, m.Source = 1, [4]byte{10, 0, 0, 2}, [4]byte{10, 0, 0, 1}
	m.SetTextUTF8(nil, []byte("Hello"))
	payload, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	flags, err := (&hdap.TMPFlags{}).AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := hdap.Message{Header: hdap.Header{Reliable: true, Service: hdap.ServiceTMP}, Opcode: hdap.Opcode(flags[0])<<8 | hdap.Opcode(hdap.TMPPrivateMessage), Payload: payload, End: 3}
	msg.Seal()
	out, err := msg.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0x89 || out[1] != 0x00 || out[2] != 0xA1 {
		t.Errorf("header %x", out[:3])
	}
	back, err := hdap.DecodeMessage(out)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := hdap.DecodeTMPMessage(back.Payload)
	if err != nil || string(tm.TextUTF8(nil)) != "Hello" || tm.Source != m.Source {
		t.Errorf("text %q, %v", tm.TextUTF8(nil), err)
	}
	ack := hdap.TMPAnswer{RequestID: 1, Destination: m.Destination, Source: m.Source, Result: 4}
	b, _ := ack.AppendBinary(nil)
	if back, err := hdap.DecodeTMPAnswer(b); err != nil || back.Result != 4 {
		t.Errorf("answer %v", err)
	}
	g := hdap.TMPGroupAnswer{RequestID: 2, Destination: [4]byte{225, 0, 0, 9}}
	b, _ = g.AppendBinary(nil)
	if back, err := hdap.DecodeTMPGroupAnswer(b); err != nil || back.RequestID != 2 {
		t.Errorf("group answer %v", err)
	}
}

func TestMessageErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		data []byte
		want error
	}{
		{nil, hdap.ErrShort},
		{[]byte{0x11, 0, 3, 0, 4, 1, 2}, hdap.ErrShort},
		{[]byte{0x11, 0, 3, 0, 0, 0xd1, 4}, hdap.ErrSignature},
		{[]byte{0x11, 0, 3, 0, 0, 0xd1, 3, 0}, hdap.ErrLength},
	} {
		if _, err := hdap.DecodeMessage(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%x: %v, want %v", tt.data, err, tt.want)
		}
	}
	if hdap.ServiceLP.String() != "LP" || hdap.Service(0x7F).String() != "Service(127)" {
		t.Error("names")
	}
	if hdap.LPConditionReport.String() != "LPConditionReport" || hdap.Opcode(0x89A1).String() != "Opcode(35233)" {
		t.Error("opcode names")
	}
	if hdap.TMPGroupMessage.String() != "TMPGroupMessage" || hdap.TMPType(0).String() != "TMPType(0)" {
		t.Error("TMP type names")
	}
}
