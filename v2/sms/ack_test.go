package sms_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func TestAppendAck(t *testing.T) {
	t.Parallel()

	m := ipMessage(t, appcapture.TMS)
	_, text, err := sms.Decode(nil, &m.Header, m.Data)
	if err != nil {
		t.Fatal(err)
	}
	var h sms.Header
	out, err := sms.AppendAck([]byte("x"), &text, &h)
	if err != nil {
		t.Fatal(err)
	}
	u := h.Data.UnconfirmedDataHeader
	if out[0] != 'x' || u == nil || u.LLIDSource != 3140598 || u.LLIDDestination != 3191868 || u.SAP != enums.SAPIPBasedPacketData {
		t.Fatalf("header %s", h.Data.String())
	}
	d, err := ip.Decode(enums.SAPIPBasedPacketData, out[1:])
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != [4]byte{12, 47, 235, 246} || d.Destination != [4]byte{12, 48, 180, 60} || d.SourcePort != ip.PortTMS || d.DestinationPort != ip.PortTMS {
		t.Errorf("datagram %+v", d)
	}
	ack, err := tms.NewAck(text.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ack.AppendBinary(nil)
	if err != nil || !bytes.Equal(d.Payload, want) {
		t.Errorf("payload %x, want %x", d.Payload, want)
	}
	for _, bad := range []sms.Text{{Format: sms.FormatIP}, {Format: sms.FormatTMS, Group: true}} {
		if b, err := sms.AppendAck([]byte("x"), &bad, &h); !errors.Is(err, sms.ErrFormat) || string(b) != "x" {
			t.Errorf("%+v: %q, %v", bad, b, err)
		}
	}
	if _, err := sms.AppendAck(nil, nil, &h); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil t: %v", err)
	}
	if _, err := sms.AppendAck(nil, &text, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil h: %v", err)
	}
}
