package sms_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

type uabPad struct{ uab, pad uint8 }

var udtTables = []struct {
	format enums.UDTFormat
	char   string
	rows   []uabPad
}{
	{enums.UDTISO7, "A", []uabPad{
		{0, 18}, {0, 16}, {0, 14}, {0, 13}, {0, 11}, {0, 9}, {0, 7}, {0, 6}, {0, 4}, {0, 2}, {0, 0},
		{1, 23}, {1, 21}, {1, 19}, {1, 17}, {1, 16}, {1, 14}, {1, 12}, {1, 10}, {1, 9}, {1, 7}, {1, 5}, {1, 3}, {1, 2}, {1, 0},
		{2, 22}, {2, 20}, {2, 19}, {2, 17}, {2, 15}, {2, 13}, {2, 12}, {2, 10}, {2, 8}, {2, 6}, {2, 5}, {2, 3}, {2, 1},
		{3, 23}, {3, 22}, {3, 20}, {3, 18}, {3, 16}, {3, 15}, {3, 13}, {3, 11}, {3, 9}, {3, 8}, {3, 6}, {3, 4}, {3, 2}, {3, 1},
	}},
	{enums.UDTISO8, "é", []uabPad{
		{0, 18}, {0, 16}, {0, 14}, {0, 12}, {0, 10}, {0, 8}, {0, 6}, {0, 4}, {0, 2}, {0, 0},
		{1, 22}, {1, 20}, {1, 18}, {1, 16}, {1, 14}, {1, 12}, {1, 10}, {1, 8}, {1, 6}, {1, 4}, {1, 2}, {1, 0},
		{2, 22}, {2, 20}, {2, 18}, {2, 16}, {2, 14}, {2, 12}, {2, 10}, {2, 8}, {2, 6}, {2, 4}, {2, 2}, {2, 0},
		{3, 22}, {3, 20}, {3, 18}, {3, 16}, {3, 14}, {3, 12}, {3, 10}, {3, 8}, {3, 6}, {3, 4}, {3, 2}, {3, 0},
	}},
	{enums.UDTUTF16BE, "Ω", []uabPad{
		{0, 16}, {0, 12}, {0, 8}, {0, 4}, {0, 0},
		{1, 20}, {1, 16}, {1, 12}, {1, 8}, {1, 4}, {1, 0},
		{2, 20}, {2, 16}, {2, 12}, {2, 8}, {2, 4}, {2, 0},
		{3, 20}, {3, 16}, {3, 12}, {3, 8}, {3, 4}, {3, 0},
	}},
}

func udtText(f enums.UDTFormat, body string) sms.Text {
	return sms.Text{
		Format: sms.FormatUDT, Source: 1001, Destination: 1002, Body: []byte(body),
		UDT: pdu.UDTHeader{SAP: enums.SAPShortData, UDTFormat: f, OptionFlag: true, ProtectFlag: true, Opcode: pdu.UDTOpcodeInboundHeader},
	}
}

// ETSI TS 102 361-4 Tables B.4, B.5 and B.8.
func TestAppendPacketUDTTables(t *testing.T) {
	t.Parallel()

	for _, tab := range udtTables {
		for i, row := range tab.rows {
			n := i + 1
			text := udtText(tab.format, strings.Repeat(tab.char, n))
			var h sms.Header
			p, err := sms.AppendPacket(nil, &text, &h)
			if err != nil {
				t.Fatalf("%v %d: %v", tab.format, n, err)
			}
			u, ok := h.Data.AsUDT()
			if !ok || u.UAB != row.uab || u.PadNibble != row.pad {
				t.Errorf("%v %d characters: UAB %d pad %d, want %d %d", tab.format, n, u.UAB, u.PadNibble, row.uab, row.pad)
			}
			var a trunk.Appended
			if err := a.SetText(tab.format, text.Body); err != nil {
				t.Fatal(err)
			}
			if a.Blocks != int(row.uab)+1 || a.PadNibble != row.pad || !bytes.Equal(p, a.Data[:len(p)]) ||
				len(p) != 12*a.Blocks-2-int(row.pad)/2 {
				t.Errorf("%v %d characters: % x, trunk.Appended % x", tab.format, n, p, a.Data[:12*a.Blocks-2])
			}
			_, got, err := sms.Decode(nil, &h.Data, p)
			if err != nil || string(got.Body) != string(text.Body) {
				t.Errorf("%v %d characters: Decode %q, %v", tab.format, n, got.Body, err)
			}
			if _, got, err := sms.Decode(nil, &h.Data, a.Data[:12*a.Blocks-2]); err != nil || string(got.Body) != string(text.Body) {
				t.Errorf("%v %d characters: Decode of whole blocks %q, %v", tab.format, n, got.Body, err)
			}
		}
		text := udtText(tab.format, strings.Repeat(tab.char, len(tab.rows)+1))
		if _, err := sms.AppendPacket(nil, &text, &sms.Header{}); !errors.Is(err, sms.ErrTextLength) {
			t.Errorf("%v %d characters: %v", tab.format, len(tab.rows)+1, err)
		}
	}
}

