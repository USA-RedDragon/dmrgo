# Send SMS and confirmed data with retries

This guide shows how to read the text of a received SMS, send unconfirmed and confirmed data packets, and resend only the blocks that the receiver did not get.

## Goal

Build data packets into bursts, reassemble them on the other side, and recover a lost block with a selective retry.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- The SMS captures in `v2/layer2/testdata`. They hold the same text in the formats of different radios.
- The packages [layer2/packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet), [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu) and [sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms). For a radio or a gateway on a live channel, also [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/data
```

## Steps

### 1. Reassemble a received SMS

Give each decoded burst to a [packet.Assembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Assembler). `AddBurst` returns the [Message](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Message) when the packet is complete. [sms.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#Decode) finds the text format from the header and the user data, and returns the text as UTF-8.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=capture -->
<!-- /dmrgo:code -->

### 2. Send an unconfirmed packet

Put a compressed UDP/IPv4 header in front of the text, and make an unconfirmed data header with the SAP UDP/IP header compression. [packet.NewPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#NewPacket) cuts the payload into blocks of the rate that you pick and adds the pad octets and the message CRC. [packet.Bursts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Bursts) codes the header and the blocks as bursts.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=unconfirmed -->
<!-- /dmrgo:code -->

[sms.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#AppendPacket) builds the user data and the data header of a text in each format that radios use, so you do not build the UDP header yourself:

<!-- dmrgo:code file=v2/sms/example_test.go symbol=Example -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=sms.Example -->
<!-- /dmrgo:output -->

### 3. Send a confirmed packet and lose a block

A confirmed data header requests a response. Each block of a confirmed packet has its own sequence number (DBSN) and CRC-9, so the receiver can identify each lost block. The example corrupts one block on the air.

<!-- dmrgo:code file=v2/examples/data/main.go lines=134-159 -->
<!-- /dmrgo:code -->

### 4. Answer with a selective ACK and resend the missing block

The receiving Assembler returns `packet.ErrBlocksMissing`, and `Missing` returns the blocks that it did not get. `Response` builds the SACK packet. At the sender, `RetryBlocks` of the response returns the blocks to send again.

<!-- dmrgo:code file=v2/examples/data/main.go lines=161-190 -->
<!-- /dmrgo:code -->

### 5. Complete the packet and send the ACK

Send the header and the missing block again. The Assembler completes the packet, and [packet.NewResponse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#NewResponse) builds the ACK.

<!-- dmrgo:code file=v2/examples/data/main.go lines=192-222 -->
<!-- /dmrgo:code -->

## Output

The first lines are the received SMS captures. Then come the unconfirmed packet, the confirmed packet with its SACK, and the retry with its ACK.

<!-- dmrgo:output example=data -->
<!-- /dmrgo:output -->

## Send on a live channel

The example above builds every packet and response by hand. On a live channel, a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) sends the CSBK preambles, the header and the blocks at their times, waits for the response, and resends the blocks that a SACK requests. A [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) with `Respond` set reassembles the packet and returns the response bursts to send. Both do no I/O: each method takes the time now and appends outputs or events to a slice of the caller.

In this example the air loses one burst of the first try. The Receiver requests the block again with a SACK, and the Sender resends only that block.

<!-- dmrgo:code file=v2/data/sender_example_test.go symbol=ExampleSender -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=data.ExampleSender -->
<!-- /dmrgo:output -->

The Sender sends <!-- dmrgo:const pkg=data name=DefaultPreambles --><!-- /dmrgo:const --> preamble CSBKs by default before a packet. The config of each machine:

<!-- dmrgo:config pkg=data type=SenderConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=data type=ReceiverConfig -->
<!-- /dmrgo:config -->

## How it works

A data packet is a header block, then the data blocks that the header announces in its blocks to follow (BF) field. The last block ends with the 32-bit message CRC, and the header gives the pad octet count (POC) before it. A confirmed packet has a DBSN and a CRC-9 in each block. The receiver answers with an ACK when it has every block and the message CRC passes, or with a SACK that lists the blocks to send again. A retry header has the Full Message Flag clear, and the Assembler adds the blocks of the retry to the packet in progress. (ETSI TS 102 361-1 clause 8.2, clause B.3.9, clause 8.2.2.3)

The text formats differ by radio: MOTOTRBO TMS, the ETSI IP text, the DMR Standard text and the H-SMS text of AnyTone radios, and ETSI defined data. `sms.Decode` reads all of them. (ETSI TS 102 361-3 clauses 5.7.1 and 6.1)

The state machines: [data Sender](../machines/data-sender.md) and [data Receiver](../machines/data-receiver.md). A program that only watches the channel can follow the preamble CSBKs of a transfer and the header that they announce with the [csbk Preambles](../machines/csbk-preambles.md).

## Related guides

- [Run TCP or UDP over DMR data](tcp-udp-over-dmr.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
- [Decode and re-encode captures](decode-captures.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
