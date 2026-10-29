# bs.Repeater

This page describes the base station state machine of package bs: a repeater, a hotspot or the slot arbiter of a network server.

## What it does

[`bs.Repeater`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater) is the base station of one carrier with two TDMA slots. It takes inbound bursts from RF and from network peers, checks the color code of the RF bursts, and gives each slot to one source at a time. It refuses the other sources, or queues them until the slot is free. It regenerates each burst that it repeats: voice goes through a [`voice.Receiver`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) and a [`voice.Transmitter`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter) per slot in relay mode, and data, CSBK and header bursts are built again from their corrected payload. After a voice call it holds the slot for the call hang time and sends Terminators with the call LC. After confirmed data it sends TD_LC for the data hang time. Pre_CSBKs before data or a CSBK hold the slot for their source. In RF mode it makes one outbound burst per slot time, with the CACH, the TC bit, the AT bit and the Short LC, and it wakes up and hibernates as the activation rules give. With channel authorization it repeats the ChanAuth PDU of one MS and gives the slot only to that MS. Voice bursts from a network source go through a [`voice.Jitter`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter) per slot, which puts them back in order and plays them at a fixed delay. (ETSI TS 102 361-1 clause 5.2 and Annex G, ETSI TS 102 361-2 clause 5.1.1, ETSI TS 102 361-3 clause 5.4.3.3)

The Repeater does no I/O. Each method takes the time `now` on a monotonic clock that the caller picks, and appends outputs to a slice of the caller.

## When to use it

Use a Repeater in [`bs.ModeRF`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ModeRF) when you build a hotspot or a repeater that has a transmitter: an MMDVM modem, an SDR or a microcontroller modulator. Set `Config.CACH` when your modulator does not make the CACH and Idle bursts itself, as with an SDR. Leave it off for an MMDVM modem, whose firmware makes them.

Use it in [`bs.ModeNetwork`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ModeNetwork) for a network server without RF. In this mode the Repeater gives each slot to one peer at a time and forwards the regenerated bursts of that peer to the other peers. It makes no CACH, no hang time bursts and no Idle bursts, and it never hibernates.

