# trunk TSCC

This page covers the trunk TSCC, the state machine that runs a single DMR Tier III trunked site: its control channel and its payload channels.

## What it does

A [TSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC) is a dedicated Tier III TSCC with its payload channels, for one site. It sends C_ALOHA and the C_SYS_Parms Short LC. It withdraws the inbound slot of the answer to a PDU that solicits one, with the AT bit of the CACH after that PDU. It registers MSs, sets up talkgroup voice calls and individual voice calls with OACSU, sends grants and late entry grants, relays the inbound bursts of each call on its payload slot, and clears calls with P_CLEAR. It can authenticate the MSs that register. It also carries UDT short data, UDT and USBD polling, call diversion and DGNA, sets up full-duplex and Ambient Listening Service calls, and runs power save, mass registration, stun, revive and kill, status calls, transmit interrupts, IP connection advice and supplementary_user data. It does not support half-duplex data calls or FOACSU. The TSCC keeps a 30 ms slot clock from its first Tick and returns one burst with its CACH per carrier for each slot. It does no I/O. (ETSI TS 102 361-4 clauses 6.2, 6.4.4 to 6.4.15, 6.6, 6.6.2.6 and 6.6.4 to 6.6.11)

## When to use it

Use a TSCC to run a small Tier III site from software: a test site for radios, a simulator for MS and Follower code, or a site on SDR or repeater hardware that you drive. One TSCC runs one control channel and up to <!-- dmrgo:const pkg=trunk name=MaxPayloadChannels -->`16`<!-- /dmrgo:const --> payload channels, each with two slots for calls. It is not a multi-site controller.

## Config

