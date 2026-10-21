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

// ExampleSender_window sends three IP messages in one sliding window. Only
// the last header asks for a response, and one ACK delivers all three.
func ExampleSender_window() {
	tx, err := data.NewSender(&data.SenderConfig{Source: layer2.SyncSourceMS, ColorCode: 1, Window: 3})
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: 3120002, Respond: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, text := range []string{"one", "two", "three"} {
		m := &data.Message{
			Header: pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
				SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001,
			}},
			Rate:    enums.DataTypeRate12,
			Payload: []byte(text),
		}
		if err := tx.Send(0, m); err != nil {
			fmt.Println(err)
			return
		}
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var replies []data.Event
	for now := time.Duration(0); tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
		for _, e := range replies {
			for _, o := range tx.Burst(e.Due, &e.Burst, layer2.Hint{}, outputs[:0]) {
				fmt.Printf("N(S) %d: %v (%v)\n", o.Seq, o.Result, o.Response)
			}
		}
		replies = replies[:0]
		for _, o := range tx.Tick(now, outputs[:0]) {
			for _, e := range rx.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				switch e.Kind {
				case data.EventDelivered:
					fmt.Printf("delivered %q, response requested %v\n", e.Message.Payload,
						e.Message.Header.ConfirmedDataHeader.ResponseRequested)
				case data.EventRespond:
					replies = append(replies, e)
				case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
				}
			}
		}
	}
	// Output:
	// delivered "one", response requested false
	// delivered "two", response requested false
	// delivered "three", response requested true
	// N(S) 0: Delivered (ACK)
	// N(S) 1: Delivered (ACK)
	// N(S) 2: Delivered (ACK)
}

// ExampleOutput_ICMP shows the ICMP error message that the IP layer gets
// for a confirmed message that got no response in its tries.
func ExampleOutput_ICMP() {
	tx, err := data.NewSender(&data.SenderConfig{Retries: 2})
	if err != nil {
		fmt.Println(err)
		return
	}
	m := &data.Message{
		Header: pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001,
		}},
		Rate:    enums.DataTypeRate12,
		Payload: []byte("no one hears this"),
	}
	if err := tx.Send(0, m); err != nil {
		fmt.Println(err)
		return
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	for {
		at, ok := tx.Deadline()
		if !ok {
			return
		}
		for _, o := range tx.Tick(at, outputs[:0]) {
			if icmp, err := o.ICMP(); err == nil {
				fmt.Printf("%v after %d tries: ICMP %v (type %d, code %d)\n", o.Result, o.Try, icmp, icmp.Type(), icmp.Code())
			}
		}
	}
	// Output:
	// RetryLimit after 2 tries: ICMP Host Unreachable (type 3, code 1)
}
