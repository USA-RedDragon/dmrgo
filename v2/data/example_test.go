package data_test

import (
	"fmt"
	"os"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// ExampleReceiver follows an SMS capture of one logical channel, with the
// bursts 60 ms apart, and prints its events.
func ExampleReceiver() {
	capture, err := os.ReadFile("../layer2/testdata/d-sms.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := data.NewReceiver(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	events := make([]data.Event, 0, data.MaxEvents)
	var now time.Duration
	handle := func(events []data.Event) {
		for _, e := range events {
			switch e.Kind {
			case data.EventCallStart:
				fmt.Printf("data call from %d to %d\n", e.Call.Source, e.Call.Target)
			case data.EventDelivered:
				fmt.Printf("%v packet, %d octets\n", e.Message.Header.Format(), len(e.Message.Payload))
			case data.EventCallEnd:
				fmt.Printf("end: %v after %v\n", e.Reason, e.Stats.Duration)
			case data.EventResponse, data.EventRespond, data.EventReassembled:
			}
		}
	}
	for off := 0; off+33 <= len(capture); off += 33 {
		raw := layer2.RawBurst(capture[off : off+33])
		handle(rx.Burst(now, &raw, layer2.Hint{}, events[:0]))
		now += 60 * time.Millisecond
	}
	if at, ok := rx.Deadline(); ok {
		handle(rx.Tick(at, events[:0]))
	}
	// Output:
	// data call from 3191868 to 9990
	// Unconfirmed packet, 56 octets
	// end: Quiet after 960ms
}

// ExampleReceiver_respond answers a confirmed packet: the Receiver of the
// addressed radio returns the ACK bursts to send, with their Due times.
func ExampleReceiver_respond() {
	bursts := readTimedExample("testdata/anytone_sms_xpr.txt")
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: 3140598, Respond: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	events := make([]data.Event, 0, data.MaxEvents)
	for _, b := range bursts[:14] {
		for _, e := range rx.Burst(b.at, &b.raw, layer2.Hint{}, events[:0]) {
			switch e.Kind {
			case data.EventDelivered:
				fmt.Printf("delivered %d octets at %v\n", len(e.Message.Payload), e.Message.Rate)
			case data.EventRespond:
				fmt.Printf("send %v at %v\n", e.Response, e.Due)
			case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
			}
		}
	}
	// Output:
	// delivered 122 octets at Rate 3/4
	// send ACK at 4.14329s
}

// ExampleNewReceiver_configError shows the error of a config field out of
// range.
func ExampleNewReceiver_configError() {
	_, err := data.NewReceiver(&data.ReceiverConfig{MaxBlocks: 200})
	fmt.Println(err)
	// Output:
	// data: MaxBlocks is out of range
}
