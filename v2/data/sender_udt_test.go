package data_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func crcCCITT(data []byte) uint16 {
	var reg uint16
	for _, b := range data {
		for i := 7; i >= 0; i-- {
			if (reg>>15)^uint16(b>>i)&1 == 1 {
				reg = reg<<1 ^ 0x1021
			} else {
				reg <<= 1
			}
		}
	}
	return ^reg
}

func udtBursts(t *testing.T, h pdu.UDTHeader, appended []byte) []layer2.RawBurst {
	t.Helper()
	n := int(h.UAB) + 1
	buf := bytes.Repeat([]byte{0xFF}, 12*n)
	copy(buf, appended)
	c := crcCCITT(buf[:12*n-2])
	buf[12*n-2], buf[12*n-1] = byte(c>>8), byte(c)
	hd := setHeader(t, (*pdu.DataHeader).SetUDT, h)
	d := make([]layer2.Data, 0, 1+n)
	d = append(d, &hd)
	for i := range n {
		d = append(d, &pdu.Rate12Data{Data: [12]byte(buf[12*i:])})
	}
	out := make([]layer2.RawBurst, 0, len(d))
	for _, x := range d {
		b, err := layer2.BuildBurst(x, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func TestSenderUDT(t *testing.T) {
	t.Parallel()
	text := []byte("dmrgo UDT text to a radio")
	h := pdu.UDTHeader{
		SAP: enums.SAPShortData, UDTFormat: enums.UDTISO8, LLIDDestination: dstID, LLIDSource: srcID,
		PadNibble: 2 * (34 - byte(len(text))), ProtectFlag: true, Opcode: pdu.UDTOpcodeInboundHeader,
	}
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1})
	if err := s.Send(0, &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetUDT, h), Payload: text}); err != nil {
		t.Fatal(err)
	}
	out := drain(t, s, 2*time.Second)
	h.UAB = 2
	want := udtBursts(t, h, text)
	if len(out) != len(want)+1 || out[len(out)-1].Kind != data.OutputDone || out[len(out)-1].Result != data.ResultSent {
		t.Fatalf("%d outputs, want %d bursts and the end", len(out), len(want))
	}
	bursts := make([]timedBurst, 0, len(want))
	for i, w := range want {
		if out[i].Burst != w {
			t.Errorf("burst %d: % x, want % x", i, out[i].Burst, w)
		}
		bursts = append(bursts, timedBurst{out[i].Due, out[i].Burst})
	}
	ev := replay(t, newReceiver(t, nil), bursts)
	if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" {
		t.Fatalf("receiver: %s", got)
	}
	m := ev[1].Message
	if got, ok := m.Header.AsUDT(); !ok || got != h || !bytes.Equal(m.Payload, text) {
		t.Fatalf("received %+v %q, want %+v %q", got, m.Payload, h, text)
	}
	again := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1})
	if err := again.Send(0, &m); err != nil {
		t.Fatal(err)
	}
	for i, o := range drain(t, again, 2*time.Second)[:len(want)] {
		if o.Burst != want[i] {
			t.Errorf("resent burst %d differs", i)
		}
	}
}

func TestSenderUDTOddPadNibble(t *testing.T) {
	t.Parallel()
	h := pdu.UDTHeader{UDTFormat: enums.UDTISO7, LLIDDestination: dstID, LLIDSource: srcID, PadNibble: 13}
	appended := []byte{0x83, 0x0F, 0x1C, 0x7F}
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: 2})
	if err := s.Send(0, &data.Message{Header: setHeader(t, (*pdu.DataHeader).SetUDT, h), Payload: appended}); err != nil {
		t.Fatal(err)
	}
	out := drain(t, s, 2*time.Second)
	if len(out) != 2+2+1 || out[len(out)-1].Result != data.ResultSent {
		t.Fatalf("%d outputs", len(out))
	}
	ev := replay(t, newReceiver(t, nil), func() []timedBurst {
		b := make([]timedBurst, 0, len(out))
		for _, o := range out[:len(out)-1] {
			b = append(b, timedBurst{o.Due, o.Burst})
		}
		return b
	}())
	if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" || !bytes.Equal(ev[1].Message.Payload, appended) {
		t.Fatalf("receiver: %s %+v", got, ev)
	}
}

func TestSenderUDTErrors(t *testing.T) {
	t.Parallel()
	h := setHeader(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{UDTFormat: enums.UDTISO8, LLIDDestination: dstID, LLIDSource: srcID})
	for _, c := range []struct {
		name string
		m    *data.Message
		want error
	}{
		{"rate 3/4", &data.Message{Header: h, Rate: enums.DataTypeRate34, Payload: payload(4)}, packet.ErrRate},
		{"47 octets", &data.Message{Header: h, Payload: payload(47)}, packet.ErrTooLong},
		{"P_HEAD", &data.Message{Header: h, Payload: payload(4), Proprietary: &pdu.ProprietaryHeader{}}, nil},
	} {
		s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1})
		err := s.Send(0, c.m)
		if c.want == nil && err == nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1, MaxBlocks: 2})
	if err := s.Send(0, &data.Message{Header: h, Payload: payload(30)}); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("MaxBlocks 2: %v", err)
	}
}
