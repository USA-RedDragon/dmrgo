# csbk.Requester

This page describes the CSBK request state machine of package csbk: it sends one CSBK request, waits for the answer and sends it again when no answer comes.

## What it does

[`csbk.Requester`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester) sends CSBK requests on one logical channel, one request at a time. It can send Pre_CSBKs before the request, for radios that scan or sleep. Each try uses the channel access of the request: an impolite try goes at once, and a polite try waits for an idle channel. A time critical polite try waits up to T_IdleSrch and then fails. A polite try that is not time critical waits a random holdoff T_Holdoff, and then another one while the channel stays busy. After each try the Requester waits T_AckWait for the answer and tries again up to N_CSBKRetry times. It reports the answer, a NACK_Rsp for the request, or why no answer came. It has built-in requests for the OACSU presence check (UU_V_Req, answered by UU_Ans_Rsp) and for the Motorola radio check and call alert (FID 0x10). Any other CSBK goes out with a [`csbk.Match`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Match) that gives the answer. (ETSI TS 102 361-1 clause 5.2.2, ETSI TS 102 361-2 clauses 5.1.2 and 5.2.2)

The Motorola radio check and call alert have no public spec. Their layouts and answer times come from captures of an AnyTone radio through a Motorola XPR 8400 repeater.

The Requester does no I/O. Each method takes the time `now` on a monotonic clock that the caller picks, and appends outputs to a slice of the caller.

## When to use it

Use a Requester when your radio, hotspot or gateway starts a CSBK service and needs its answer: a radio check of another radio, a call alert, the presence check before an OACSU individual call, or a CSBK of your own design with a known answer.

Use a [csbk.Responder](csbk-responder.md) for the other side, the radio that answers. Use a [csbk.Wakeup](csbk-wakeup.md) to wake a BS outbound, which has no CSBK answer.

## Config

