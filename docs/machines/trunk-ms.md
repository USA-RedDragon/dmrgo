# trunk MS

This page covers the trunk MS, the state machine of a DMR Tier III mobile station that registers on a trunked site and makes calls.

## What it does

An [MS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS) is a Tier III radio. It hunts for and follows a TSCC as a [Follower](trunk-follower.md) does. It also registers and de-registers, makes voice and packet data calls by random access, answers C_AHOY and P_AHOY, follows C_MOVE and P_PROTECT, and ends its own calls with P_MAINT. It sends its random access requests in the inbound slot that the AT bit of the CACH shows as free, with a random backoff. It also takes part in the UDT services: short data, polling, call diversion and DGNA, and full-duplex and Ambient Listening Service calls. The MS returns the bursts to send with their Due times and does no I/O. You send the voice of a granted call with a [voice Transmitter](voice-transmitter.md) with the MS SYNC. (ETSI TS 102 361-4 clauses 6.2 to 6.6, 6.6.2.6 and 6.6.4 to 6.6.11)

## When to use it

Use an MS to build a Tier III radio, a dispatcher console on a radio link, a test tool for a trunked site, or a simulator that runs MSs against a [TSCC](trunk-tscc.md). When you only listen, use a Follower: it never transmits.

## Config

Pass an [MSConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MSConfig) to [NewMS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#NewMS). The individual address is required, so a nil config returns an error. Set the talkgroups whose calls the MS follows. The system code, hunt list and channel plan work as in the Follower. Set a different Seed for each MS of a simulation, so that their random access backoffs differ. Set Auth to authenticate the TSCC before a stun, revive or kill.

<!-- dmrgo:config pkg=trunk type=MSConfig -->
<!-- /dmrgo:config -->

## States and transitions

[State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.State) returns the channel state of the MS. It is the [State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#State) type of the Follower.

<!-- dmrgo:states pkg=trunk type=State -->
<!-- /dmrgo:states -->

The channel states move as for the Follower: hunting, then the confirmed TSCC, then the payload channel of a call, then back to the TSCC. The MS follows its own calls and the individual calls to it, in addition to its talkgroups. It also leaves a TSCC whose signal level falls below MinLevelIdle, as [Quality](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Quality) reports it, or whose bit error rate is too high. (ETSI TS 102 361-4 clauses 6.3.2.3 and 6.3.3.1)

On the TSCC, a request runs beside the channel state. [Busy](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Busy) reports whether a registration, de-registration or call request runs. The MS registers when the TSCC requests registration, unless NoRegistration is set. [Registered](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Registered) reports whether it holds a registration for the TSCC that it is on. A call request ends with a grant and a tune request, or with a refusal. [Off](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Off), [Stunned](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Stunned), [Killed](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Killed) and [Asleep](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Asleep) report the other conditions of the MS. These have no state type, so the table above does not show them.

## Inputs and outputs

Inputs from the channel:

- [Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Burst) and [Decoded](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Decoded) take each outbound burst, its [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint), and the CACH field before it. Pass the time at the start of the burst. Set Hint.Slot, because the MS counts TDMA frames by the bursts of its slot. Random access needs the CACH for its AT bit.
- [Lost](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Lost) reports that the receiver lost the signal, and [Quality](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Quality) gives the signal level of the TSCC.
- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Tick) runs the timers. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Deadline) returns.

Inputs from the user:

- [Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Call) and [CallData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.CallData) request a voice or packet data call. [CallDuplex](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.CallDuplex), [CallDuplexData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.CallDuplexData) and [CallAmbient](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.CallAmbient) request full-duplex and Ambient Listening Service calls.
- [Cancel](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Cancel) stops the running request, and [EndCall](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.EndCall) ends the own call on the payload channel.
- [PowerOff](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.PowerOff) de-registers the MS before it is switched off.
- [AuthResponse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.AuthResponse) answers an authentication challenge of the TSCC.
- The services: [SendStatus](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.SendStatus), [PollStatus](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.PollStatus), [Interrupt](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Interrupt), [AdviseIP](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.AdviseIP), [SendShortData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.SendShortData), [PollShortData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.PollShortData), [Divert](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Divert) and [AssignGroups](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.AssignGroups).
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS.Reset) clears the site, the registration and any request. It keeps a stun or kill.

