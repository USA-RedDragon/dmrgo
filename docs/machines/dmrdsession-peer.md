# dmrdsession Peer

This page covers the dmrdsession Peer, the state machine of the repeater side of a session of the MMDVM network protocol or of the HomeBrew Repeater Protocol (HBRP).

## What it does

A [Peer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer) is the repeater side of one session with a master, as a repeater, a hotspot or DMRGateway runs it. It speaks one of two dialects of package [dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd): the MMDVM network protocol of MMDVMHost and DMRGateway ([DialectMMDVM](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DialectMMDVM)), which BrandMeister, DMR+, HBlink, FreeDMR and DMRHub masters accept, or the HomeBrew Repeater Protocol in the hex text form of its 2015 specification and of BrandMeister ([DialectHBRP](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DialectHBRP)). The two dialects share the DMRD packet and differ in the login and keepalive messages. The Peer follows DMRGateway: it sends RPTL, RPTK with the SHA-256 token of the salt and the password, RPTC and, when it has options, RPTO, each after the acknowledgement of the one before. It sends a step again every Retry, pings every Retry when logged in, and logs in again after Timeout without a reply, after MSTNAK and after MSTCL. It does no I/O: you send and receive the UDP packets. (DMRGateway ee79d38 DMRNetwork.cpp; DL5DI, G4KLX and DG1HT, IPSC Protocol Specs for homebrew DMR repeater, 2015-07-26)

## When to use it

Use a Peer to connect a repeater, a hotspot, a gateway or a test client to a master that speaks the MMDVM network protocol or HBRP. Pick the dialect of the master: most masters today take the MMDVM network protocol. The Peer handles the session only. Decode and encode the DMRD packets with package dmrd, and give the voice to a [Jitter](voice-jitter.md) and a [Receiver](voice-receiver.md).

## Config

Pass a [PeerConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#PeerConfig) to [NewPeer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewPeer). The config is required. Config is the RPTC message of the dialect: build it with [dmrd.MMDVMConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#MMDVMConfig) or [dmrd.HBRPConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#HBRPConfig), with the same repeater ID as the config. Options apply to the MMDVM network protocol only: HBRP has no RPTO. Set Backoff to wait longer after each failed login.

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=PeerConfig -->
<!-- /dmrgo:config -->

## States and transitions

[State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.State) returns the current state. The Peer, the Master and the Bridge share the [State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#State) type, and each uses some of its values.

<!-- dmrgo:states pkg=transport/dmrd/dmrdsession type=Peer -->
<!-- /dmrgo:states -->

The first Tick sends the RPTL. Each acknowledgement moves the Peer to the next login step: the token, the config, and the options when it has them. The acknowledgement of the last step completes the login, and the Peer reports that the session is up. When the session ends, by Timeout without an acknowledgement or a pong, by MSTNAK, or by MSTCL, the Peer reports it down and waits before it logs in again. A session that ends before the login completes doubles the wait, from Retry up to Backoff. [Close](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Close) ends the session for good, until [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Reset).

## Inputs and outputs

Inputs:

- [Packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Packet) takes each UDP packet from the master.
- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Tick) runs the timers. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Deadline) returns.
- [Close](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Close) sends RPTCL when the Peer is logged in, as DMRGateway does when it closes.
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Peer.Reset) makes the Peer new, so the next Tick sends the RPTL.

Outputs are [Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Output) values with an [OutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#OutputKind): a packet to send to the master, the session up, the session down with a [Reason](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Reason) and the state it ended in, and the DMRD, DMRA, DMRG and RPTSBKN packets of a logged-in session. The packet of a send output points into the Peer and is valid until the next call. The packet of a data output points into the packet that you passed in. Copy a packet that you keep.

Give each method a slice with capacity <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=MaxOutputs --><!-- /dmrgo:const -->. The Peer drops a packet longer than <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=MaxPacket --><!-- /dmrgo:const --> octets.

## Timers

- Retry: the time between two sends of a login step, between two pings, and before a new login. The default is <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=RetryDefault --><!-- /dmrgo:const -->, as DMRGateway sets it.
- Timeout: the time without an acknowledgement or a pong after which the Peer ends the session. The default is <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=TimeoutDefault --><!-- /dmrgo:const -->.
- Backoff: the longest wait before a new login. By default the Peer waits Retry each time, as DMRGateway does.

## Worked example

The package example logs a Peer into a [Master](dmrdsession-master.md) over the MMDVM network protocol in memory, and passes each packet that one sends to the other:

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/example_test.go symbol=Example -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=transport/dmrd/dmrdsession.Example -->
<!-- /dmrgo:output -->

The next example runs an HBRP Peer whose master does not answer, with short timers. The Peer sends the RPTL again each Retry, ends the session at Timeout, and logs in again after the Retry wait.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/example_test.go symbol=ExamplePeer -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=transport/dmrd/dmrdsession.ExamplePeer -->
<!-- /dmrgo:output -->

## Errors

Packet and Tick return no errors. The Peer ignores a packet it cannot use. NewPeer returns an error for a nil config, and a configuration error for a dialect other than the MMDVM network protocol or HBRP, a Config that is not an RPTC of the dialect for the ID, Options with HBRP or longer than <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=MaxOptions --><!-- /dmrgo:const --> octets, or a negative timer.

<!-- dmrgo:errors pkg=transport/dmrd/dmrdsession type=Peer -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [dmrdsession Master](dmrdsession-master.md): the master side of the same protocols.
- [dmrdsession Bridge](dmrdsession-bridge.md): OpenBridge links between masters.
- [ipscsession Peer](ipscsession-peer.md): the peer side of IPSC.
- [voice Jitter](voice-jitter.md) and [voice Receiver](voice-receiver.md): take the voice of the DMRD packets.
- bs Repeater (docs/machines/bs-repeater.md): a repeater whose network side a Peer can carry.

Guides:

- Connect to a network (MMDVM network protocol, HBRP, IPSC, Hytera)
- Clean up bursts for a hotspot