Pass a [TSCCConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCConfig) to [NewTSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#NewTSCC). The system code and at least one payload channel are required, so a nil config returns an error. Set Reg to require registration. Turn on each optional service that the site offers: authentication, transmit interrupt, power save, IP and supplementary data. Set Lead when the bursts must reach an RF FIFO ahead of air time.

<!-- dmrgo:config pkg=trunk type=TSCCConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `SysCode` | `pdu.SysCode` | zero value | SysCode is the C_SYScode of the site. |
| `ColorCode` | `enums.ColorCode` | zero value | ColorCode is the color code of the TSCC and its payload channels. |
| `Channel` | `uint16` | zero value | Channel is the channel number of the TSCC, for the outputs of its carrier: 1 to 0xFFE. |
| `Payload` | `[16]uint16` | zero value | Payload holds the first PayloadLen channel numbers of the payload channels, each 1 to 0xFFE, different from each other and from Channel. Each has two slots for calls. |
| `PayloadLen` | `int` | zero value | PayloadLen is the number of entries of Payload in use, 1 or more. |
| `Reg` | `bool` | zero value | Reg requests MSs to register, and refuses the services of an MS that is not registered. |
| `MaxRegistrations` | `int` | `64` | MaxRegistrations is the size of the registration table, at most 65536. 0 means 64. |
| `NRandWait` | `enums.NRandWait` | `constants.NDefaultNW` (5) | NRandWait is the NRand_Wait of C_ALOHA. 0 means the code of constants.NDefaultNW. |
| `Backoff` | `enums.Backoff` | `3` | Backoff is the backoff number of C_ALOHA. 0 means 3. |
| `GrantRepeats` | `int` | `2` | GrantRepeats is the number of times the TSCC sends a grant. 0 means 2; a negative value is an error. |
| `ClearRepeats` | `int` | `3` | ClearRepeats is the number of times the TSCC sends P_CLEAR. 0 means 3; a negative value is an error. |
| `LateEntry` | `time.Duration` | `time.Second` (1s) | LateEntry is the time between the late entry grants of a call. 0 means 1 s, and a negative value sends none. |
| `Hangtime` | `time.Duration` | `constants.TVHangtimeDefault` (4s) | Hangtime is TV_Hangtime, the time after the end of a transmission in which the call keeps its payload slot. 0 or a negative value means constants.TVHangtimeDefault. |
| `Inactive` | `time.Duration` | `constants.TVInactiveDefault` (5s) | Inactive is TV_Inactive: the TSCC clears a call that has no inbound burst for this long outside the hang time. 0 or a negative value means constants.TVInactiveDefault. |
| `AnswerTimeout` | `time.Duration` | `constants.TPTimerDefault` (10s) | AnswerTimeout is how long the TSCC waits for the answer of a called MS to its C_AHOY, and for the answer to an authentication challenge. 0 or a negative value means constants.TPTimerDefault. |
| `Authenticate` | `bool` | zero value | Authenticate challenges each MS that registers with a C_AHOY, and registers it only when its answer matches the response that the consumer passes to ExpectAuth after TSCCAuthChallenge. A wrong answer gets C_NACKD Reg_Denied, and no answer within AnswerTimeout gets C_NACKD Reg_Refused. |
| `Seed` | `uint64` | zero value | Seed seeds the random choice of the authentication challenges. |
| `Auth` | `Authenticator` | zero value | Auth, when set, answers the authentication challenge of an MS before a stun, revive or kill. The TSCC keeps Auth and does not copy what it points to, such as a *trunkauth.Keys and its Entries. Do not change it while the TSCC uses it. (ETSI TS 102 361-4 clauses 6.4.9.2 and 6.4.10) |
| `TransmitInterrupt` | `bool` | zero value | TransmitInterrupt accepts the transmit interrupt requests of MSs, status calls with StatusInterrupt and StatusTerminate. (ETSI TS 102 361-4 clause 6.4.14.3) |
| `MaxCall` | `time.Duration` | zero value | MaxCall is the longest time of a call. At its end the TSCC stops the transmission with the reverse channel and clears the call. 0 or a negative value means no limit. (ETSI TS 102 361-4 clause 6.4.14.3.2) |
| `PowerSave` | `bool` | zero value | PowerSave gives the MSs that request it a power save frame, and holds the C_AHOY to an MS that sleeps until its frame. (ETSI TS 102 361-4 clause 6.4.7) |
| `Awake` | `time.Duration` | `constants.TAwakeDefault` (1s) | Awake is T_Awake, as in MSConfig. 0 or a negative value means constants.TAwakeDefault. |
| `IP` | `bool` | zero value | IP offers the IP bearer service: the TSCC takes the IP connection advice of MSs. Without it the TSCC refuses the advice with IP_Connection_failed. (ETSI TS 102 361-4 clause 6.4.11) |
| `Supplementary` | `bool` | zero value | Supplementary takes the supplementary_user data of call requests and passes it to the called party before the call. Without it the TSCC sets up the call without the data. (ETSI TS 102 361-4 clause 6.4.13) |
| `Lead` | `time.Duration` | not machine-readable; see the description | Lead makes Tick emit the bursts of a slot this long before its Due time, so that an RF FIFO can be filled ahead. 0 means at Due; it must be at least 0 and less than MaxLead. The timers of the TSCC keep to the Due times. |
<!-- /dmrgo:config -->

## States and transitions

The TSCC has no State type: the site always runs. Each payload slot holds at most one call, and the TSCC moves each call through these steps:

- Set-up. An MS requests a call with C_RAND. For an individual call the TSCC first sends C_AHOY to the called MS and waits for its answer (OACSU). A call that does not set up in time is refused.
- Grant. The TSCC sends the grant on the control channel and late entry grants while the call runs. The call takes a free payload slot; when no slot is free, the TSCC refuses the request with Sys_Busy_Refused.
- Call. The TSCC relays the inbound bursts of the call on its payload slot. Outside the hang time, a call with no inbound burst for TV_Inactive is cleared.
- Hang time. After the end of a transmission the call keeps its slot for TV_Hangtime, so that a reply can start without a new request.
- Clear. P_MAINT from the MS, the end of the hang time, TV_Inactive, or the maximum call time ends the call. At the maximum call time the TSCC first stops the transmission with the reverse channel. The TSCC sends P_CLEAR on the slot and reports the end with its reason.

Registration and the services run on the control channel beside the calls. [Registered](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Registered) reports whether an MS is in the registration table. (ETSI TS 102 361-4 clauses 6.4.4, 6.4.8 and 6.6)

## Inputs and outputs

Inputs:

- [Inbound](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Inbound) takes a burst received on the inbound channel of a carrier, with Hint.Slot set. On the TSCC it answers C_RAND, C_ACKU and C_NACKU. On a payload slot it keeps the burst for the relay and ends the call on P_MAINT.
- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Tick) runs the timers and returns the outbound bursts of the slot that is due. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Deadline) returns. A Tick that comes a slot or more late sends only the last slot, and [Skipped](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Skipped) counts the others.
- [ExpectAuth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.ExpectAuth) gives the response that the TSCC accepts to an authentication challenge.
- The services: [SendStatus](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.SendStatus), [Interrupt](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Interrupt), [MassRegister](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.MassRegister), [Stun](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Stun), [Kill](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Kill), [SendShortData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.SendShortData), [AssignGroups](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.AssignGroups), [PollShortData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.PollShortData) and [PollUSBD](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.PollUSBD).

Outputs are [TSCCOutput](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCOutput) values with a [TSCCOutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCOutputKind). A burst output has the carrier channel number, the slot, the CACH in transmit order, the burst, its Due time and a Hint. The other kinds report registrations, the start and end of calls, refusals, authentication challenges, and the results of the services. [UDT](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.UDT), [USBD](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.USBD) and [Diverted](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Diverted) return the data of the last such output. Give each method a slice with capacity <!-- dmrgo:const pkg=trunk name=MaxTSCCOutputs -->`43`<!-- /dmrgo:const -->.

## Timers

