# Send an SMS

This step reads a text message from a capture with a data.Receiver and sms.Decode, then sends a reply with sms.AppendPacket and a data.Sender, and gets the acknowledgement back.

## How a text goes on air

A text message is the user data of a data packet. The packet starts with a data header, which holds the addresses and the format, and continues with data blocks. A confirmed packet requests a response from the receiving radio: an ACK when every block arrived, or a SACK that lists the blocks to send again. Before the header, the sender sends preamble CSBKs, so that a radio in power save wakes up. (ETSI TS 102 361-1 clause 8.2, ETSI TS 102 361-3 clauses 5 and 6)

Radios put the text in the user data in more than one format. [Package sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms) reads each of them and builds each of them. (ETSI TS 102 361-3 clauses 5.7.1 and 6.1)

## The capture

`v2/data/testdata/anytone_sms_xpr.txt` holds the bursts of a confirmed SMS from radio 3191868 to radio 3140598 at rate 3/4. A BTECH DA-7X2 with AnyTone D890UV firmware sent it, and a Motorola XPR 8400 repeater repeated it on time slot 2. An RTL-SDR recorded the output of the repeater. Each line holds the time of a burst in milliseconds and the burst in hex.

## The code

The example has two parts. The first part, `readText`, replays the capture to a [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) and returns the decoded text of the packet that it delivers:

<!-- dmrgo:code file=v2/sms/example_test.go symbol=readText -->
```go
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
```
<!-- /dmrgo:code -->

The second part, `sendReply`, builds a reply in the same format with [sms.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#AppendPacket) and sends it with a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender). A second Receiver, at the address of the radio, takes the place of the radio. It sends its ACK back to the Sender:

<!-- dmrgo:code file=v2/sms/example_test.go symbol=sendReply -->
```go
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
```
<!-- /dmrgo:code -->

Run it from the `v2` directory:

```sh
go test -run Example_reply -v ./sms
```

It prints:

<!-- dmrgo:output test=sms.Example_reply -->
```text
AnyTone text from 3191868 to 3140598: "THE QUICK BROWN FOX JUMPED OVER THE LAZY DOG"
radio: "got it" from 3140598 <nil>
sender: Delivered after 1 tries (ACK)
```
<!-- /dmrgo:output -->

## What the code does

- The first Receiver has the address of the radio that the text is for, but no `Respond`. It reads the packet and sends nothing, as a monitor does. With `Respond: true` it returns the bursts of an ACK in an `EventRespond`.
- `sms.Decode` takes the data header and the user data of the packet. It finds the format of the text and returns the text as UTF-8 in `Body`.
- `sms.AppendPacket` writes the user data of a text into a slice that you give it, and sets the data header to send it with. `Confirmed: true` selects a confirmed packet.
- `tx.Send(0, m)` queues the message. The Sender does not send at once: each `tx.Tick(now, ...)` returns the bursts that are due at `now`, each with its `Due` time. In a real program, the bursts go to a transmitter or a network, and `tx.Deadline()` gives the time of the next Tick.
- The bursts of the response go back to the Sender through `tx.Burst`. The Sender then returns an `Output` with the result of the message.

The example connects the Sender and the Receiver directly, without a channel between them. [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) also does the channel access of a radio, the retries, and the selective retry of the blocks that a SACK names.

Next: [Receive on a radio](05-receive-on-a-radio.md).