[`csbk.NewRequester`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewRequester) takes a [`*csbk.RequesterConfig`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#RequesterConfig). A nil config is the zero config. `Slot` must be slot 1 or slot 2, so a nil config returns an error. `Source` selects the SYNC: BS sourced for a repeater outbound, MS sourced for a radio.

<!-- dmrgo:config pkg=csbk type=RequesterConfig -->
<!-- /dmrgo:config -->

Each request also takes a [`*csbk.RequestOptions`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#RequestOptions). A nil value is polite to own color code, not time critical, and without Pre_CSBKs.

<!-- dmrgo:config pkg=csbk type=RequestOptions -->
<!-- /dmrgo:config -->

## States and transitions

<!-- dmrgo:states pkg=csbk type=RequesterState -->
<!-- /dmrgo:states -->

These inputs move the state:

- A request method starts a request. An impolite request, or a polite one on an idle channel, goes to sending at once. A polite request on a busy channel waits for channel access.
- In channel access, `Channel` with an idle channel starts the try of a time critical request, or of a request without a holdoff. A request that is not time critical starts its try when its holdoff ends on an idle channel, or waits another holdoff. A time critical request that waits T_IdleSrch ends with `ResultDenied` (ETSI TS 102 361-2 Figure 5.22).
- In sending, each Tick emits one burst: the Pre_CSBKs, then the request. After the request the Requester waits for the answer.
- While it waits, an answer ends the request. When T_AckWait passes, the Requester starts the next try with `RequestOptions.RetryPreambles` Pre_CSBKs, through channel access again, or ends with `ResultNoAnswer` after the last try.
- `Cancel` ends a running request with `ResultCanceled`. `Reset` ends it without an output.

An answer counts only after the first try went out. An answer that comes while a retry goes out also ends the request.

## Inputs and outputs

Every method that takes `dst` appends at most [`csbk.MaxOutputs`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#MaxOutputs) outputs, and does not allocate when `dst` has that room.

| Method | Takes | Returns |
|---|---|---|
| [`RequestOACSU`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestOACSU) | target, source, the service options and the request options | an error |
| [`RequestRadioCheck`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestRadioCheck) | target, source and the request options | an error |
| [`RequestCallAlert`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestCallAlert) | target, source and the request options | an error |
| [`Request`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Request) | a [`*pdu.CSBK`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK), a `Match` and the request options | an error |
| [`Channel`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Channel) | whether the channel is busy | nothing |
| [`Burst`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Burst) | a raw burst and its [`layer2.Hint`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) | `dst` with the outputs |
| [`Decoded`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Decoded) | a burst that the caller decoded, or nil | `dst` with the outputs |
| [`Tick`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Tick) | the time now | `dst` with at most one burst, and the end of a request |
| [`Deadline`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Deadline) | nothing | the time of the next Tick, and false when no timer runs |
| [`Lost`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Lost) | the time now | `dst`; the channel counts as idle |
| [`Cancel`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Cancel) | the time now | `dst` with `ResultCanceled` when a request runs |
| [`State`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.State), [`Reset`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Reset) | nothing | the state, or nothing |

A request method only starts the request. The first burst comes from the next `Tick`, so call `Deadline` after the request and `Tick` at that time.

`RequestOACSU` is always time critical and must be polite; it returns an error for impolite access. When the answer is Proceed, the individual call that follows may use impolite access (ETSI TS 102 361-2 clause 5.2.2.3).

`Request` sends your CSBK with the Last Block bit and its CRC set, and does not change your value. It takes the target and source from the CSBK: from the standard addresses for the standard FID, from the payload for the Motorola CSBKs, and from octets 4 to 6 and 7 to 9 for any other FID. Make the `Match` with [`csbk.NewMatch`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewMatch): a FID and up to four answer opcodes. A NACK_Rsp from the target that names the opcode of the request also ends a custom request.

Call `Channel` each time the activity on the logical channel changes, from the CACH AT bit, the carrier or a repeater. Until the first report the channel counts as idle, as it is for a network transport.

Feed every burst that you receive on the channel to `Burst`, or to `Decoded` when you decode the burst once for several machines. The Requester takes only an answer addressed from the target of the request to its source, and drops a CSBK whose CRC fails. A `ColorCode` other than 0 also drops received CSBKs with another color code, in `Burst` and in `Decoded`; 0 accepts every color code.

Each [`csbk.Output`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Output) is one of:

- `OutputBurst`: a burst to send at `Due`, with a `Hint` of a CSBK on the configured slot, and the `Call` of the request.
- `OutputDone`: the end of the request, with its `Service`, its [`Result`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Result), the `Call`, and the answer in `CSBK` when one came. For `ResultNACK`, `Reason` has the reason code of the NACK_Rsp.

## Timers

| Timer | Config field | Default |
|---|---|---|
| T_AckWait, the wait for the answer after each try (ETSI TS 102 361-2 clause A.1) | `AckWait` | <!-- dmrgo:const pkg=constants name=TAckWait --><!-- /dmrgo:const --> |
| T_AckWait of the Motorola radio check and call alert | `AckWait` | <!-- dmrgo:const pkg=csbk name=DefaultMotorolaAckWait --><!-- /dmrgo:const --> |
| N_CSBKRetry, the tries after the first | `Retries` | <!-- dmrgo:const pkg=constants name=NCSBKRetry --><!-- /dmrgo:const -->, at most <!-- dmrgo:const pkg=csbk name=MaxRetries --><!-- /dmrgo:const --> |
| T_IdleSrch of a time critical try (ETSI TS 102 361-1 clause F.1) | `IdleSearch` | <!-- dmrgo:const pkg=csbk name=DefaultIdleSearch --><!-- /dmrgo:const --> |
| Upper end of the random holdoff T_Holdoff (ETSI TS 102 361-1 clause 5.2.2.3) | `HoldoffMax` | <!-- dmrgo:const pkg=csbk name=DefaultHoldoffMax --><!-- /dmrgo:const --> |
| Time between two bursts of a try | none | <!-- dmrgo:const pkg=csbk name=BurstInterval --><!-- /dmrgo:const --> |

A negative `AckWait` waits until an answer or `Cancel`. A negative `Retries` sends no retry. A negative `IdleSearch` does not wait for an idle channel. A negative `HoldoffMax` turns the holdoff off, and the try waits for an idle channel. Pre_CSBKs per try are at most <!-- dmrgo:const pkg=csbk name=MaxPreambles --><!-- /dmrgo:const -->.

T_AckWait starts at the `Due` time of the request burst. `Config.Lead` makes `Tick` emit each burst early, for an RF FIFO, and the timers still keep to the `Due` times. `Config.Seed` seeds the holdoff, so the same inputs give the same outputs. Give each Requester on a channel its own seed.

## Worked example

This example sends a Motorola radio check with 5 Pre_CSBKs and no retries. No radio answers, so the request ends when T_AckWait passes after the request burst.

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=ExampleRequester_RequestRadioCheck -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=csbk.ExampleRequester_RequestRadioCheck -->
<!-- /dmrgo:output -->

The package example sends an OACSU presence check from a Requester to a [csbk.Responder](csbk-responder.md) and reads the answer back:

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=Example -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=csbk.Example -->
<!-- /dmrgo:output -->

## Errors

<!-- dmrgo:errors pkg=csbk type=Requester -->
<!-- /dmrgo:errors -->

Match [`csbk.ErrBusy`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#ErrBusy) and [`csbk.ErrNoAddresses`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#ErrNoAddresses) with `errors.Is`, and a [`*dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) with `errors.As`; its `Field` names the field out of range. A request that fails returns its error at once and starts nothing. A request that starts and then fails ends with an `OutputDone` and gives no error.

## Related machines and guides

- [csbk.Responder](csbk-responder.md) answers the requests of a Requester.
- [csbk.Preambles](csbk-preambles.md) follows the Pre_CSBKs that a Requester sends before its request.
- [csbk.Wakeup](csbk-wakeup.md) wakes a BS before a request on a repeater that hibernates.
- [data.Sender](data-sender.md) uses the same channel access rules for data.
- The guide on radio check, call alert and other CSBK services uses a Requester.