- Slot clock: one slot each 30 ms from the first Tick, starting with TS1.
- TV_Hangtime: the time after the end of a transmission in which the call keeps its slot. The default is <!-- dmrgo:const pkg=constants name=TVHangtimeDefault -->`4s`<!-- /dmrgo:const -->. (ETSI TS 102 361-4 Table A.1)
- TV_Inactive: the time without an inbound burst, outside the hang time, after which the TSCC clears a call. The default is <!-- dmrgo:const pkg=constants name=TVInactiveDefault -->`5s`<!-- /dmrgo:const -->. (ETSI TS 102 361-4 Table A.1)
- Answer timeout: the wait for the answer of a called MS to C_AHOY, and for the answer to an authentication challenge. The default is <!-- dmrgo:const pkg=constants name=TPTimerDefault -->`10s`<!-- /dmrgo:const -->.
- T_Awake in power save: <!-- dmrgo:const pkg=constants name=TAwakeDefault -->`1s`<!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 clause 6.4.7)
- Late entry grants and the maximum call time: TSCCConfig.LateEntry and TSCCConfig.MaxCall. (ETSI TS 102 361-4 clause 6.4.14.3.2)
- Lead: less than <!-- dmrgo:const pkg=trunk name=MaxLead -->`1s`<!-- /dmrgo:const -->. The timers keep to the Due times.

## Worked example

This example runs a TSCC with one payload channel and one MS. The TSCC reports the registration of the MS, the start of its talkgroup call on the payload channel, and the end of the call when the MS sends P_MAINT. The helper newServiceSite and its run method, in example_services_test.go, carry the bursts on the 30 ms slot clock; the [MS page](trunk-ms.md#worked-example) shows them.

<!-- dmrgo:code file=v2/trunk/example_test.go symbol=ExampleTSCC -->
```go
func ExampleTSCC() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	s.onTSCC = func(o *trunk.TSCCOutput) {
		switch o.Kind {
		case trunk.TSCCRegistered:
			fmt.Println(o.Kind, o.Address)
		case trunk.TSCCCallStart:
			fmt.Println(o.Kind, "talkgroup", o.Grant.Target, "from", o.Grant.Source, "on channel", o.Grant.Channel, o.Grant.Slot)
		case trunk.TSCCCallEnd:
			fmt.Println(o.Kind, o.Reason)
		default:
		}
	}
	s.run(3 * time.Second)
	_ = s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{})
	s.run(time.Second)
	s.take(0, s.ms[0].EndCall(s.now, s.mout[:0]))
	s.run(time.Second)
	fmt.Println("registered", s.tscc.Registered(3120001))
}
```
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=trunk.ExampleTSCC -->
```text
Registered 3120001
CallStart talkgroup 9 from 3120001 on channel 10 TS1
CallEnd Maintenance
registered true
```
<!-- /dmrgo:output -->

## Errors

Tick and Inbound return no errors. The service methods return an error for an address or data out of range, and Interrupt returns an error for a slot without a call. NewTSCC returns a configuration error for a field out of range. Match the errors with errors.Is.

<!-- dmrgo:errors pkg=trunk type=TSCC -->
| Error | Returned by | Meaning |
|---|---|---|
| [`dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) | `TSCC.Interrupt`, `TSCC.MassRegister`, `TSCC.PollShortData`, `TSCC.PollUSBD`, `TSCC.SendShortData`, `TSCC.SendStatus`, `TSCC.Stun` | ConfigError is the error of a constructor, and of a machine method that takes a config, option or argument field, for a field with a value out of range. |
| [`trunk.ErrBusy`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ErrBusy) | `TSCC.PollShortData`, `TSCC.PollUSBD`, `TSCC.SendShortData`, `TSCC.SendStatus`, `TSCC.Stun` | ErrBusy is returned by MS.Call while a registration or call request runs. |
| [`trunk.ErrNoCall`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ErrNoCall) | `TSCC.Interrupt` | ErrNoCall is returned by MS.Interrupt when the MS is not on the payload channel of a voice call, and by TSCC.Interrupt for a slot without a call. |
| [`trunk.ErrUDTFormat`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ErrUDTFormat) | `TSCC.AssignGroups` | ErrUDTFormat is returned for appended data in another UDT Format than the method reads or writes, or in a Format over 15. |
| [`trunk.ErrUDTLength`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ErrUDTLength) | `TSCC.PollShortData`, `TSCC.SendShortData` | ErrUDTLength is returned for data that does not fit in 1 to 4 appended blocks, for blocks whose size does not match the format, and for a PadNibble over 31. |
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [trunk MS](trunk-ms.md): the radios that register on the TSCC and make calls.
- [trunk Follower](trunk-follower.md): follows the site without transmitting.
- bs Repeater (docs/machines/bs-repeater.md): a conventional repeater, for a site that does not trunk.

Guides:

- Build and decode trunking messages
