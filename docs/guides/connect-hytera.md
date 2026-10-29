# Connect to a Hytera repeater

This guide shows how to read and build the UDP packets of Hytera IP Multi-Site Connect: the P2P service that registers a repeater, and the DMR service that carries one burst per packet.

## Goal

Take part in the P2P service of a Hytera repeater, and send and receive the bursts of a call as frames of the DMR service.

## What you need

- Go and a checkout of dmrgo. The examples run offline: the call example uses UDP on the loopback interface, and it contacts no network.
- The package [transport/hytera](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera), and [voice](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice) for the call.

Package hytera encodes and decodes packets and keeps no state. It has no session machine: your program runs the P2P service, its timers and the sockets of each service. Hytera publishes no specification. The layouts follow real captures, and a field whose meaning no source proves is kept as received, so a decode followed by an encode gives the same octets.

Run the examples from the top of the repository:

```sh
go test -C v2 -run Example -v ./transport/hytera
```

## Steps

### 1. Run the P2P service

A repeater sends P2P commands to the master to request a service: registration, DMR or RDAC. The master accepts the request and redirects the repeater to the UDP port of the service, and the repeater acknowledges the redirect. [hytera.DecodeP2P](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#DecodeP2P) decodes a command. [P2P.Service](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#P2P.Service) returns the service that it is about, and [P2P.RedirectPort](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#P2P.RedirectPort) the port of a redirect. `P2P.AppendBinary` builds a command. The field docs of [P2P](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#P2P) give the values that the captures show for each step.

Each side also sends a heartbeat. [hytera.DecodeHeartbeat](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#DecodeHeartbeat) reads the 20-octet heartbeat of the P2P service and the 1-octet keepalive of the DMR service. Answer a heartbeat with the reply that the master sends in the capture:

<!-- dmrgo:code file=v2/transport/hytera/example_test.go lines=31-37 -->
<!-- /dmrgo:code -->

### 2. Read the bursts of a call

Each packet of the DMR service is one 72-octet [Frame](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame). [hytera.DecodeFrame](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#DecodeFrame) decodes it. `HasBurst` reports whether it holds a burst, as sync and wakeup packets do not. [Frame.Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame.Call) returns the call, and [Frame.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame.Hint) the `layer2.Hint` for the voice and data machines.

<!-- dmrgo:code file=v2/transport/hytera/example_test.go lines=14-22 -->
<!-- /dmrgo:code -->

### 3. Send the bursts of a call

Take the outputs of a machine, such as a [voice.Transmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter), and put each burst in a Frame. [Frame.SetCallHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame.SetCallHint) sets the call and the slot type from the `Call` and `Hint` of the output. Count `Seq` up for each packet. The example sets `PacketType` to the values of the bursts and the terminator in the capture of a Hytera repeater.

The example sends a group call over the loopback interface and follows it with a [voice.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) on the other side:

<!-- dmrgo:code file=v2/transport/hytera/udp_example_test.go lines=42-109 -->
<!-- /dmrgo:code -->

## Output

The group call over the loopback interface:

<!-- dmrgo:output test=hytera.ExampleFrame_SetCallHint -->
<!-- /dmrgo:output -->

A voice LC header frame from a capture, and the heartbeat reply:

<!-- dmrgo:output test=hytera.ExampleDecodeFrame -->
<!-- /dmrgo:output -->

<!-- dmrgo:output test=hytera.ExampleDecodeHeartbeat -->
<!-- /dmrgo:output -->

## How it works

A Frame holds the slot, the slot type, which gives the kind of burst, the color code, the 33-octet burst, the call type and the source and destination IDs. The octet pairs of the burst are swapped in the packet, and `DecodeFrame` and `AppendBinary` swap them back. The call type is 0x01 for a group call from a Hytera repeater, but BrandMeister sends 0x00 on group calls too, so `Frame.Call` takes the group flag from the LC of a voice LC header or terminator when it decodes. `Frame.Hint` takes the data type from the slot type field of the burst when the burst has a data SYNC, because Hytera_Homebrew_Bridge sends CSBK and data bursts with the terminator slot type.

The RDAC service and the Hytera application protocols HRNP and HSTRP are not in the package. HDAP is in package [hytera/hdap](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/hytera/hdap). (OK-DMR dmr-kaitai 7489ff1 ip_site_connect_protocol.ksy and ip_site_connect_heartbeat.ksy; OK-DMR Hytera_Homebrew_Bridge 05e1bf9 and its captures)

The bursts go to and from the voice machines: [voice Transmitter](../machines/voice-transmitter.md) and [voice Receiver](../machines/voice-receiver.md).

## Related guides

- [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md)
- [Connect to an IPSC system](connect-ipsc.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Use an MMDVM modem](mmdvm-modem.md)
