package data_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

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
