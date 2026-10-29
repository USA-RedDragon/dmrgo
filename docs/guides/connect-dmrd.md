# Connect to a network with the MMDVM network protocol or HBRP

This guide shows how to log a repeater or hotspot into a master with the MMDVM network protocol or the HomeBrew Repeater Protocol (HBRP), how to run the master side, and how to pass the DMRD packets of a call.

## Goal

Run the repeater side (Peer) and the master side (Master) of a session over UDP, log in with the password of the master, and send the bursts of a call in DMRD packets.

## What you need

- Go and a checkout of dmrgo. The example runs offline: both sides use UDP on the loopback interface, and it contacts no network.
- The packages [transport/dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd) and [transport/dmrd/dmrdsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession).
- The dialect of your master. Package dmrd covers four protocols built around the same DMRD packet. They differ in the login, keepalive and other control messages:
  - [dmrd.DialectMMDVM](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Dialect): the MMDVM network protocol of MMDVMHost and DMRGateway. BrandMeister, DMR+, HBlink, FreeDMR and DMRHub masters accept it. It has binary repeater IDs, RPTACK, RPTPING and MSTPONG.
  - `dmrd.DialectHBRP`: the HomeBrew Repeater Protocol, in the form of its 2015 specification and of the Homebrew port of BrandMeister. It has repeater IDs, salt and token as hex text, MSTACK, MSTPING and RPTPONG, and no RPTO.
  - `dmrd.DialectLocal`: the link between MMDVMHost and DMRGateway on one host, without a login.
  - OpenBridge, the link between two masters. A [dmrdsession.Bridge](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Bridge) checks its signatures.

The MMDVM network protocol and HBRP are different protocols. Pick the dialect that your master speaks.

Run the example from the top of the repository:

```sh
go test -C v2 -run ExamplePeer_loopback -v ./transport/dmrd/dmrdsession
```

## Steps

### 1. Make the RPTC configuration

The repeater sends its configuration in an RPTC message during the login. [dmrd.MMDVMConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#MMDVMConfig) is the RPTC of the MMDVM network protocol and [dmrd.HBRPConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#HBRPConfig) that of HBRP. `Reset` fills the fixed octets, and the setters write the repeater ID, callsign, frequencies, color code and the other fields in place.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go lines=63-74 -->
<!-- /dmrgo:code -->

### 2. Make the Peer and the Master

[dmrdsession.NewPeer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewPeer) takes the dialect, the repeater ID, the password of the master and the RPTC. `PeerConfig.Options` is the RPTO options text, which the Peer sends after the RPTC is acknowledged. [dmrdsession.NewMaster](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewMaster) makes the master side of one repeater. Keep one Master for each address that sends you packets.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go lines=75-82 -->
<!-- /dmrgo:code -->

### 3. Pass packets between the socket and the machines

The machines do no I/O. Read each UDP packet and give it to `Packet` of the machine. Send the `Packet` of each `OutputSend` output to the other side. Call `Tick` at the time that `Deadline` returns: the first `Tick` of a Peer sends the RPTL, and later ticks send the retries and the pings.

At the Master, `OutputLogin` reports an RPTL. Answer it with `Admit` and a random salt and the password of the repeater, or with `Refuse`. `OutputConfig` reports the RPTC. Check it and call `Accept` or `Refuse`. The Peer returns `OutputUp` when the login is complete.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go lines=84-149 -->
<!-- /dmrgo:code -->

### 4. Send and receive the bursts of a call

Once the Peer is up, send each burst in a DMRD packet on the same socket. The Master returns each DMRD packet that it gets from a logged-in repeater as `OutputData`. [dmrd.DecodeData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DecodeData) decodes it, and [Data.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.Hint) gives the `layer2.Hint` for the voice and data machines. To build a DMRD packet from the output of a machine, such as a `voice.Transmitter`, set the call and burst kind with [Data.SetCallHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.SetCallHint) and encode it with `Data.AppendBinary`.

<!-- dmrgo:code file=v2/transport/dmrd/example_test.go lines=13-22 -->
<!-- /dmrgo:code -->

## Output

The login in each dialect, then the voice LC header of a call at the master:

<!-- dmrgo:output test=dmrdsession.ExamplePeer_loopback -->
<!-- /dmrgo:output -->

The DMRD packet and the hint of its burst:

<!-- dmrgo:output test=dmrd.ExampleDecodeData -->
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=PeerConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=MasterConfig -->
<!-- /dmrgo:config -->

## How it works

The Peer follows DMRGateway. It sends RPTL with its repeater ID, and the master answers with a salt. It then sends RPTK with the SHA-256 token of the salt and the password, then RPTC and, when it has options, RPTO, each after the acknowledgement of the one before. It sends a step again every `Retry` when no answer comes, pings every `Retry` when it is logged in, and logs in again after `Timeout` without a reply, after MSTNAK and after MSTCL. Package [dmrdauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth) computes the token.

The Master answers each step and sends `OutputLogin` and `OutputConfig` to the caller before it answers the RPTL and the RPTC. It answers a packet from a repeater that is not logged in with MSTNAK. It ends the session after `LoginTimeout` without a packet during the login, and after `Timeout` after it.

The DMRD packet is the same in every dialect: the sequence number, the source and destination IDs, the repeater ID, the slot, the call type, the frame type and voice position or data type, the stream ID and the 33-octet burst. The 55-octet form adds the bit error count and the RSSI, and the 71-octet form of DMRGateway adds a trunking UUID. `Data.Size` sets the form; its zero value sends 55 octets, as MMDVMHost and DMRGateway do. DecodeData and Data.AppendBinary take no dialect. (DL5DI, G4KLX and DG1HT, IPSC Protocol Specs for homebrew DMR repeater, 2015-07-26; MMDVMHost 9384d68 and DMRGateway ee79d38)

The state machines: [dmrdsession Peer](../machines/dmrdsession-peer.md), [dmrdsession Master](../machines/dmrdsession-master.md) and [dmrdsession Bridge](../machines/dmrdsession-bridge.md).

## Related guides

- [Connect to an IPSC system](connect-ipsc.md)
- [Connect to a Hytera repeater](connect-hytera.md)
- [Use an MMDVM modem](mmdvm-modem.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Clean up bursts for a hotspot](clean-up-bursts.md)