Do not use a Repeater only to follow calls. A monitor or a scanner needs a [`voice.Receiver`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) and a [`data.Receiver`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) per slot. If you make the CACH with your own slot logic, use [`bs.CACH`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#CACH) alone.

## Config

[`bs.New`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#New) takes a [`*bs.Config`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Config). A nil config is the zero Config. A duration of 0 selects the default in the field doc, and a negative duration turns that timer off. New returns a `*dmrgo.ConfigError` for a field out of range.

<!-- dmrgo:config pkg=bs type=Config -->
<!-- /dmrgo:config -->

## States and transitions

The Repeater has two levels of state. [`Repeater.State`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.State) returns the state of the carrier: off, keyed with both slots free, or repeating one or both slots (ETSI TS 102 361-1 Figure G.5).

<!-- dmrgo:states pkg=bs type=State -->
<!-- /dmrgo:states -->

[`Repeater.SlotState`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.SlotState) returns the state of one slot (ETSI TS 102 361-1 Figure G.6, ETSI TS 102 361-3 clause 5.4.3.3).

<!-- dmrgo:states pkg=bs type=SlotState -->
<!-- /dmrgo:states -->

These inputs move the slot state:

- A Voice LC Header, or a voice burst of a call that the Repeater joins late, starts a voice call on a free slot. A data header starts data. A Pre_CSBK holds the slot for its source until the data header or the CSBK comes, or until `Config.Lost` passes without a Pre_CSBK. The carrier counts a slot that holds Pre_CSBKs as repeating.
- The Terminator of a voice call, or `Config.Lost` without a burst from the owner, ends the call and starts the call hang time. In the call hang time, a call to the same target from any source takes the slot, and so does a call to any target from the source of the last call. All RF MSs count as one source, so after an RF call any RF MS can take the slot; the politeness rules of the MSs keep the others off. A broadcast call and a call to the all-MS ID get no call hang time (ETSI TS 102 361-2 clause 5.3.2.3.2).
- The last block of confirmed data starts the data hang time, which waits for the response.
- A call that runs longer than `Config.Timeout`, or that the caller refuses with [`Repeater.Reject`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Reject), goes to the state that drops the rest of the call until it ends. A call that ends in this state gets no hang time.
- With `Config.ChannelAuth`, the first MS that starts a transmission in hang time moves the slot to channel authorization. The multi-site mode first collects ChanAuth PDUs from RF and from the network for T_BS_ChanAuthSel and picks the lowest source address (ETSI TS 102 361-2 clause 5.1.1.6).
- In RF mode, a network burst keys the transmitter. From RF, [`bs.ActivationAny`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ActivationAny) keys it on any valid burst with the own color code, and [`bs.ActivationWakeup`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ActivationWakeup) keys it only on a BS_Dwn_Act CSBK with the own color code (ETSI TS 102 361-2 clause 5.1.1.1). T_MSInactiv without inbound activity puts the carrier back to hibernation (ETSI TS 102 361-1 clause 5.2.1.3).

## Inputs and outputs

Every method that takes `dst` appends at most [`bs.MaxOutputs`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#MaxOutputs) outputs. Give it a slice with that capacity, and the method does not allocate.

| Method | Takes | Returns |
|---|---|---|
| [`Inbound`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Inbound) | a raw burst, its [`layer2.Hint`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) and its [`bs.Source`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Source) | `dst` with the outputs |
| [`Decoded`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Decoded) | a burst that the caller decoded, or nil for one that did not decode | `dst` with the outputs |
| [`Lost`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Lost) | a slot and a source whose signal the transport lost | `dst` with the outputs |
| [`Tick`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Tick) | the time now | `dst` with at most one outbound burst and the outputs of the timers |
| [`Deadline`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Deadline) | nothing | the time of the next Tick, and false when no timer runs |
| [`Reject`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Reject) | a slot | an error |
| [`State`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.State), [`SlotState`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.SlotState), [`JitterStats`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.JitterStats) | nothing, or a slot | the state, or the jitter buffer counts of a slot |
| [`Reset`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater.Reset) | nothing | nothing; it ends every call without outputs |

`Inbound` is the usual input. Set `Source.RF` for a burst from your receiver, and set `Source.Peer` to the peer ID for a burst from the network. Put the slot in `Hint.Slot`. A burst on a slot other than slot 1 or slot 2 is dropped without an output; use [`bs.CheckSlot`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#CheckSlot) to find such a burst first. A nil raw burst is a transport frame without a burst: only the Hint applies, and `Hint.SyncLost` ends a call of that source. A network voice burst with `Hint.HasSeq` goes into the jitter buffer of its slot, and a later Tick takes it.

Use `Decoded` when you feed one decoded burst to several machines. The Repeater sends the burst as it encodes it again.

Each [`bs.Output`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Output) has a [`Kind`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#OutputKind):

- `OutputBurst` is a burst to send at `Due` to the places in `To`: [`ToRF`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ToRF), [`ToNetwork`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ToNetwork) or both. With `Config.CACH`, an RF burst also has `HasCACH` and the `CACH` field. `Hint` gives the slot and the burst kind for your transport.
- `OutputCallStart` and `OutputCallEnd` mark a source that takes a slot and the end of its call or data, with `Service`, `Call` and, at the end, `End`.
- `OutputRejected` is a source that the Repeater refused, with a [`Reason`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Reason).
- `OutputKey` and `OutputDekey` turn the transmitter on and off in RF mode.

The time loop is the same as for every dmrgo machine. After each call, read `Deadline` and call `Tick` at that time. In RF mode `Tick` makes one outbound burst per slot time while the carrier is keyed. `Tick` takes at most one burst from the jitter buffers per call, so a caller that runs late calls `Tick` until `Deadline` is in the future. `Config.Lead` makes `Tick` emit each outbound burst early, so that you can fill a transmit FIFO.

To refuse a call for an access rule of your own, call `Reject` on its `OutputCallStart`. The next method that takes `dst` appends `OutputRejected` with `ReasonRejected`, and `OutputCallEnd` follows when the call ends.

## Timers

| Timer | Config field | Default |
|---|---|---|
| Slot time, the period of Tick in RF mode (ETSI TS 102 361-1 clause 4.2.2) | none | <!-- dmrgo:const pkg=bs name=SlotTime --><!-- /dmrgo:const --> |
| Call hang time T_CallHt | `CallHang` | <!-- dmrgo:const pkg=constants name=TCallHang --><!-- /dmrgo:const --> |
| Data hang time T_DataHngtime | `DataHang` | <!-- dmrgo:const pkg=constants name=TDataHangtime --><!-- /dmrgo:const --> |
| T_MSInactiv, the time to hibernation (ETSI TS 102 361-1 clause F.1) | `Inactivity` | <!-- dmrgo:const pkg=bs name=DefaultInactivity --><!-- /dmrgo:const --> |
| Time without a burst from the slot owner that ends its call | `Lost` | <!-- dmrgo:const pkg=voice name=DefaultLost --><!-- /dmrgo:const --> |
| Longest call | `Timeout` | <!-- dmrgo:const pkg=constants name=TTO --><!-- /dmrgo:const --> |
| Longest wait of a queued source | `PendingAge` | <!-- dmrgo:const pkg=bs name=DefaultPendingAge --><!-- /dmrgo:const --> |
| T_BS_ChanAuthRsp after a Voice LC Header | `ChanAuthResponse` | <!-- dmrgo:const pkg=constants name=TBSChanAuthRspVoice --><!-- /dmrgo:const --> |
| T_BS_ChanAuthRsp after a Pre_CSBK, single site | `ChanAuthResponse` | <!-- dmrgo:const pkg=constants name=TBSChanAuthRspCSBKSingle --><!-- /dmrgo:const --> |
| T_BS_ChanAuthRsp after a Pre_CSBK, multi-site | `ChanAuthResponse` | <!-- dmrgo:const pkg=constants name=TBSChanAuthRspCSBKMulti --><!-- /dmrgo:const --> |
| T_BS_ChanAuthSel of multi-site channel authorization | `ChanAuthSelect` | <!-- dmrgo:const pkg=constants name=TBSChanAuthSel --><!-- /dmrgo:const --> |
| Playout delay of the network jitter buffer | `Jitter` | <!-- dmrgo:const pkg=voice name=DefaultJitter --><!-- /dmrgo:const -->, at most <!-- dmrgo:const pkg=voice name=MaxJitter --><!-- /dmrgo:const --> |

A negative `Inactivity` keeps the transmitter on after it wakes up. A negative `Jitter` turns the buffer off, and each network burst is taken as it comes. Data and CSBK bursts from the network have no playout delay. They wait only for a short gap in their sequence to fill, for the Data Header of their block, or for voice of the same source that is still in the jitter buffer; the `Config.Jitter` doc gives these waits.

## Worked example

This example feeds a recorded SMS to slot 2 of a hotspot in RF mode, with a Tick every slot time, and counts the bursts that go out on each slot.

<!-- dmrgo:code file=v2/bs/example_test.go lines=13-50 -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=bs.ExampleRepeater -->
<!-- /dmrgo:output -->

The first valid burst keys the transmitter. The Pre_CSBKs, the data header and the rate 1/2 blocks go out again on slot 2, and both slots get Idle bursts between them. After T_MSInactiv without activity the transmitter dekeys.

[`bs.Config.ChannelAuth`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Config) has its own example, `ExampleConfig_channelAuth`, in the package docs.

## Errors

<!-- dmrgo:errors pkg=bs type=Repeater -->
<!-- /dmrgo:errors -->

Only `New` and `Reject` return errors. The burst methods drop what they cannot use and do not return an error: a burst on a bad slot, an RF burst with another color code, and any call on a zero Repeater. No method panics.

## Related machines and guides

- [voice.Receiver](voice-receiver.md), [voice.Transmitter](voice-transmitter.md) and [voice.Jitter](voice-jitter.md) are the voice machines that the Repeater runs on each slot.
- [data.Receiver](data-receiver.md) follows the data calls that a Repeater repeats, for a gateway or a monitor.
- [csbk.Wakeup](csbk-wakeup.md) is the MS side of the BS_Dwn_Act wakeup that `ActivationWakeup` waits for.
- [csbk.Preambles](csbk-preambles.md) follows Pre_CSBKs on the MS side.
- The guide on how to clean up bursts for a hotspot, and the guide on how to connect to a network, use a Repeater.
