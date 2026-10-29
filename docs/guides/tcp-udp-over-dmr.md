# Run TCP or UDP over DMR data

This guide shows how to carry UDP datagrams and IPv4 packets over DMR confirmed data between two radios, with the results of two simulated sessions.

## Goal

Connect an IP application to a DMR channel with a data Sender and a data Receiver at each radio.

## What you need

- Go and a checkout of dmrgo. Both examples run offline, on a simulated channel with bit errors.
- The packages [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data) and [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip).
- For TCP, a TCP/IP stack. The tcp example uses the netstack of gVisor.

The udp and tcp examples are their own Go modules, so their dependencies (zstd for udp, gVisor for tcp) do not become dependencies of dmrgo. Run them from the top of the repository:

```sh
go run -C v2/examples/udp .
go run -C v2/examples/tcp .
```

The README of each example describes its parts, its flags, the columns of its tables and what the results show: [udp](../../v2/examples/udp/README.md), [tcp](../../v2/examples/tcp/README.md).

## Steps

### 1. Make a Sender and a Receiver for each radio

A [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) sends packets and a [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) receives them. Set `Address` to the radio ID of the radio and `Respond` so that the Receiver answers confirmed packets. Make the output and event slices with capacity `data.MaxOutputs` and `data.MaxEvents`, so that they never grow.

<!-- dmrgo:code file=v2/examples/udp/link.go lines=48-63 -->
<!-- /dmrgo:code -->

### 2. Send a UDP datagram with a compressed header

[ip.AppendCompressed](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#AppendCompressed) puts a compressed UDP/IPv4 header in front of the payload. Send it in a confirmed packet with the SAP UDP/IP header compression. A compressed header has no IPv4 addresses, so the LLIDs of the data header carry the addresses. The Sender takes one packet at a time: send the next one when its state is `data.SenderIdle` again. (ETSI TS 102 361-3 clause 7.2.3)

<!-- dmrgo:code file=v2/examples/udp/link.go lines=70-94 -->
<!-- /dmrgo:code -->

### 3. Send an IPv4 packet

For TCP, send each IPv4 packet of the stack as one confirmed packet with the SAP IP based packet data. The MTU of the stack must fit one DMR packet. `Send` returns `data.ErrBusy` while the Sender has a packet in progress, so keep the packet in the queue and try again later. (ETSI TS 102 361-3 clause 5.5)

<!-- dmrgo:code file=v2/examples/tcp/link.go lines=56-75 -->
<!-- /dmrgo:code -->

### 4. Drive the machines with the clock and the channel

The machines do no I/O. Call `Tick` at the time that `Deadline` returns, and pass each burst that the radio hears to `Burst` of both machines. Call `Channel` when the channel goes busy or idle. A polite Sender waits for an idle channel before it starts a try.

<!-- dmrgo:code file=v2/examples/udp/link.go lines=96-119 -->
<!-- /dmrgo:code -->

### 5. Send the outputs and deliver the packets

An `OutputBurst` is a burst to send at its `Due` time, and `OutputDone` ends a packet with its result and its count of tries. An `EventRespond` is a response burst that the Receiver sends, and `EventDelivered` holds a packet. [ip.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Decode) reads the datagram from the user data for the SAP of the header.

<!-- dmrgo:code file=v2/examples/udp/link.go lines=121-153 -->
<!-- /dmrgo:code -->

## Output

The udp example sends filled forms in different encodings, at each data rate and bit error rate, and prints the size of each encoding and the time from a form to its acknowledgement.

<!-- dmrgo:output example=udp -->
<!-- /dmrgo:output -->

The tcp example runs a TN3270 session over TCP, on a simplex and a duplex channel, with and without a DMR window, and prints the time of each screen.

<!-- dmrgo:output example=tcp -->
<!-- /dmrgo:output -->

## How it works

The DMR IP bearer service carries an IPv4 packet in the user data of a data packet. With the SAP IP based packet data, the packet has its full IPv4 header. With the SAP UDP/IP header compression, a short header replaces the IPv4 and UDP headers, and the ports <!-- dmrgo:const pkg=ip name=PortText --><!-- /dmrgo:const --> and <!-- dmrgo:const pkg=ip name=PortLocation --><!-- /dmrgo:const --> need no extended header. [ip.RadioAddress](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#RadioAddress) gives the IPv4 address of a radio ID, for a stack that needs one. (ETSI TS 102 361-3 clauses 5.5, 5.6 and 7.2.3)

A DMR ACK covers one radio hop, and the IP bearer service delivers a packet again after its response is lost. So an application on UDP needs its own acknowledgement and its own check for copies, as the form protocol of the udp example has. (ETSI TS 102 361-3 clause 5.4.1.3)

By default a Sender uses stop and wait: each packet requests a response. `SenderConfig.Window` turns on a sliding window, where only the last packet of a window requests a response. The Receiver keeps the blocks of the last packet only, so one lost block makes the Sender send that packet and every later packet of the window again. The tcp results show when a window helps. (ETSI TS 102 361-3 clause 5.4.4)

The state machines: [data Sender](../machines/data-sender.md) and [data Receiver](../machines/data-receiver.md).

## Related guides

- [Send SMS and confirmed data with retries](send-data.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
