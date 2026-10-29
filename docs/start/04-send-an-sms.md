# Send an SMS

This step reads a text message from a capture with a data.Receiver and sms.Decode, then sends a reply with sms.AppendPacket and a data.Sender, and gets the acknowledgement back.

## How a text goes on air

A text message is the user data of a data packet. The packet starts with a data header, which holds the addresses and the format, and continues with data blocks. A confirmed packet requests a response from the receiving radio: an ACK when every block arrived, or a SACK that lists the blocks to send again. Before the header, the sender sends preamble CSBKs, so that a radio in power save wakes up. (ETSI TS 102 361-1 clause 8.2, ETSI TS 102 361-3 clauses 5 and 6)

Radios put the text in the user data in more than one format. [Package sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms) reads each of them and builds each of them. (ETSI TS 102 361-3 clauses 5.7.1 and 6.1)

## The capture

`v2/data/testdata/anytone_sms_xpr.txt` holds the bursts of a confirmed SMS from radio 3191868 to radio 3140598 at rate 3/4. A BTECH DA-7X2 with AnyTone D890UV firmware sent it, and a Motorola XPR 8400 repeater repeated it on time slot 2. An RTL-SDR recorded the output of the repeater. Each line holds the time of a burst in milliseconds and the burst in hex.

## The code

The example has two parts. The first part replays the capture to a [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) and decodes the text of the packet that it delivers:

<!-- dmrgo:code file=v2/sms/example_test.go lines=72-109 -->
<!-- /dmrgo:code -->

The second part builds a reply in the same format with [sms.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#AppendPacket) and sends it with a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender). A second Receiver, at the address of the radio, takes the place of the radio. It sends its ACK back to the Sender:

<!-- dmrgo:code file=v2/sms/example_test.go lines=111-153 -->
<!-- /dmrgo:code -->

Run it from the `v2` directory:

```sh
go test -run Example_reply -v ./sms
```

It prints:

<!-- dmrgo:output test=sms.Example_reply -->
<!-- /dmrgo:output -->

## What the code does

- The first Receiver has the address of the radio that the text is for, but no `Respond`. It reads the packet and sends nothing, as a monitor does. With `Respond: true` it returns the bursts of an ACK in an `EventRespond`.
- `sms.Decode` takes the data header and the user data of the packet. It finds the format of the text and returns the text as UTF-8 in `Body`.
- `sms.AppendPacket` writes the user data of a text into a slice that you give it, and sets the data header to send it with. `Confirmed: true` selects a confirmed packet.
- `tx.Send(0, m)` queues the message. The Sender does not send at once: each `tx.Tick(now, ...)` returns the bursts that are due at `now`, each with its `Due` time. In a real program, the bursts go to a transmitter or a network, and `tx.Deadline()` gives the time of the next Tick.
- The bursts of the response go back to the Sender through `tx.Burst`. The Sender then returns an `Output` with the result of the message.

The example connects the Sender and the Receiver directly, without a channel between them. [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) also does the channel access of a radio, the retries, and the selective retry of the blocks that a SACK names.

Next: [Receive on a radio](05-receive-on-a-radio.md).
