package data_test

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSenderDefinedDataWithoutAppendedBlocks(t *testing.T) {
	t.Parallel()
	l := &link{
		t:  t,
		tx: newSender(t, &data.SenderConfig{Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1}),
		rx: newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
	}
	m := message(t, 40)
	m.Header = setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
		SAP: enums.SAPShortData, LLIDDestination: dstID, LLIDSource: srcID, DefinedData: enums.DDBinary, ResponseRequested: true,
	})
	l.run(m, 10*time.Second)
	if len(l.done) != 1 || l.done[0].Result != data.ResultDelivered || len(l.got) != 1 || !bytes.Equal(l.got[0], payload(40)) {
		t.Fatalf("done %+v, delivered %x", l.done, l.got)
	}
}

func TestReceiverRemovesPadsOfWholeResend(t *testing.T) {
	t.Parallel()
	dd := setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
		SAP: enums.SAPShortData, LLIDDestination: dstID, LLIDSource: srcID, AppendedBlocks: 1, DefinedData: enums.DDBinary, SARQ: true,
	})
	for _, h := range []pdu.DataHeader{sapHeader(t, enums.SAPUDPIPHeaderCompression, 0, false), sapHeader(t, enums.SAPShortData, 0, false), dd} {
		l := &link{
			t:    t,
			tx:   newSender(t, &data.SenderConfig{Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1, Retries: 3}),
			rx:   newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
			drop: func(try uint8, i int) bool { return try == 1 && i == 0 },
		}
		m := message(t, 61)
		m.Header = h
		l.run(m, 10*time.Second)
		if len(l.done) != 1 || l.done[0].Result != data.ResultDelivered {
			t.Fatalf("%v: done %+v", h.Format(), l.done)
		}
		if len(l.got) != 1 || !bytes.Equal(l.got[0], payload(61)) {
			t.Errorf("%v: delivered %x, want %x", h.Format(), l.got, payload(61))
		}
	}
}

func TestReceiverSelectiveRetryOfResend(t *testing.T) {
	t.Parallel()
	for _, rate := range []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1} {
		sent := map[uint8]int{}
		l := &link{
			t:  t,
			tx: newSender(t, &data.SenderConfig{Preambles: -1, Source: layer2.SyncSourceMS, ColorCode: 1}),
			rx: newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
			drop: func(try uint8, i int) bool {
				if try > 0 {
					sent[try]++
				}
				return try == 1 && (i == 3 || i == 6) || try == 2 && i == 1
			},
		}
		m := message(t, 200)
		m.Header, m.Rate = sapHeader(t, enums.SAPUDPIPHeaderCompression, 0, false), rate
		l.run(m, 20*time.Second)
		var done []data.Output
		for _, o := range l.done {
			if o.Kind == data.OutputDone {
				done = append(done, o)
			}
		}
		if len(done) != 1 || done[0].Result != data.ResultDelivered || done[0].Try != 3 {
			t.Fatalf("%v: done %+v", rate, done)
		}
		if want := []string{"SACK", "SACK", "ACK"}; !slices.Equal(l.resp, want) {
			t.Errorf("%v: responses %q, want %q", rate, l.resp, want)
		}
		if sent[2] != 3 || sent[3] != 2 {
			t.Errorf("%v: try 2 sent %d bursts, try 3 sent %d, want 3 and 2", rate, sent[2], sent[3])
		}
		if len(l.got) != 1 || !bytes.Equal(l.got[0], payload(200)) {
			t.Errorf("%v: delivered %x", rate, l.got)
		}
	}
}
