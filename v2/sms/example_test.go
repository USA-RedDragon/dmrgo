package sms_test

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

var header sms.Header

// Example builds a MOTOTRBO text message as an unconfirmed packet, segments
// it into rate 1/2 blocks and reads the text back.
func Example() {
	t := sms.Text{Format: sms.FormatTMS, Source: 3191868, Destination: 3140598, Body: []byte("Hello")}
	data, err := sms.AppendPacket(nil, &t, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header.Data, data, enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	m := packet.Message{Header: p.Header, Data: data}
	_, got, err := sms.Decode(nil, &m.Header, m.Data)
	fmt.Printf("%d blocks, %s from %d: %s %v\n", len(p.Blocks), got.Format, got.Source, got.Body, err)
	// Output:
	// 5 blocks, TMS from 3191868: Hello <nil>
}

// ExampleAppendAck answers a TMS text that requests an acknowledgement.
func ExampleAppendAck() {
	in := sms.Text{Format: sms.FormatTMS, Source: 3191868, Destination: 3140598, Sequence: 3, AckRequired: true, Body: []byte("Hi")}
	data, err := sms.AppendPacket(nil, &in, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, got, err := sms.Decode(nil, &header.Data, data)
	if err != nil || !got.AckRequired {
		fmt.Println(got.AckRequired, err)
		return
	}
	var ackHeader sms.Header
	ack, err := sms.AppendAck(nil, &got, &ackHeader)
	if err != nil {
		fmt.Println(err)
		return
	}
	u, _ := ackHeader.Data.AsUnconfirmed()
	fmt.Printf("%d octets from %d to %d\n", len(ack), u.LLIDSource, u.LLIDDestination)
	// Output:
	// 33 octets from 3140598 to 3191868
}

// Example_reply reads a confirmed text from a capture of a radio, then
// sends a reply to that radio with a data.Sender. A second data.Receiver
// takes the place of the radio: it acknowledges the reply, and the Sender
// reports the result.
func Example_reply() {
	in, err := readText("../data/testdata/anytone_sms_xpr.txt")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%v text from %d to %d: %q\n", in.Format, in.Source, in.Destination, in.Body)
	if err := sendReply(in); err != nil {
		fmt.Println(err)
	}
	// Output:
	// AnyTone text from 3191868 to 3140598: "THE QUICK BROWN FOX JUMPED OVER THE LAZY DOG"
	// radio: "got it" from 3140598 <nil>
	// sender: Delivered after 1 tries (ACK)
}

func readText(path string) (sms.Text, error) {
	f, err := os.Open(path)
	if err != nil {
		return sms.Text{}, err
	}
	defer f.Close()
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: 3140598})
	if err != nil {
		return sms.Text{}, err
	}
	events := make([]data.Event, 0, data.MaxEvents)
	var in sms.Text
	sc := bufio.NewScanner(f)
	for sc.Scan() && in.Body == nil {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		ms, err := strconv.ParseFloat(fields[0], 64)
		b, err2 := hex.DecodeString(fields[1])
		if err != nil || err2 != nil || len(b) != 33 {
			return sms.Text{}, fmt.Errorf("bad capture line %q", sc.Text())
		}
		raw := layer2.RawBurst(b)
		now := time.Duration(ms * float64(time.Millisecond))
		for _, e := range rx.Burst(now, &raw, layer2.Hint{}, events[:0]) {
			if e.Kind != data.EventDelivered {
				continue
			}
			if _, in, err = sms.Decode(nil, &e.Message.Header, e.Message.Payload); err != nil {
				return sms.Text{}, err
			}
		}
	}
	return in, sc.Err()
}

func sendReply(in sms.Text) error {
	out := sms.Text{Format: in.Format, Source: in.Destination, Destination: in.Source, Confirmed: true, Body: []byte("got it")}
	var h sms.Header
	payload, err := sms.AppendPacket(nil, &out, &h)
	if err != nil {
		return err
	}
	tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot2, Source: layer2.SyncSourceMS, ColorCode: 7})
	if err != nil {
		return err
	}
	if err := tx.Send(0, &data.Message{Header: h.Data, Payload: payload}); err != nil {
		return err
	}
	radio, err := data.NewReceiver(&data.ReceiverConfig{Address: in.Source, Respond: true})
	if err != nil {
		return err
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var replies []data.Event
	for now := time.Duration(0); tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
		for _, e := range replies {
			for _, o := range tx.Burst(e.Due, &e.Burst, layer2.Hint{}, outputs[:0]) {
				fmt.Printf("sender: %v after %d tries (%v)\n", o.Result, o.Try, o.Response)
			}
		}
		replies = replies[:0]
		for _, o := range tx.Tick(now, outputs[:0]) {
			for _, e := range radio.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				switch e.Kind {
				case data.EventDelivered:
					_, got, err := sms.Decode(nil, &e.Message.Header, e.Message.Payload)
					fmt.Printf("radio: %q from %d %v\n", got.Body, got.Source, err)
				case data.EventRespond:
					replies = append(replies, e)
				case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
				}
			}
		}
	}
	return nil
}
