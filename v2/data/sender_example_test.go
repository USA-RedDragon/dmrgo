package data_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ExampleSender sends a confirmed message to a Receiver. The air between
// them loses the third burst of the first try, so the Receiver asks for
// that block again with a SACK, and the Sender resends only that block.
func ExampleSender() {
	tx, err := data.NewSender(&data.SenderConfig{Source: layer2.SyncSourceMS, ColorCode: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: 3120002, Respond: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	m := &data.Message{
		Header: pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001,
		}},
		Rate:    enums.DataTypeRate12,
		Payload: []byte("a confirmed message of a few blocks"),
	}
	if err := tx.Send(0, m); err != nil {
		fmt.Println(err)
		return
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var replies []data.Event
	sent := 0
	for now := time.Duration(0); tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
		for _, e := range replies {
			for _, o := range tx.Burst(e.Due, &e.Burst, layer2.Hint{}, outputs[:0]) {
				fmt.Printf("%v after %d tries (%v)\n", o.Result, o.Try, o.Response)
			}
		}
		replies = replies[:0]
		for _, o := range tx.Tick(now, outputs[:0]) {
			sent++
			if sent == 3 {
				continue
			}
			for _, e := range rx.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				switch e.Kind {
				case data.EventDelivered:
					fmt.Printf("delivered %q\n", e.Message.Payload)
				case data.EventRespond:
					if e.Burst.SlotType().DataType == enums.DataTypeDataHeader {
						fmt.Printf("receiver sends %v\n", e.Response)
					}
					replies = append(replies, e)
				case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
				}
			}
		}
	}
	// Output:
	// receiver sends SACK
	// delivered "a confirmed message of a few blocks"
	// receiver sends ACK
	// Delivered after 2 tries (ACK)
}