Outputs are [Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Output) values with an [OutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#OutputKind). In addition to the outputs of a Follower, an MS returns bursts to send in a slot at a Due time, the registration, refusals with a [Reason](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Reason) and the C_NACKD reason, C_AHOY calls, and the results of the services. Send each burst at its Due time. Give each method a slice with capacity <!-- dmrgo:const pkg=trunk name=MaxOutputs --><!-- /dmrgo:const -->.

## Timers

- TRand_TC, the longest time of a random access: <!-- dmrgo:const pkg=constants name=TRandTCDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 clause 6.2.1.1.7)
- TP_Timer, the longest wait for the grant of a call: <!-- dmrgo:const pkg=constants name=TPTimerDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 Table A.1)
- TNP_Timer, the longest wait for the answer to a registration after an intermediate acknowledgement: <!-- dmrgo:const pkg=constants name=TNPTimerDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 Table A.1)
- T_Pending, the wait of a called MS for the grant after it accepted a C_AHOY: <!-- dmrgo:const pkg=constants name=TPendingDefault --><!-- /dmrgo:const --> by default.
- T_Dereg, the longest wait for the answer to a de-registration: <!-- dmrgo:const pkg=constants name=TDeregDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 clause 6.4.6)
- T_DENREG, how long a Reg_Denied keeps the MS off a site area: <!-- dmrgo:const pkg=constants name=TDenRegDefault --><!-- /dmrgo:const --> by default.
- T_Awake, the time the MS stays awake after a PDU in power save: <!-- dmrgo:const pkg=constants name=TAwakeDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 clause 6.4.7)
- T_Nosig, TV_Inactive and the payload hang time, as for the [Follower](trunk-follower.md#timers).

The MS leaves a TSCC whose bit error rate is above <!-- dmrgo:const pkg=trunk name=DefaultMaxBERPermille --><!-- /dmrgo:const --> parts per thousand by default. The spec leaves this threshold to the manufacturer. (ETSI TS 102 361-4 clause 6.3.2.3)

## Worked example

This example runs an MS against a TSCC with one payload channel. The MS registers, makes a talkgroup call, follows the grant to the payload channel, and ends the call with P_MAINT.

<!-- dmrgo:code file=v2/trunk/example_test.go lines=98-137 -->
<!-- /dmrgo:code -->

The helper carries the bursts between the machines. Each 30 ms it gives the TSCC the inbound bursts that are due, ticks the TSCC and the MS, and gives each MS the outbound bursts of its channel:

<!-- dmrgo:code file=v2/trunk/example_services_test.go lines=74-105 -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=trunk.ExampleMS -->
<!-- /dmrgo:output -->

The last tune request has channel number 0 because the example has no hunt list: the TSCC is then channel CHNULL.

## Errors

Call, CallData and the other requests return an error when the MS is not on a confirmed TSCC, while another request runs, when the TSCC requires a registration that the MS does not hold, when the TSCC stunned the MS, and for a target or options out of range. NewMS returns a configuration error for an address out of range, a slot other than unknown, 1 or 2, and the errors of NewFollower. Match the errors with errors.Is.

<!-- dmrgo:errors pkg=trunk type=MS -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [trunk TSCC](trunk-tscc.md): the site controller that an MS registers on.
- [trunk Follower](trunk-follower.md): follows a site without transmitting.
- [voice Transmitter](voice-transmitter.md): sends the voice of a granted call.
- [voice Receiver](voice-receiver.md): decodes the voice of a call that the MS follows.
- data Sender (docs/machines/data-sender.md): sends the data of a granted packet data call.

Guides:

- Build and decode trunking messages
- Send SMS and confirmed data with retries
