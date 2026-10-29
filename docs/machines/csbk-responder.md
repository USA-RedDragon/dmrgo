# csbk.Responder

This page describes the CSBK answer state machine of package csbk: it answers the CSBK requests that address one radio.

## What it does

[`csbk.Responder`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder) answers the CSBKs on one logical channel that address its radio. It answers UU_V_Req, the presence check of an OACSU individual call, with UU_Ans_Rsp. It answers the Motorola radio check with the radio check answer and the Motorola call alert with the call alert acknowledgement. It answers any other standard CSBK that addresses the radio, and that it does not support, with the Feature Not Supported NACK_Rsp, which an MS must send. It answers one request at a time: a request that comes before the answer to the one before has gone out replaces that answer. The answer goes in a slot of the logical channel of the request, a TDMA frame or more after the start of the request. On the offset channel timing of a BS, the answer goes one slot later, as the first inbound slot leaves no time to decode the request; ETSI gives no answer delay, so this is the choice of dmrgo. (ETSI TS 102 361-1 clauses 5.1.1 and 5.2.1.7, Table 5.1, ETSI TS 102 361-2 clauses 5.1.2 and 5.2.2)

The Motorola radio check and call alert have no public spec. Their layouts and answer times come from captures of an AnyTone radio through a Motorola XPR 8400 repeater.

The Responder does no I/O. Each method takes the time `now` on a monotonic clock that the caller picks, and appends outputs to a slice of the caller.

## When to use it

Use a Responder in a radio, or in a hotspot or gateway that acts as a radio with its own address, so that other radios get an answer to a radio check, a call alert or an OACSU call. An MS must answer an unsupported CSBK with a Feature Not Supported NACK_Rsp, and the Responder sends it unless you set `NoFNS`.

Use a [csbk.Requester](csbk-requester.md) to send the requests. Do not use a Responder for group CSBKs: it drops a CSBK whose target is a talkgroup.

## Config

[`csbk.NewResponder`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewResponder) takes a [`*csbk.ResponderConfig`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#ResponderConfig). It has no nil default: a nil config, and an `Address` of 0, return an error. `Slot` must be slot 1 or slot 2.

<!-- dmrgo:config pkg=csbk type=ResponderConfig -->
<!-- /dmrgo:config -->

`OACSU` sets the answer to UU_V_Req: Proceed, Deny or the Feature Not Supported NACK_Rsp, as [`csbk.OACSUAnswer`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#OACSUAnswer) gives. `NoRadioCheck` and `NoCallAlert` turn off the Motorola answers, as a radio does when its settings turn them off. `Offset` selects the offset channel timing of a BS.

## States and transitions

<!-- dmrgo:states pkg=csbk type=ResponderState -->
<!-- /dmrgo:states -->

These inputs move the state:

- A CSBK that addresses the radio, and that the Responder answers, queues the answer. On an idle channel, or with impolite access, the answer is ready to send at the next slot of the answer grid. On a busy channel with polite access, it waits for channel access.
- In channel access, the answers to the radio check, the call alert and UU_V_Req are time critical: `Channel` with an idle channel lets the answer go, and when T_IdleSrch passes on a busy channel the Responder drops the answer and appends `OutputDone` with `ResultDenied`. A NACK_Rsp is not time critical: it waits a random holdoff, and then another while the channel stays busy (ETSI TS 102 361-2 clause 5.1.2.2).
- The Tick at the `Due` time of the answer emits it and returns to idle.
- `Reset` drops the queued answer without an output.

A request whose answer is turned off by `NoFNS` still gives an `OutputRequest`, but it queues no answer and leaves a queued answer as it is.

## Inputs and outputs

Every method that takes `dst` appends at most [`csbk.MaxOutputs`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#MaxOutputs) outputs, and does not allocate when `dst` has that room.

| Method | Takes | Returns |
|---|---|---|
| [`Burst`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Burst) | a raw burst and its [`layer2.Hint`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) | `dst` with an `OutputRequest` for a request it answers |
| [`Decoded`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Decoded) | a burst that the caller decoded, or nil | the same as `Burst` |
| [`Channel`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Channel) | whether the channel is busy | nothing |
| [`Tick`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Tick) | the time now | `dst` with the answer burst, or `OutputDone` with `ResultDenied` |
| [`Deadline`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Deadline) | nothing | the time of the next Tick, and false when no answer waits |
| [`Lost`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Lost) | the time now | `dst`; the channel counts as idle |
| [`State`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.State), [`Reset`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder.Reset) | nothing | the state, or nothing |

Feed every burst of the logical channel to `Burst`, or to `Decoded` when you decode the burst once for several machines. Pass `now` as the start of the received burst, because the answer grid starts there. The Responder takes a CSBK whose target is its `Address` and whose source is another radio, and drops a CSBK whose CRC fails. It does not check the color code of the bursts it receives; `ColorCode` sets only the color code of the answers.

Each [`csbk.Output`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Output) is one of:

- `OutputRequest`: a request for the radio, with its `Service`, its `Call` and the request in `CSBK`. For `ServiceCallAlert`, alert the user of the radio.
- `OutputBurst`: the answer to send at `Due`, with a `Hint` of a CSBK on the configured slot. Its `Call` goes from the radio back to the source of the request.
- `OutputDone` with `ResultDenied`: a time critical answer that the channel denied.

Call `Channel` each time the activity on the channel changes. Until the first report the channel counts as idle.

## Timers

| Timer | Config field | Default |
|---|---|---|
| Answer grid: the answer goes this long after the start of the request, or a whole number of these later | none | <!-- dmrgo:const pkg=csbk name=BurstInterval --><!-- /dmrgo:const --> |
| T_IdleSrch of a time critical answer (ETSI TS 102 361-1 clause F.1) | `IdleSearch` | <!-- dmrgo:const pkg=csbk name=DefaultIdleSearch --><!-- /dmrgo:const --> |
| Upper end of the random holdoff T_Holdoff of a NACK_Rsp | `HoldoffMax` | <!-- dmrgo:const pkg=csbk name=DefaultHoldoffMax --><!-- /dmrgo:const --> |

With `Offset`, the answer grid starts half a `BurstInterval` later. A polite answer that gets channel access between two slots of the grid goes in the next slot of the grid. A negative `IdleSearch` denies a time critical answer on a busy channel at once. A negative `HoldoffMax` turns the holdoff off, and the NACK_Rsp waits for an idle channel. `Lead` makes `Tick` emit the answer early, for an RF FIFO; when the lead is more than one `BurstInterval`, the answer moves to a later slot of the grid.

## Worked example

This example sends a Motorola radio check from a Requester to two Responders: one on the aligned channel timing of a BS, and one with `Offset`. It prints when each answer goes on air.

<!-- dmrgo:code file=v2/csbk/example_test.go lines=127-152 -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=csbk.ExampleResponder -->
<!-- /dmrgo:output -->

## Errors

<!-- dmrgo:errors pkg=csbk type=Responder -->
<!-- /dmrgo:errors -->

Only `NewResponder` returns an error, a [`*dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError). Match it with `errors.As`; its `Field` names the field out of range. The package example `ExampleNewResponder_configError` shows a config without an address. An answer that the channel denies gives an `OutputDone` with `ResultDenied`, and no error.

## Related machines and guides

- [csbk.Requester](csbk-requester.md) sends the requests that a Responder answers.
- [csbk.Preambles](csbk-preambles.md) follows the Pre_CSBKs that come before a request.
- The guide on radio check, call alert and other CSBK services uses a Responder.
