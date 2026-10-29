# dmrdsession Master

This page covers the dmrdsession Master, the state machine of the master side of the session of one repeater, for the MMDVM network protocol or the HomeBrew Repeater Protocol (HBRP).

## What it does

A [Master](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master) is the master side of the session of one repeater. It speaks one of two dialects of package [dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd): the MMDVM network protocol of MMDVMHost and DMRGateway ([DialectMMDVM](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DialectMMDVM)), or the HomeBrew Repeater Protocol in the hex text form of its 2015 specification and of BrandMeister ([DialectHBRP](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DialectHBRP)). It answers each login step. It reports the RPTL and the RPTC to you before it answers them, so that you decide which repeaters log in and with which password. It takes the RPTO of a logged-in repeater with an RPTACK, as DMRGateway waits for it before it sends traffic. It answers a packet from a repeater that is not logged in with MSTNAK. It ends the session after LoginTimeout without a packet during the login, and Timeout after it. It does no I/O. (DL5DI, G4KLX and DG1HT, IPSC Protocol Specs for homebrew DMR repeater, 2015-07-26; HBlink3 1db2730 hblink.py master_maintenance_loop; DMRGateway ee79d38)

## When to use it

Use a Master in a server that repeaters, hotspots and gateways log in to: a network master, a test master for a repeater, or a bridge between networks. Keep one Master for each source address that sends you packets, and pass it the packets of that address. A Master makes no random numbers and keeps no password list: you draw the salt and look up the password when the Master reports a login.

## Config

Pass a [MasterConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#MasterConfig) to [NewMaster](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewMaster). The config is required, and the dialect must be the MMDVM network protocol or HBRP.

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=MasterConfig -->
<!-- /dmrgo:config -->

## States and transitions

[State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.State) returns the current state, and [ID](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.ID) the repeater ID from the last RPTL. The Peer, the Master and the Bridge share the [State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#State) type, and each uses some of its values.

<!-- dmrgo:states pkg=transport/dmrd/dmrdsession type=Master -->
<!-- /dmrgo:states -->

An RPTL starts a login: the Master reports it and waits for [Admit](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Admit) or [Refuse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Refuse). Admit sends the salt. An RPTK with the token of the salt and the password gets an acknowledgement, and the Master waits for the RPTC. The Master reports the RPTC and waits for [Accept](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Accept) or Refuse. Accept completes the login and the session runs. A wrong token, Refuse, an RPTCL, a new RPTL of a logged-in repeater, the login timeout, the timeout, or [Close](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Close) ends the session. [Resume](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Resume) starts a logged-in session without a login, for a repeater that logged in to another replica of a server and now sends its packets here.

## Inputs and outputs

Inputs:

- [Packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Packet) takes each UDP packet from the repeater. Each packet that the Master takes restarts Timeout.
- [Admit](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Admit), [Accept](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Accept) and [Refuse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Refuse) answer the login and config reports.
- [Close](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Close) sends MSTCL, as a master that shuts down sends it.
- [Resume](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Resume) starts a session without a login.
- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Tick) runs the timeout. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Deadline) returns.
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Master.Reset) ends the session without a packet or an output.

Outputs are [Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Output) values with an [OutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#OutputKind): packets to send to the repeater; the login and the config to check; the accepted options; the session up and down, with a [Reason](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Reason); the DMRD, DMRA and DMRG packets; the RPTG, RPTRSSI and RPTINTR messages; and each ping that the Master answered, so you can keep the time of the last ping. Every output but a send carries the repeater ID. The packet of a send output points into the Master and is valid until the next call. The packets of the other outputs point into the packet that you passed in. Copy a packet that you keep.

Give each method a slice with capacity <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=MaxOutputs --><!-- /dmrgo:const -->.

## Timers

- LoginTimeout: the time after the first RPTL, and after the RPTK that proves the password, in which the repeater must finish its login. The default is <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=LoginTimeoutDefault --><!-- /dmrgo:const -->, the time after which an HBlink3 master drops a repeater. A repeated RPTL does not restart it.
- Timeout: the time without a packet from the repeater after which the Master ends the session. The default is <!-- dmrgo:const pkg=transport/dmrd/dmrdsession name=TimeoutDefault --><!-- /dmrgo:const -->.

Tick sends nothing when the session times out. A packet of the repeater after that gets MSTNAK, so the repeater logs in again.

## Worked example

The package example logs a [Peer](dmrdsession-peer.md) into a Master over the MMDVM network protocol in memory. The Master reports the config, which the example accepts, and the options:

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/example_test.go lines=11-82 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=transport/dmrd/dmrdsession.Example -->
<!-- /dmrgo:output -->

The next example resumes the session of a repeater, lets it time out, and shows the MSTNAK that answers the next ping of the repeater.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/example_test.go lines=127-152 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=transport/dmrd/dmrdsession.ExampleMaster -->
<!-- /dmrgo:output -->

## Errors

Packet, Tick and the answer methods return no errors. NewMaster returns an error for a nil config, and a configuration error for a dialect other than the MMDVM network protocol or HBRP, or a negative timer.

<!-- dmrgo:errors pkg=transport/dmrd/dmrdsession type=Master -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [dmrdsession Peer](dmrdsession-peer.md): the repeater side of the same protocols.
- [dmrdsession Bridge](dmrdsession-bridge.md): OpenBridge links to other masters.
- [ipscsession Master](ipscsession-master.md): the master side of IPSC.
- [voice Jitter](voice-jitter.md): puts the voice packets of each repeater in order.

Guides:

- Connect to a network (MMDVM network protocol, HBRP, IPSC, Hytera)
- Rewrite and relay a voice call
