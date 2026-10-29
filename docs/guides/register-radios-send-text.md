# Register radios and send text: ARS and TMS

This guide shows how a server reads the registration of a MOTOTRBO radio, answers it, and reads and sends text messages of the MOTOTRBO Text Messaging Service.

## Goal

Act as the ARS and TMS server of a MOTOTRBO system: take each registration that a radio sends after power on, answer it, read the text messages that radios send, and send text with an acknowledgement.

## What you need

- Go and a checkout of dmrgo. The examples run offline.
- The packages [mototrbo/ars](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars), [mototrbo/tms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms) and [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip).
- To carry the packets on a channel, a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) and a [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver). The guide [Send SMS and confirmed data with retries](send-data.md) shows them.

ARS and TMS are UDP protocols. A radio sends each message in an IPv4 packet, and the IPv4 packet goes in a DMR data packet with the SAP IP based packet data. Motorola does not publish the layout of ARS or TMS. The packages follow open implementations and radio captures.

Run the examples from the top of the repository:

```sh
go test -C v2 -run Example -v ./mototrbo/ars ./mototrbo/tms
```

## Steps

### 1. Read a registration and answer it

A radio sends a device registration to UDP port [ip.PortARS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Port) when it powers on or changes channel, and again before its registration ends. [ip.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Decode) takes the IPv4 and UDP headers off the user data of the packet. [ars.DecodeMessage](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#DecodeMessage) decodes the payload, and [Message.DeviceID](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#Message.DeviceID) returns the radio ID.

[ars.NewAck](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#NewAck) builds the answer with the refresh time, in units of 30 minutes. Send it back in an IPv4 packet with the addresses and ports of the registration swapped. [ars.NewFailure](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#NewFailure) builds a refusal, and [ars.NewQuery](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#NewQuery) requests that a radio register again.

The example first builds the registration as a radio does, with [ars.NewRegistration](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars#NewRegistration) and [ip.AppendIPv4](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#AppendIPv4). [ip.RadioAddress](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#RadioAddress) gives the IPv4 address of a radio ID.

<!-- dmrgo:code file=v2/mototrbo/ars/example_test.go lines=31-57 -->
<!-- /dmrgo:code -->

### 2. Read a text message

A text goes to UDP port [ip.PortTMS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Port). [tms.DecodeMessage](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms#DecodeMessage) decodes it, and [Message.TextUTF8](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms#Message.TextUTF8) returns the text as UTF-8. The example reads a text that an AnyTone radio sent in its M-SMS mode.

When `Header.AckRequired` is set, answer with [tms.NewAck](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms#NewAck) and the sequence number of the text. The radio in the capture does not request an acknowledgement.

<!-- dmrgo:code file=v2/mototrbo/tms/example_test.go lines=36-56 -->
<!-- /dmrgo:code -->

### 3. Send a text message

[tms.NewText](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms#NewText) builds a text with a sequence number, and [Message.SetTextUTF8](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms#Message.SetTextUTF8) sets its text. Use a new sequence number for each text, so that the acknowledgement matches it. Put the encoded message in an IPv4 packet to port `ip.PortTMS` of the radio, as in step 1.

<!-- dmrgo:code file=v2/mototrbo/tms/example_test.go lines=16-25 -->
<!-- /dmrgo:code -->

### 4. Carry the packets on a channel

Give the IPv4 packet to a `data.Sender` as the payload of a confirmed data message with the SAP IP based packet data and the radio ID of the radio as the destination LLID. On the receive side, a `data.Receiver` returns the payload of each data packet. Pass it to `ip.Decode`, and pick ARS or TMS by the destination port. The guide [Send SMS and confirmed data with retries](send-data.md) shows both machines.

To read text in every format that radios use, not only TMS, use [sms.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#Decode). It calls `tms.DecodeMessage` for a packet to the TMS port.

## Output

The registration and its answer:

<!-- dmrgo:output test=ars.ExampleNewAck -->
<!-- /dmrgo:output -->

The text message from the capture:

<!-- dmrgo:output test=tms.ExampleDecodeMessage -->
<!-- /dmrgo:output -->

A text that requests an acknowledgement, and the acknowledgement:

<!-- dmrgo:output test=tms.ExampleNewText -->
<!-- /dmrgo:output -->

## How it works

Each radio has an IPv4 address of the form network.x.y.z, where x.y.z is its 24-bit radio ID and the network ID is <!-- dmrgo:const pkg=ip name=DefaultNetwork --><!-- /dmrgo:const --> unless the radio is set up with another one. A data packet with the SAP IP based packet data carries the IPv4 header, the UDP header and the payload. A packet with the SAP UDP/IP header compression carries a compressed header in place of both, and `ip.Decode` reads that form too. (ETSI TS 102 361-3 clauses 5.1, 5.5 and 5.6)

An ARS message starts with a 2-octet length, then one or two header octets: the PDU type, and for a registration the event, or for an answer the refresh time. A registration then holds the radio ID as decimal ASCII digits, the user identifier and the password. A TMS message has the same length field, a header octet with the PDU type and the acknowledgement request, an address field that is empty in radio to radio messages, two octets with the sequence number and the text encoding, and the text in UTF-16LE. Radios start the text with a CR LF. The packages do no I/O and allocate only to grow a slice of the caller.

The packets go on the channel as confirmed data. The state machines: [data Sender](../machines/data-sender.md) and [data Receiver](../machines/data-receiver.md). The [csbk Preambles](../machines/csbk-preambles.md) follows the preamble CSBKs that come before a data packet.

## Related guides

- [Send SMS and confirmed data with retries](send-data.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
- [Run TCP or UDP over DMR data](tcp-udp-over-dmr.md)
- [Radio check, call alert and other CSBK services](csbk-services.md)
