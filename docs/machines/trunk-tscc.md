# trunk TSCC

This page covers the trunk TSCC, the state machine that runs a single DMR Tier III trunked site: its control channel and its payload channels.

## What it does

A [TSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC) is a dedicated Tier III TSCC with its payload channels, for one site. It sends C_ALOHA and the C_SYS_Parms Short LC. It withdraws the inbound slot of the answer to a PDU that solicits one, with the AT bit of the CACH after that PDU. It registers MSs, sets up talkgroup voice calls and individual voice calls with OACSU, sends grants and late entry grants, relays the inbound bursts of each call on its payload slot, and clears calls with P_CLEAR. It can authenticate the MSs that register. It also carries UDT short data, UDT and USBD polling, call diversion and DGNA, sets up full-duplex and Ambient Listening Service calls, and runs power save, mass registration, stun, revive and kill, status calls, transmit interrupts, IP connection advice and supplementary_user data. It does not support half-duplex data calls or FOACSU. The TSCC keeps a 30 ms slot clock from its first Tick and returns one burst with its CACH per carrier for each slot. It does no I/O. (ETSI TS 102 361-4 clauses 6.2, 6.4.4 to 6.4.15, 6.6, 6.6.2.6 and 6.6.4 to 6.6.11)

## When to use it

Use a TSCC to run a small Tier III site from software: a test site for radios, a simulator for MS and Follower code, or a site on SDR or repeater hardware that you drive. One TSCC runs one control channel and up to <!-- dmrgo:const pkg=trunk name=MaxPayloadChannels --><!-- /dmrgo:const --> payload channels, each with two slots for calls. It is not a multi-site controller.

## Config

Pass a [TSCCConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCConfig) to [NewTSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#NewTSCC). The system code and at least one payload channel are required, so a nil config returns an error. Set Reg to require registration. Turn on each optional service that the site offers: authentication, transmit interrupt, power save, IP and supplementary data. Set Lead when the bursts must reach an RF FIFO ahead of air time.

<!-- dmrgo:config pkg=trunk type=TSCCConfig -->
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

Outputs are [TSCCOutput](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCOutput) values with a [TSCCOutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCCOutputKind). A burst output has the carrier channel number, the slot, the CACH in transmit order, the burst, its Due time and a Hint. The other kinds report registrations, the start and end of calls, refusals, authentication challenges, and the results of the services. [UDT](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.UDT), [USBD](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.USBD) and [Diverted](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC.Diverted) return the data of the last such output. Give each method a slice with capacity <!-- dmrgo:const pkg=trunk name=MaxTSCCOutputs --><!-- /dmrgo:const -->.

## Timers

- Slot clock: one slot each 30 ms from the first Tick, starting with TS1.
- TV_Hangtime: the time after the end of a transmission in which the call keeps its slot. The default is <!-- dmrgo:const pkg=constants name=TVHangtimeDefault --><!-- /dmrgo:const -->. (ETSI TS 102 361-4 Table A.1)
- TV_Inactive: the time without an inbound burst, outside the hang time, after which the TSCC clears a call. The default is <!-- dmrgo:const pkg=constants name=TVInactiveDefault --><!-- /dmrgo:const -->. (ETSI TS 102 361-4 Table A.1)
- Answer timeout: the wait for the answer of a called MS to C_AHOY, and for the answer to an authentication challenge. The default is <!-- dmrgo:const pkg=constants name=TPTimerDefault --><!-- /dmrgo:const -->.
- T_Awake in power save: <!-- dmrgo:const pkg=constants name=TAwakeDefault --><!-- /dmrgo:const --> by default. (ETSI TS 102 361-4 clause 6.4.7)
- Late entry grants and the maximum call time: TSCCConfig.LateEntry and TSCCConfig.MaxCall. (ETSI TS 102 361-4 clause 6.4.14.3.2)
- Lead: less than <!-- dmrgo:const pkg=trunk name=MaxLead --><!-- /dmrgo:const -->. The timers keep to the Due times.

## Worked example

This example runs a TSCC with one payload channel and one MS. The TSCC reports the registration of the MS, the start of its talkgroup call on the payload channel, and the end of the call when the MS sends P_MAINT. The helper newServiceSite and its run method, in example_services_test.go, carry the bursts on the 30 ms slot clock; the [MS page](trunk-ms.md#worked-example) shows them.

<!-- dmrgo:code file=v2/trunk/example_test.go lines=139-165 -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=trunk.ExampleTSCC -->
<!-- /dmrgo:output -->

## Errors

Tick and Inbound return no errors. The service methods return an error for an address or data out of range, and Interrupt returns an error for a slot without a call. NewTSCC returns a configuration error for a field out of range. Match the errors with errors.Is.

<!-- dmrgo:errors pkg=trunk type=TSCC -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [trunk MS](trunk-ms.md): the radios that register on the TSCC and make calls.
- [trunk Follower](trunk-follower.md): follows the site without transmitting.
- bs Repeater (docs/machines/bs-repeater.md): a conventional repeater, for a site that does not trunk.

Guides:

- Build and decode trunking messages
