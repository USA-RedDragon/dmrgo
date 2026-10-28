package data_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var testProp = pdu.ProprietaryHeader{SAP: enums.SAPUDPIPHeaderCompression, MFID: enums.FIDMotorola, ManufacturerData: [8]byte{0x11, 1, 0, 0, 1, 2, 3, 4}}

func TestSenderProprietaryUnconfirmed(t *testing.T) {
	t.Parallel()
	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPProprietaryPacketData, LLIDDestination: dstID, LLIDSource: srcID})
	prop := testProp
	m := &data.Message{Header: u, Proprietary: &prop, Payload: payload(40)}
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: 2, Source: layer2.SyncSourceMS, ColorCode: 1})
	if err := s.Send(0, m); err != nil {
		t.Fatal(err)
	}
	prop.MFID = 0x68
	out := drain(t, s, time.Second)
	var bursts []timedBurst
	var types []string
	for _, o := range out {
		if o.Kind == data.OutputBurst {
			bursts = append(bursts, timedBurst{o.Due, o.Burst})
			types = append(types, o.Hint.DataType.String())
		}
	}
	if len(out) == 0 || out[len(out)-1].Kind != data.OutputDone || out[len(out)-1].Result != data.ResultSent {
		t.Fatalf("outputs %+v", out)
	}
	if len(bursts) != 2+2+4 {
		t.Fatalf("%d bursts: %v", len(bursts), types)
	}
	var b layer2.Burst
	if err := b.DecodeFromBytes(bursts[3].raw); err != nil {
		t.Fatal(err)
	}
	dh, ok := b.Data.(*pdu.DataHeader)
	if !ok {
		t.Fatalf("burst 3 is %T", b.Data)
	}
	if ph, ok := dh.AsProprietary(); !ok || ph != testProp {
		t.Errorf("burst 3: %v, want the P_HEAD %+v", dh, testProp)
	}
	if err := b.DecodeFromBytes(bursts[2].raw); err != nil {
		t.Fatal(err)
	}
	first, _ := b.Data.(*pdu.DataHeader)
	if uh, ok := first.AsUnconfirmed(); !ok || uh.BlocksToFollow != 5 || uh.PadOctetCount != 4 || !uh.FullMessageFlag {
		t.Errorf("header %+v", uh)
	}
	ev := replay(t, newReceiver(t, nil), bursts)
	for _, e := range ev {
		if e.Kind != data.EventDelivered {
			continue
		}
		if e.Message.Proprietary == nil || *e.Message.Proprietary != testProp || !bytes.Equal(e.Message.Payload, payload(40)) || e.Message.BadCRC {
			t.Errorf("delivered %+v", e.Message)
		}
		return
	}
	t.Errorf("no delivery: %s", kinds(ev))
}

func TestSenderProprietaryConfirmed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		drop  func(try uint8, i int) bool
		tries uint8
		resp  string
		prop  bool
		n     int
	}{
		{"clean", nil, 1, "ACK", true, 1},
		{"lost block", func(try uint8, i int) bool { return try == 1 && i == 4 }, 2, "SACK+ACK", true, 1},
		{"lost header", func(try uint8, i int) bool { return try == 1 && i == 0 }, 2, "ACK", true, 1},
		{"lost P_HEAD", func(try uint8, i int) bool { return try == 1 && i == 1 }, 2, "SACK+ACK", true, 1},
		{"lost P_HEAD and a block", func(try uint8, i int) bool { return try == 1 && (i == 1 || i == 5) }, 3, "SACK+SACK+ACK", true, 1},
		{"lost ACK", func(try uint8, i int) bool { return try == 0 && i == 0 }, 2, "ACK+ACK", true, 2},
	} {
		l := &link{
			t:    t,
			tx:   newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1, Retries: 3}),
			rx:   newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
			drop: c.drop,
		}
		prop := testProp
		m := message(t, 60)
		m.Header = sapHeader(t, enums.SAPProprietaryPacketData, 0, false)
		m.Proprietary = &prop
		l.run(m, 10*time.Second)
		if len(l.done) != 1 || l.done[0].Result != data.ResultDelivered || l.done[0].Try != c.tries {
			t.Errorf("%s: done %+v", c.name, l.done)
			continue
		}
		if r := strings.Join(l.resp, "+"); r != c.resp {
			t.Errorf("%s: responses %q, want %q", c.name, r, c.resp)
		}
		if len(l.msgs) != c.n {
			t.Errorf("%s: %d deliveries, want %d, as the IP bearer service delivers duplicates", c.name, len(l.msgs), c.n)
		}
		for _, d := range l.msgs {
			if !bytes.Equal(d.Payload, payload(60)) || (d.Proprietary != nil) != c.prop || c.prop && *d.Proprietary != testProp {
				t.Errorf("%s: delivered %+v", c.name, d)
			}
		}
	}
}

func TestSenderProprietaryLimits(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1})
	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPProprietaryPacketData, LLIDDestination: dstID, LLIDSource: srcID})
	prop := testProp
	if err := s.Send(0, &data.Message{Header: u, Proprietary: &prop, Payload: payload(127*12 - 4)}); !errors.Is(err, packet.ErrTooLong) {
		t.Errorf("127 blocks and a P_HEAD: %v", err)
	}
	bad := pdu.ProprietaryHeader{SAP: 16}
	if err := s.Send(0, &data.Message{Header: u, Proprietary: &bad, Payload: payload(10)}); err == nil {
		t.Error("a P_HEAD with SAP 16 was sent")
	}
	if s.State() != data.SenderIdle {
		t.Errorf("state %v", s.State())
	}
	if err := s.Send(0, &data.Message{Header: u, Proprietary: &prop, Payload: payload(126*12 - 4)}); err != nil {
		t.Errorf("126 blocks and a P_HEAD: %v", err)
	}
}
