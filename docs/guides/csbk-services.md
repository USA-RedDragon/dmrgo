# Radio check, call alert and other CSBK services

This guide shows how to send a radio check, a call alert and other CSBK requests, how a radio answers them, and how to follow preambles, wake a repeater and take part in direct mode timing.

## Goal

Send a Motorola radio check or call alert to a radio and get its answer, answer the requests that address your own radio, and use the other CSBK services of package csbk: the OACSU presence check, Pre_CSBKs, the repeater wakeup and the direct mode timing.

## What you need

- Go and a checkout of dmrgo. The examples run offline.
- The package [csbk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk), and [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) for the bursts.
- On a live channel, a source of bursts and a way to send them: a modem, a network transport or an SDR. The other guides connect each one.

Run the examples from the top of the repository:

```sh
go test -C v2 -run Example -v ./csbk
```

## Steps

### 1. Make a Requester and a Responder

A [Requester](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester) sends a request and waits for its answer. A [Responder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Responder) answers the requests that address one radio. Make each with its constructor, [NewRequester](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewRequester) and [NewResponder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewResponder). The config sets the slot, the color code and the SYNC source: `layer2.SyncSourceMS` for a radio and `layer2.SyncSourceBS` for a repeater outbound.

Both machines do no I/O. Each method takes the time now and appends outputs to a slice of the caller. A slice with capacity `csbk.MaxOutputs` never grows.

### 2. Send a radio check and a call alert

[Requester.RequestRadioCheck](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestRadioCheck) starts a radio check, and [Requester.RequestCallAlert](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestCallAlert) a call alert, which makes the target alert its user. The next `Tick` returns the request burst. Send it at its `Due` time.

Give each received burst to `Burst` of each machine. The Responder returns an `OutputRequest` for a request to its radio. Its `Deadline` returns the time of the answer, and `Tick` at that time returns the answer burst. The Requester returns an `OutputDone` with the `Result` when the answer comes.

In this example the two machines pass the bursts to each other in memory:

<!-- dmrgo:code file=v2/csbk/example_test.go lines=132-159 -->
<!-- /dmrgo:code -->

### 3. Handle a radio that does not answer

The Requester sends the request again after each wait for the answer, up to the number of retries of its config, and then returns `OutputDone` with `ResultNoAnswer`. Call `Deadline` and `Tick` in a loop until `Deadline` returns false. [RequestOptions](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#RequestOptions) adds Pre_CSBKs before the first try, for radios that scan or sleep, and sets the channel access of each try.

<!-- dmrgo:code file=v2/csbk/example_test.go lines=44-62 -->
<!-- /dmrgo:code -->

Report the state of the channel with `Channel` of each machine, so that a polite try waits for an idle channel. Until the first report the channel counts as idle, as it is for a network transport.

### 4. Check that a radio is present before a call

[Requester.RequestOACSU](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestOACSU) sends the OACSU presence check (UU_V_Req) of a private call, and the Responder answers with UU_Ans_Rsp. `ResponderConfig.OACSU` sets the answer.

<!-- dmrgo:code file=v2/csbk/example_test.go lines=18-35 -->
<!-- /dmrgo:code -->

### 5. Send any other CSBK

[Requester.Request](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Request) sends a CSBK that you build with [pdu.CSBK](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK). A [Match](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Match) from [NewMatch](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewMatch) gives the feature set and the opcodes of the answer that ends the request. A NACK_Rsp from the target also ends it.

### 6. Follow preambles, wake a repeater, keep direct mode timing

- [Preambles](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Preambles) follows the Pre_CSBKs on a channel and reports the CSBK or data that they announce.
- [Wakeup](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup) sends BS_Dwn_Act until the repeater outbound starts.
- [Timing](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Timing) follows, elects and propagates the timing leader of a TDMA direct mode channel with CT_CSBKs.

<!-- dmrgo:code file=v2/csbk/example_test.go lines=95-108 -->
<!-- /dmrgo:code -->

## Output

The radio check and the call alert, each answered:

<!-- dmrgo:output test=csbk.ExampleResponder -->
<!-- /dmrgo:output -->

A radio check that no radio answers:

<!-- dmrgo:output test=csbk.ExampleRequester_RequestRadioCheck -->
<!-- /dmrgo:output -->

The OACSU presence check:

<!-- dmrgo:output test=csbk.Example -->
<!-- /dmrgo:output -->

The wakeup of a repeater that does not answer:

<!-- dmrgo:output test=csbk.ExampleWakeup -->
<!-- /dmrgo:output -->

The configs of the Requester and the Responder:

<!-- dmrgo:config pkg=csbk type=RequesterConfig -->
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=csbk type=ResponderConfig -->
<!-- /dmrgo:config -->

## How it works

A CSBK is one data burst: 96 bits that hold the opcode, the feature set ID (FID), the data and a CRC. A request goes in the slot of its logical channel, and the answer comes in a slot of the same logical channel. The Requester waits T_AckWait for the answer after each try and sends N_CSBKRetry tries after the first. A polite try waits for an idle channel. A time critical try waits at most T_IdleSrch for an idle channel. Another polite try waits a random holdoff T_Holdoff while the channel is busy. (ETSI TS 102 361-2 clauses 5.1.1.1, 5.1.2 and 5.2.2, and clause A.1)

The Responder answers in a slot of the logical channel of the request, a TDMA frame or more after its start. On the offset channel timing of a BS, set with `ResponderConfig.Offset`, the answer goes one slot later, because the first inbound slot after the request leaves no time to decode it. ETSI gives no answer delay; this is the choice of dmrgo. A Responder answers a standard CSBK that it does not support with the Feature Not Supported NACK_Rsp. (ETSI TS 102 361-1 clauses 5.1.1 and 5.2.1.7, Table 5.1)

The Motorola radio check and call alert use FID 0x10 and have no public specification. Their layouts, and the timing of their answers, come from captures of an AnyTone radio through a Motorola XPR 8400 repeater.

The state machines: [csbk Requester](../machines/csbk-requester.md), [csbk Responder](../machines/csbk-responder.md), [csbk Preambles](../machines/csbk-preambles.md), [csbk Wakeup](../machines/csbk-wakeup.md) and [csbk Timing](../machines/csbk-timing.md).

## Related guides

- [Register radios and send text: ARS and TMS](register-radios-send-text.md)
- [Build and decode trunking messages](trunking.md)
- [Send SMS and confirmed data with retries](send-data.md)
- [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md)
- [Use an MMDVM modem](mmdvm-modem.md)