func deliver(t *testing.T, h *pdu.DataHeader, payload []byte) data.Message {
	t.Helper()
	tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1, Source: layer2.SyncSourceMS})
	if err != nil {
		t.Fatal(err)
	}
	rx, err := data.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Send(0, &data.Message{Header: *h, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var got []data.Message
	for now := time.Duration(0); now < 2*time.Second; now += 10 * time.Millisecond {
		for _, o := range tx.Tick(now, outputs[:0]) {
			if o.Kind == data.OutputDone && o.Result != data.ResultSent {
				t.Fatalf("result %v", o.Result)
			}
			for _, e := range rx.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				if e.Kind == data.EventDelivered {
					m := e.Message
					m.Payload = bytes.Clone(m.Payload)
					got = append(got, m)
				}
			}
		}
	}
	if len(got) != 1 {
		t.Fatalf("%d messages delivered", len(got))
	}
	return got[0]
}

func TestUDTRoundTripAndReply(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		format enums.UDTFormat
		body   string
	}{
		{enums.UDTISO7, "QSL 73, back on the hour"},
		{enums.UDTISO7, "Eleven ch.."},
		{enums.UDTISO8, "Grüße aus München"},
		{enums.UDTUTF16BE, "你好 \U0001F4E1 ok"},
	} {
		in := udtText(c.format, c.body)
		in.Group = c.format == enums.UDTISO8
		in.UDT.Emergency = c.format == enums.UDTISO7
		var h sms.Header
		p, err := sms.AppendPacket(nil, &in, &h)
		if err != nil {
			t.Fatalf("%v: %v", c.format, err)
		}
		m := deliver(t, &h.Data, p)
		_, got, err := sms.Decode(nil, &m.Header, m.Payload)
		if err != nil || got.Format != sms.FormatUDT || string(got.Body) != c.body || got.Source != in.Source ||
			got.Destination != in.Destination || got.Group != in.Group {
			t.Fatalf("%v: %+v, %v", c.format, got, err)
		}
		sent, _ := h.Data.AsUDT()
		if got.UDT != sent {
			t.Errorf("%v: UDT %+v, sent %+v", c.format, got.UDT, sent)
		}

		reply := got
		reply.Source, reply.Destination, reply.Body = got.Destination, got.Source, []byte("rx")
		var rh sms.Header
		if _, err := sms.AppendPacket(nil, &reply, &rh); err != nil {
			t.Fatal(err)
		}
		r, _ := rh.Data.AsUDT()
		want := sent
		want.LLIDSource, want.LLIDDestination = sent.LLIDDestination, sent.LLIDSource
		want.UAB, want.PadNibble = r.UAB, r.PadNibble
		if r != want || r.UAB != 0 {
			t.Errorf("%v: reply %+v, want %+v", c.format, r, want)
		}
	}
}

func TestUDTErrors(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		text sms.Text
		want error
	}{
		{"binary", udtText(enums.UDTBinary, "hi"), sms.ErrFormat},
		{"mixed", udtText(enums.UDTMixed, "hi"), sms.ErrFormat},
		{"LIP", udtText(enums.UDTLIP, "hi"), sms.ErrFormat},
		{"empty", udtText(enums.UDTISO8, ""), sms.ErrTextLength},
		{"7-bit", udtText(enums.UDTISO7, "café"), shortdata.ErrCharacter},
		{"8-bit", udtText(enums.UDTISO8, "Ω"), shortdata.ErrCharacter},
	} {
		dst := []byte("keep")
		dst, err := sms.AppendPacket(dst, &c.text, &sms.Header{})
		if !errors.Is(err, c.want) || string(dst) != "keep" {
			t.Errorf("%s: %v %q, want %v", c.name, err, dst, c.want)
		}
	}

	for _, c := range []struct {
		name string
		u    pdu.UDTHeader
		want error
	}{
		{"binary", pdu.UDTHeader{UDTFormat: enums.UDTBinary}, sms.ErrNotText},
		{"BCD", pdu.UDTHeader{UDTFormat: enums.UDTBCD}, sms.ErrNotText},
		{"pad over the blocks", pdu.UDTHeader{UDTFormat: enums.UDTISO8, PadNibble: 21}, sms.ErrLength},
	} {
		h := setHeader(t, (*pdu.DataHeader).SetUDT, c.u)
		if _, _, err := sms.Decode(nil, &h, []byte("hello")); !errors.Is(err, c.want) {
			t.Errorf("Decode %s: %v, want %v", c.name, err, c.want)
		}
	}
	h := setHeader(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{UDTFormat: enums.UDTISO8, UAB: 3})
	if _, got, err := sms.Decode(nil, &h, bytes.Repeat([]byte("a"), 60)); err != nil || len(got.Body) != 46 {
		t.Errorf("Decode of 60 octets: %d %v", len(got.Body), err)
	}
	if _, got, err := sms.Decode(nil, &h, nil); err != nil || len(got.Body) != 0 {
		t.Errorf("Decode of no octets: %q %v", got.Body, err)
	}
	if sms.FormatUDT.String() != "UDT" {
		t.Errorf("String %q", sms.FormatUDT.String())
	}
}
