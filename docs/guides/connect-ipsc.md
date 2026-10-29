# Connect to an IPSC system

This guide shows how to join a Motorola IP Site Connect (IPSC) system as a peer, or run its master, with the HMAC-SHA1 authentication of MOTOTRBO repeaters, and how to pass the voice and data packets of a call.

## Goal

Register a peer with the master of an IPSC system over UDP, get the peer list, keep the link alive, and send and receive the packets of a call. Or run the master that MOTOTRBO repeaters register with.

## What you need

- Go and a checkout of dmrgo. The example runs offline: both sides use UDP on the loopback interface, and it contacts no network.
- The packages [transport/ipsc](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc), [transport/ipsc/ipscsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession) and [transport/ipsc/ipscauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth).
- The peer ID of your side and, when the system uses authentication, its key: the hex key of the MOTOTRBO or DMRlink configuration.

Run the example from the top of the repository:

```sh
go test -C v2 -run 'ExamplePeer_loopback$' -v ./transport/ipsc/ipscsession
```

## Steps

### 1. Parse the key and make the Peer and the Master

[ipscauth.ParseKey](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth#ParseKey) reads the hex key of the configuration and pads it on the left to 20 octets, as DMRlink does. Set `Auth` and `Key` in the config of each side. [ipscsession.NewPeer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#NewPeer) takes the peer ID and the address of the master as an [Addr](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#Addr). [AddrOf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#AddrOf) converts the IP and port of a `net.UDPAddr`, as the example does for each packet that it reads. [ipscsession.NewMaster](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#NewMaster) makes the master.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go lines=37-65 -->
<!-- /dmrgo:code -->

### 2. Pass packets between the socket and the machines

The machines do no I/O. Give each UDP packet to `Packet` of the machine with the address that it came from. Send the `Packet` of each `OutputSend` output to its `Addr`: an IPSC peer also talks to the other peers of the list, so each output has its own address. Call `Tick` at the time that `Deadline` returns. The first `Tick` of a Peer sends the registration request, and later ticks send the keepalives.

The Peer returns `OutputUp` when the master answers its registration, and `OutputPeers` when it gets a peer list. It then registers with each other peer of the list. The Master returns `OutputUp` for each new peer.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go lines=68-121 -->
<!-- /dmrgo:code -->

### 3. Send and receive the packets of a call

With `Auth`, sign each voice or data packet with [Peer.Sign](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#Peer.Sign) before you send it. The machines check the trailer of each packet that they get, and return the voice and data packets of registered peers as `OutputData`, without the trailer. [ipsc.DecodeUser](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#DecodeUser) decodes one. The Master does not send them on by itself; send each one to the other peers that should get it.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go lines=134-143 -->
<!-- /dmrgo:code -->

An IPSC user packet carries the decoded payload of a burst in place of the 264-bit burst: three AMBE+2 frames without FEC, or the payload of a data burst. [User.RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.RawBurst) rebuilds the burst, [User.SetRawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.SetRawBurst) takes one apart, and [User.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.Hint) gives the `layer2.Hint` for the voice and data machines. The hint marks voice bursts A and E only; a [layer2.PositionTracker](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#PositionTracker) places B, C, D and F. To build the packets of a call from scratch, [ipsc.UserOpcode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#UserOpcode) picks the opcode and an [ipsc.Stream](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#Stream) fills the RTP header and the call fields of each packet.

<!-- dmrgo:code file=v2/transport/ipsc/example_test.go lines=14-26 -->
<!-- /dmrgo:code -->

## Output

The registration with the master, the peer list, and the voice LC header of a call at the master:

<!-- dmrgo:output test=ipscsession.ExamplePeer_loopback -->
<!-- /dmrgo:output -->

A voice LC header and a voice burst from a Motorola XPR 8400:

<!-- dmrgo:output test=ipsc.ExampleDecodeUser -->
<!-- /dmrgo:output -->

The authentication trailer of a registration request:

<!-- dmrgo:output test=ipscauth.ExampleKey_Sign -->
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=transport/ipsc/ipscsession type=PeerConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=transport/ipsc/ipscsession type=MasterConfig -->
<!-- /dmrgo:config -->

## How it works

A peer sends a registration request to the master, and the master answers with a registration reply. The peer then requests the peer list, registers with each other peer of the list, and sends keepalives to the master and to each peer every `Alive`. It takes a link as down after `MaxMissed` keepalives without a reply. The master sends the peer list to every peer when one requests it and when a peer leaves, times out or is dropped. It drops a peer after `Timeout` without a packet. The defaults of these timers are those of DMRlink.

With authentication, each packet ends with a 10-octet trailer: the first 10 octets of the HMAC-SHA1 of the packet, with the 20-octet key. A side with `Auth` drops a packet without a good trailer and signs what it sends. A Master can serve repeaters with keys of their own: pass the key of the sending peer to `Master.Packet`.

The voice and data packets have an RTP header, and octet 17 holds the call info: the slot, the end of the call and the privacy bit. (DMRlink 1f50db2 dmrlink.py and ipsc_const.py; ipsc2hbrp; captures of a Motorola XPR 8400, 2026-10-15 to 2026-10-20)

The state machines: [ipscsession Peer](../machines/ipscsession-peer.md) and [ipscsession Master](../machines/ipscsession-master.md).

## Related guides

- [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md)
- [Connect to a Hytera repeater](connect-hytera.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Register radios and send text: ARS and TMS](register-radios-send-text.md)
