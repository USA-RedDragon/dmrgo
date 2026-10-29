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

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=ExampleResponder_callAlert -->
```go
func ExampleResponder_callAlert() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	r, _ := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 3191868, ColorCode: 1, Source: layer2.SyncSourceMS})
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	now := time.Duration(0)
	for i := range 2 {
		var err error
		if i == 0 {
			err = q.RequestRadioCheck(now, 3191868, 3140598, nil)
		} else {
			err = q.RequestCallAlert(now, 3191868, 3140598, nil)
		}
		if err != nil {
			fmt.Println(err)
			return
		}
		out = q.Tick(now, out[:0])
		request := out[0]

		out = r.Burst(request.Due, &request.Burst, request.Hint, out[:0])
		fmt.Println("responder:", out[0].Kind, out[0].Service, "from", out[0].Call.Source)
		at, _ := r.Deadline()
		out = r.Tick(at, out[:0])
		answer := out[0]

		out = q.Burst(answer.Due, &answer.Burst, answer.Hint, out[:0])
		fmt.Println("requester:", out[0].Kind, out[0].Service, out[0].Result, "after", answer.Due-request.Due)
		now = answer.Due + time.Second
	}
}
```
<!-- /dmrgo:code -->

### 3. Handle a radio that does not answer

The Requester sends the request again after each wait for the answer, up to the number of retries of its config, and then returns `OutputDone` with `ResultNoAnswer`. Call `Deadline` and `Tick` in a loop until `Deadline` returns false. [RequestOptions](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#RequestOptions) adds Pre_CSBKs before the first try, for radios that scan or sleep, and sets the channel access of each try.

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=ExampleRequester_RequestRadioCheck -->
```go
func ExampleRequester_RequestRadioCheck() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 7, Source: layer2.SyncSourceMS, Retries: -1})
	if err := q.RequestRadioCheck(0, 3140598, 3191868, &csbk.RequestOptions{Preambles: 5}); err != nil {
		panic(err)
	}
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	bursts := 0
	for {
		at, ok := q.Deadline()
		if !ok {
			break
		}
		out = q.Tick(at, out[:0])
		for i := range out {
			if out[i].Kind == csbk.OutputBurst {
				bursts++
			} else {
				fmt.Println(bursts, "bursts,", out[i].Result, "at", at)
			}
		}
	}
}
```
<!-- /dmrgo:code -->

Report the state of the channel with `Channel` of each machine, so that a polite try waits for an idle channel. Until the first report the channel counts as idle, as it is for a network transport.

### 4. Check that a radio is present before a call

[Requester.RequestOACSU](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.RequestOACSU) sends the OACSU presence check (UU_V_Req) of a private call, and the Responder answers with UU_Ans_Rsp. `ResponderConfig.OACSU` sets the answer.

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=Example -->
```go
func Example() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	r, _ := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS})
	out := make([]csbk.Output, 0, csbk.MaxOutputs)

	if err := q.RequestOACSU(0, 1002, 1001, layer3.ServiceOptions{}, nil); err != nil {
		panic(err)
	}
	out = q.Tick(0, out[:0])
	request := out[0].Burst

	out = r.Burst(out[0].Due, &request, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Service, "from", out[0].Call.Source)
	at, _ := r.Deadline()
	out = r.Tick(at, out[:0])
	answer, due := out[0].Burst, out[0].Due

	out = q.Burst(due, &answer, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Result, "at", due)
}
```
<!-- /dmrgo:code -->

### 5. Send any other CSBK

[Requester.Request](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Requester.Request) sends a CSBK that you build with [pdu.CSBK](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK). A [Match](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Match) from [NewMatch](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewMatch) gives the feature set and the opcodes of the answer that ends the request. A NACK_Rsp from the target also ends it.

### 6. Follow preambles, wake a repeater, keep direct mode timing

- [Preambles](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Preambles) follows the Pre_CSBKs on a channel and reports the CSBK or data that they announce.
- [Wakeup](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup) sends BS_Dwn_Act until the repeater outbound starts.
- [Timing](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Timing) follows, elects and propagates the timing leader of a TDMA direct mode channel with CT_CSBKs.

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=ExampleWakeup -->
```go
func ExampleWakeup() {
	w, _ := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, ColorCode: 7})
	_ = w.Wake(0, 314926, 3191868)
	for {
		at, ok := w.Deadline()
		if !ok {
			break
		}
		for _, o := range w.Tick(at, nil) {
			if o.Kind == csbk.OutputDone {
				fmt.Println(at, o.Kind, o.Result)
			} else {
				fmt.Println(at, o.Kind)
			}
		}
	}
}
```
<!-- /dmrgo:code -->

## Output

The radio check and the call alert, each answered:

<!-- dmrgo:output test=csbk.ExampleResponder_callAlert -->
```text
responder: Request RadioCheck from 3140598
requester: Done RadioCheck Answered after 60ms
responder: Request CallAlert from 3140598
requester: Done CallAlert Answered after 60ms
```
<!-- /dmrgo:output -->

A radio check that no radio answers:

<!-- dmrgo:output test=csbk.ExampleRequester_RequestRadioCheck -->
```text
6 bursts, NoAnswer at 2.3s
```
<!-- /dmrgo:output -->

The OACSU presence check:

<!-- dmrgo:output test=csbk.Example -->
```text
Request OACSU from 1001
Done Answered at 60ms
```
<!-- /dmrgo:output -->

The wakeup of a repeater that does not answer:

<!-- dmrgo:output test=csbk.ExampleWakeup -->
```text
0s Burst
360ms Burst
720ms Done NoAnswer
```
<!-- /dmrgo:output -->

The configs of the Requester and the Responder:

<!-- dmrgo:config pkg=csbk type=RequesterConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `ColorCode` | `enums.ColorCode` | zero value | ColorCode is the color code of the slot types of the bursts sent. Other than 0, it also drops received CSBKs with another color code, as WakeupConfig.ColorCode does; 0 accepts every color code. |
| `Source` | `layer2.SyncSource` | zero value | Source selects the SYNC pattern: layer2.SyncSourceBS, the zero value, for a repeater outbound, layer2.SyncSourceMS for a radio. |
| `Slot` | `enums.Slot` | zero value | Slot is the TDMA slot of the machine, which it puts in the Hint of each burst it sends. It must be Slot1 or Slot2. |
| `AckWait` | `time.Duration` | `constants.TAckWait` (360ms) or `DefaultMotorolaAckWait` (2s) | AckWait is T_AckWait, the wait for the answer after each try. 0 means constants.TAckWait (360 ms), and DefaultMotorolaAckWait for the Motorola radio check and call alert. A negative value waits until an answer or Cancel. (ETSI TS 102 361-2 clause A.1) |
| `Retries` | `int` | `constants.NCSBKRetry` (1) | Retries is N_CSBKRetry, the number of tries after the first. 0 means constants.NCSBKRetry (1), a negative value means none, and it is at most MaxRetries. |
| `IdleSearch` | `time.Duration` | `DefaultIdleSearch` (540ms) | IdleSearch is T_IdleSrch, the wait for an idle channel before a polite time critical try. 0 means DefaultIdleSearch, and a negative value does not wait. |
| `HoldoffMax` | `time.Duration` | `DefaultHoldoffMax` (1s) | HoldoffMax is the upper end of the random holdoff T_Holdoff of a request that is not time critical: a polite try on a busy channel waits a random holdoff, and then another while the channel stays busy. 0 means DefaultHoldoffMax, and a negative value means no holdoff: the try waits for an idle channel. (ETSI TS 102 361-1 clause 5.2.2.3) |
| `Seed` | `uint64` | zero value | Seed seeds the generator of the random holdoff, so that the same inputs give the same outputs. Requesters on one channel need different seeds. |
| `Lead` | `time.Duration` | not machine-readable; see the description | Lead makes Tick emit a burst this long before its Due time, so that an RF FIFO can be filled ahead. 0 means at Due; it must not be negative. The timers of the machine keep to the Due times. |
| `Scratch` | `*layer2.Burst` | `layer2.NewBurst()` | Scratch, when set, is the Burst that Burst decodes into. Machines that run on one goroutine can share one, because a machine is done with it when its method returns. Make it with layer2.NewBurst. Nil means the Requester makes its own. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=csbk type=ResponderConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Address` | `uint32` | zero value | Address is the 24-bit address of the radio. The Responder answers the CSBKs that address it. |
| `ColorCode` | `enums.ColorCode` | zero value | ColorCode is the color code of the slot types of the bursts sent. Other than 0, it also drops received CSBKs with another color code, as WakeupConfig.ColorCode does; 0 accepts every color code. |
| `Source` | `layer2.SyncSource` | zero value | Source selects the SYNC pattern: layer2.SyncSourceBS, the zero value, for a repeater outbound, layer2.SyncSourceMS for a radio. |
| `Slot` | `enums.Slot` | zero value | Slot is the TDMA slot of the machine, which it puts in the Hint of each burst it sends. It must be Slot1 or Slot2. |
| `Access` | `enums.Access` | zero value | Access is the channel access policy of the answers. The zero value is polite to own color code. |
| `Offset` | `bool` | zero value | Offset puts the answers on the offset channel timing of a BS, in which the inbound slot of a logical channel starts half a TDMA frame after its outbound slot: the answer starts 90 ms after the start of the request, and then on that grid. The zero value is the aligned channel timing of a BS, and a direct channel, where it starts 60 ms after. (ETSI TS 102 361-1 clause 5.1.1, Table 5.1) |
| `OACSU` | `OACSUAnswer` | zero value | OACSU is the answer to UU_V_Req. The zero value is OACSUProceed. |
| `NoRadioCheck` | `bool` | zero value | NoRadioCheck leaves Motorola radio checks without an answer and without an OutputRequest. An AnyTone radio answers them only when its settings allow it. |
| `NoCallAlert` | `bool` | zero value | NoCallAlert leaves Motorola call alerts without an acknowledgement and without an OutputRequest. |
| `NoFNS` | `bool` | zero value | NoFNS leaves standard CSBKs that the Responder does not support without the Feature Not Supported NACK_Rsp, which is mandatory for an MS. Their OutputRequest stays. (ETSI TS 102 361-2 clause 5.1.2) |
| `IdleSearch` | `time.Duration` | `DefaultIdleSearch` (540ms) | IdleSearch is T_IdleSrch, the wait for an idle channel before a polite answer other than NACK_Rsp. 0 means DefaultIdleSearch, and a negative value does not wait. |
| `HoldoffMax` | `time.Duration` | `DefaultHoldoffMax` (1s) | HoldoffMax is the upper end of the random holdoff T_Holdoff of a NACK_Rsp: a polite NACK_Rsp on a busy channel waits a random holdoff, and then another while the channel stays busy. 0 means DefaultHoldoffMax, and a negative value means no holdoff: the NACK_Rsp waits for an idle channel. (ETSI TS 102 361-2 clause 5.1.2.2) |
| `Seed` | `uint64` | zero value | Seed seeds the generator of the random holdoff. |
| `Lead` | `time.Duration` | not machine-readable; see the description | Lead makes Tick emit a burst this long before its Due time, so that an RF FIFO can be filled ahead. 0 means at Due; it must not be negative. The timers of the machine keep to the Due times. |
| `Scratch` | `*layer2.Burst` | `layer2.NewBurst()` | Scratch, when set, is the Burst that Burst decodes into, as for RequesterConfig.Scratch. |
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
