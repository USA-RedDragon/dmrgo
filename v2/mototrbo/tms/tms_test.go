package tms_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
)

func captured(t testing.TB) []byte {
	t.Helper()
	data, err := hex.DecodeString(appcapture.TMS)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ip.Decode(enums.SAPIPBasedPacketData, data)
	if err != nil || d.DestinationPort != ip.PortTMS {
		t.Fatalf("datagram %+v, %v", d, err)
	}
	return d.Payload
}

func TestCapturedText(t *testing.T) {
	t.Parallel()

	payload := captured(t)
	m, err := tms.DecodeMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := m.Sequence()
	if m.Length != 28 || !m.Header.Extension || m.Header.AckRequired || !m.Header.Reserved || m.Header.Type != tms.TypeText ||
		len(m.Address) != 0 || !ok || seq != 1 || m.SequenceHigh.Encoding != tms.EncodingUTF16LE {
		t.Fatalf("got %s", m.String())
	}
	if got := string(m.TextUTF8(nil)); got != "\r\nMOTO TEST" {
		t.Errorf("text %q", got)
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, payload) {
		t.Errorf("AppendBinary = %x, %v, want %x", out, err, payload)
	}
	n, err := tms.NewText(1, false)
	if err != nil {
		t.Fatal(err)
	}
	n.SetTextUTF8(nil, []byte("\r\nMOTO TEST\x00"))
	out, err = n.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, payload) {
		t.Errorf("NewText = %x, %v, want %x", out, err, payload)
	}
}

func TestAck(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		seq  uint8
		want string
	}{{1, "00039f0001"}, {100, "00049f008460"}} {
		m, err := tms.NewAck(tt.seq)
		if err != nil {
			t.Fatal(err)
		}
		out, err := m.AppendBinary(nil)
		if err != nil || hex.EncodeToString(out) != tt.want {
			t.Errorf("NewAck(%d) = %x, %v, want %s", tt.seq, out, err, tt.want)
		}
		back, err := tms.DecodeMessage(out)
		if err != nil {
			t.Fatal(err)
		}
		if seq, ok := back.Sequence(); !ok || seq != tt.seq || back.Header.Type != tms.TypeAck || len(back.Text) != 0 {
			t.Errorf("decoded %s", back.String())
		}
	}
	if _, err := tms.NewAck(128); err == nil {
		t.Error("NewAck(128) did not fail")
	}
	if _, err := tms.NewText(200, true); err == nil {
		t.Error("NewText(200) did not fail")
	}
}

func TestAvailability(t *testing.T) {
	t.Parallel()

	m, err := tms.DecodeMessage([]byte{0, 3, 0x90, 0, 0x03})
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Type != tms.TypeAvailability || m.Capability != 3 {
		t.Fatalf("got %s", m.String())
	}
	if _, ok := m.Sequence(); ok {
		t.Error("an availability message has a sequence number")
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	payload := captured(t)
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", nil, tms.ErrShort},
		{"length beyond data", payload[:20], tms.ErrShort},
		{"trailing", append(append([]byte{}, payload...), 0), tms.ErrLength},
		{"address beyond", []byte{0, 3, 0x20, 5, 0}, tms.ErrShort},
	}
	for _, tt := range tests {
		if _, err := tms.DecodeMessage(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if tms.TypeAck.String() != "Ack" || tms.Type(3).String() != "Type(3)" || tms.EncodingUTF16LE.String() != "UTF-16LE" || tms.Encoding(9).String() != "Encoding(9)" {
		t.Error("names")
	}
}
