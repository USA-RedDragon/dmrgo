# Report and request location: LIP, LRRP, NMEA

This guide shows how to request the position of a radio and read the position that it reports, in the LRRP, NMEA and LIP formats that DMR radios use.

## Goal

Send a location request to a radio, build its answer, and read the position from the answer.

## What you need

- Go and a checkout of dmrgo. The code on this page comes from Go Example functions, which `go test` runs.
- The packages [location](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location), [mototrbo/lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp), [nmea](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea), [lip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip) and [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip).
- A way to send data packets. [Send SMS and confirmed data with retries](send-data.md) shows how.

Run the examples from the top of the repository:

```sh
go test -C v2 -run Example -v ./location ./lip
```

Which format to use depends on the radio:

| Radios | Format | Carried in |
| --- | --- | --- |
| MOTOTRBO, and Hytera | LRRP request and report | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortLRRP --><!-- /dmrgo:const --> |
| AnyTone | NMEA RMC sentence | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortNMEA --><!-- /dmrgo:const --> |
| Hytera | HDAP location protocol | UDP datagram to port <!-- dmrgo:const pkg=ip name=PortHyteraLocation --><!-- /dmrgo:const --> |
| Radios and systems that use ETSI LIP | LIP PDU | appended data of a UDT (ETSI TS 102 361-4) |

## Steps

### 1. Request a position with LRRP

Make an [lrrp.Message](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp#Message) with an immediate request document and a request ID, and write it with `AppendBinary`. Put it in a UDP datagram to the LRRP port with [ip.AppendIPv4](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#AppendIPv4). [ip.RadioAddress](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#RadioAddress) gives the IPv4 address of each radio ID. Send the datagram in a data packet with the SAP IP based packet data.

<!-- dmrgo:code file=v2/location/example_test.go lines=105-131 -->
<!-- /dmrgo:code -->

[location.Decoder.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Decoder.Decode) returns `location.ErrNoFix` for a request, so a program that reads every data packet on a channel can give each one to the Decoder.

### 2. Answer the request and read the position

The radio reads the request with [lrrp.DecodeMessageInto](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp#DecodeMessageInto) and answers with an immediate report that carries the same request ID, a result code and a position. The Decoder reads the answer into a [location.Fix](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Fix), with the request ID that it answers.

<!-- dmrgo:code file=v2/location/example_test.go lines=133-170 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=location.ExampleDecoder_Decode_lrrp -->
<!-- /dmrgo:output -->

An `lrrp.Message` is large, and a `location.Decoder` holds one. Keep them in a struct or a package variable, not in a local variable on a small stack, as the examples do.

### 3. Send a position as NMEA

[location.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#AppendPacket) writes a Fix as the NMEA RMC sentence of an AnyTone radio, behind a compressed UDP/IPv4 header in an unconfirmed packet, and sets the data header in a [location.Header](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/location#Header). The same Decoder reads it back.

<!-- dmrgo:code file=v2/location/example_test.go lines=78-94 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=location.ExampleAppendPacket -->
<!-- /dmrgo:output -->

To read or write the sentence alone, use [nmea.ParseRMC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea#ParseRMC) and [nmea.AppendRMC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/nmea#AppendRMC). `ParseRMC` returns an error for a sentence with a bad checksum.

### 4. Request a position with LIP

A LIP PDU is a string of bits. Encode an [lip.ImmediateRequest](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#ImmediateRequest) with the report type that you want, and put it in the appended data of a UDT with [trunk.Appended.SetLIP](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Appended.SetLIP). The radio reads the PDU type with [lip.DecodeHeader](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#DecodeHeader) and the request with `DecodeImmediateRequest`. (ETSI TS 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 clause 6.2.16)

<!-- dmrgo:code file=v2/lip/example_test.go lines=39-67 -->
<!-- /dmrgo:code -->

### 5. Answer with a LIP long location report

The radio answers with an [lip.LongReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#LongReport) of the type that the request specified, here with the time of the position. [lip.DecodeLongReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/lip#DecodeLongReport) reads it. Velocities and accuracies stay codes, as the spec codes them in steps. (ETSI TS 100 392-18-1 clause 6.2.2)

<!-- dmrgo:code file=v2/lip/example_test.go lines=69-93 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=lip.ExampleDecodeImmediateRequest -->
<!-- /dmrgo:output -->

## How it works

The LRRP, NMEA and HDAP formats all travel in UDP datagrams in DMR data packets. `location.Decoder` reads the datagram with [ip.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Decode), for a full IPv4 header or a compressed one, and selects the format from the destination UDP port. It returns one `Fix` for all formats, with `Format` set to the format of the report. For an answer without a position, such as the answer of a radio without a GNSS fix, it returns the Fix with its result code and `location.ErrNoPosition`.

An LRRP message is a document of tokens in MOTOTRBO binary XML. A triggered request requests reports on a period or a distance, and a stop request ends it. The [lrrp](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp) package doc describes the coding of the latitude and longitude.

LIP is the location protocol of ETSI TS 100 392-18-1. A DMR UDT carries it in the LIP format, and the USBD polling service of a trunked system compresses it. The lip package reads and writes the long location report, the report acknowledgement, the immediate request and the enable and disable request and response. [trunk.DecodeLIPShortReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#DecodeLIPShortReport) reads the short location report. (ETSI TS 102 361-4 clause B.3.10, ETSI TS 100 392-18-1 clause 6)

To send the data packets on a channel, use the [data Sender](../machines/data-sender.md) and read them with the [data Receiver](../machines/data-receiver.md). On a Tier III system, a UDT goes through the short data services of the [trunk MS](../machines/trunk-ms.md).

## Related guides

- [Send SMS and confirmed data with retries](send-data.md)
- [Run TCP or UDP over DMR data](tcp-udp-over-dmr.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md), which reads the GPS position in the embedded LC of a voice call
