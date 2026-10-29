# csbk.Wakeup

This page describes the BS wakeup state machine of package csbk: it sends BS_Dwn_Act until the outbound of a BS starts.

## What it does

[`csbk.Wakeup`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup) is the MS side of the BS outbound activation. It sends a BS_Dwn_Act CSBK with MS sourced SYNC to the address of a BS. It then waits T_SyncWu for a BS sourced burst with the color code of the BS, and sends BS_Dwn_Act again, up to N_Wakeup times in all. It reports that the outbound started, that it did not start after the last BS_Dwn_Act, or that the caller canceled the wakeup. (ETSI TS 102 361-1 clause 5.2.2.2.4)

The Wakeup does no I/O. Each method takes the time `now` on a monotonic clock that the caller picks, and appends outputs to a slice of the caller.

## When to use it

Use a Wakeup in a radio, or in a device that acts as a radio, before it transmits to a repeater that hibernates and wakes only on BS_Dwn_Act. Run the wakeup first, and start the call, the data or the CSBK request when the Wakeup reports `ResultAnswered`.

A repeater that wakes on any valid burst does not need a Wakeup. On the BS side, a [bs.Repeater](bs-repeater.md) with `Config.Activation` set to [`bs.ActivationWakeup`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#ActivationWakeup) wakes only on BS_Dwn_Act.

## Config

[`csbk.NewWakeup`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#NewWakeup) takes a [`*csbk.WakeupConfig`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#WakeupConfig). A nil config is the zero config. `Slot` must be slot 1 or slot 2, so a nil config returns an error. `ColorCode` is the color code of the BS: the Wakeup sends BS_Dwn_Act with it and takes only an outbound data burst with it.

<!-- dmrgo:config pkg=csbk type=WakeupConfig -->
<!-- /dmrgo:config -->

## States and transitions

<!-- dmrgo:states pkg=csbk type=WakeupState -->
<!-- /dmrgo:states -->

These inputs move the state:

- `Wake` starts a wakeup. The first BS_Dwn_Act goes out at the next Tick.
- In sending, the Tick at the `Due` time emits one BS_Dwn_Act, and the Wakeup waits T_SyncWu for the outbound.
- While it waits, a BS sourced burst, or `OutboundSync`, ends the wakeup with `ResultAnswered`. When T_SyncWu passes, the Wakeup sends again, or ends with `ResultNoAnswer` after N_Wakeup BS_Dwn_Act bursts.
- `Cancel` ends a running wakeup with `ResultCanceled`. `Reset` ends it without an output.

The outbound counts only after the first BS_Dwn_Act went out. A BS data burst counts only when its slot type decodes and has the color code of the BS. A BS voice burst has no slot type, so its BS sourced voice SYNC is enough.

## Inputs and outputs

Every method that takes `dst` appends at most [`csbk.MaxOutputs`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#MaxOutputs) outputs, and does not allocate when `dst` has that room.

| Method | Takes | Returns |
|---|---|---|
| [`Wake`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Wake) | the address of the BS and the source address of the radio | an error |
| [`Burst`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Burst) | a raw burst and its [`layer2.Hint`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) | `dst` with `OutputDone` when the burst is the outbound |
| [`Decoded`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Decoded) | a burst that the caller decoded, or nil | the same as `Burst` |
| [`OutboundSync`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.OutboundSync) | the time now | `dst` with `OutputDone` and `ResultAnswered` while a wakeup runs |
| [`Tick`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Tick) | the time now | `dst` with a BS_Dwn_Act burst, or `OutputDone` with `ResultNoAnswer` |
| [`Deadline`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Deadline) | nothing | the time of the next Tick, and false when no timer runs |
| [`Lost`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Lost) | the time now | `dst`; it runs the timers as `Tick` does |
| [`Cancel`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Cancel) | the time now | `dst` with `ResultCanceled` while a wakeup runs |
| [`State`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.State), [`Reset`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Wakeup.Reset) | nothing | the state, or nothing |

Feed the bursts that you receive to `Burst` or `Decoded`. If your modem reports the outbound SYNC itself and gives you no burst, call `OutboundSync` when it does. `OutboundSync` and `Cancel` do not run the timers; `now` only gives them the shape of the other methods.

Each [`csbk.Output`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Output) is one of:

- `OutputBurst`: a BS_Dwn_Act to send at `Due`, with a `Hint` of a CSBK on the configured slot. Its `Call` has the BS address as the target and the radio as the source.
- `OutputDone`: the end of the wakeup, with `ServiceWakeup` and its [`Result`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#Result).

## Timers

| Timer | Config field | Default |
|---|---|---|
| T_SyncWu, the wait for the outbound after each BS_Dwn_Act (ETSI TS 102 361-1 clause F.1) | `SyncWait` | <!-- dmrgo:const pkg=csbk name=DefaultSyncWait --><!-- /dmrgo:const --> |
| N_Wakeup, the most BS_Dwn_Act bursts (ETSI TS 102 361-1 clause F.2) | `Attempts` | <!-- dmrgo:const pkg=csbk name=DefaultWakeups --><!-- /dmrgo:const -->, at most <!-- dmrgo:const pkg=csbk name=MaxRetries --><!-- /dmrgo:const --> |

T_SyncWu starts at the `Due` time of each BS_Dwn_Act. A negative `SyncWait` waits until the outbound or `Cancel`, and sends one BS_Dwn_Act only. `Lead` makes `Tick` emit each burst early, for an RF FIFO; the next BS_Dwn_Act then goes out when T_SyncWu ends, with its `Due` time one lead later.

## Worked example

This example wakes a repeater that does not answer. With the defaults, the Wakeup sends two BS_Dwn_Act bursts, one T_SyncWu apart, and ends one T_SyncWu after the second.

<!-- dmrgo:code file=v2/csbk/example_test.go symbol=ExampleWakeup -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=csbk.ExampleWakeup -->
<!-- /dmrgo:output -->

## Errors

<!-- dmrgo:errors pkg=csbk type=Wakeup -->
<!-- /dmrgo:errors -->

`Wake` returns [`csbk.ErrBusy`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk#ErrBusy) while a wakeup runs; match it with `errors.Is`. `NewWakeup` and `Wake` return a [`*dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) for a value out of range; match it with `errors.As`. A BS that does not wake gives an `OutputDone` with `ResultNoAnswer`, and no error.

## Related machines and guides

- [bs.Repeater](bs-repeater.md) is the BS side, which wakes on BS_Dwn_Act with `ActivationWakeup`.
- [csbk.Requester](csbk-requester.md) and [data.Sender](data-sender.md) send after the outbound starts.
- The guide [Radio check, call alert and other CSBK services](../guides/csbk-services.md) wakes a repeater with a Wakeup.
