package data_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestMessageSetRawDataSends(t *testing.T) {
	t.Parallel()

	payload := []byte("an R_HEAD message of several blocks at rate 1/2")
	for _, confirmed := range []bool{false, true} {
		var m data.Message
		m.Payload = payload
		if err := m.SetRawData(pdu.RawDataHeader{
			ResponseRequested: confirmed, SAP: enums.SAPShortData, LLIDDestination: 3120002, LLIDSource: 3120001,
			SourcePort: 3, DestinationPort: 5,
		}); err != nil {
			t.Fatal(err)
		}
		s := newSender(t, &data.SenderConfig{ColorCode: 1, Preambles: -1})
		if err := s.Send(0, &m); err != nil {
			t.Fatal(err)
		}
		r := newReceiver(t, &data.ReceiverConfig{})
		var ev []data.Event
		for _, o := range drain(t, s, 400*time.Millisecond) {
			if o.Kind == data.OutputBurst {
				ev = r.Burst(o.Due, &o.Burst, o.Hint, ev)
			}
		}
		var got *data.Message
		for i := range ev {
			if ev[i].Kind == data.EventDelivered {
				got = &ev[i].Message
			}
		}
		if got == nil {
			t.Fatalf("confirmed %v: no message in %s", confirmed, kinds(ev))
		}
		rh, ok := got.Header.AsRawData()
		blocks := (len(payload) + 4 + 11) / 12
		if confirmed {
			blocks = (len(payload) + 4 + 9) / 10
		}
		if !ok || int(rh.AppendedBlocks) != blocks || rh.SourcePort != 3 || rh.DestinationPort != 5 || rh.ResponseRequested != confirmed || string(got.Payload) != string(payload) {
			t.Fatalf("confirmed %v: delivered %+v %q, want %d blocks", confirmed, rh, got.Payload, blocks)
		}
	}
}

func TestMessageSetRawDataKeepsAppendedBlocks(t *testing.T) {
	t.Parallel()

	var m data.Message
	if err := m.SetRawData(pdu.RawDataHeader{AppendedBlocks: 7, LLIDDestination: 1, LLIDSource: 2}); err != nil {
		t.Fatal(err)
	}
	if rh, ok := m.Header.AsRawData(); !ok || rh.AppendedBlocks != 7 {
		t.Fatalf("%+v %v", rh, ok)
	}
	before := m
	var fre *dmrgo.FieldRangeError
	if err := m.SetRawData(pdu.RawDataHeader{SourcePort: 8}); !errors.As(err, &fre) || m.Header != before.Header {
		t.Fatalf("SourcePort 8: %v, header %+v", err, m.Header)
	}
	var nilMessage *data.Message
	if err := nilMessage.SetRawData(pdu.RawDataHeader{}); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Fatalf("nil Message: %v", err)
	}
}
