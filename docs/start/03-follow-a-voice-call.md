# Follow a voice call

This step feeds the bursts of the same capture to a voice.Receiver, which turns them into the events of a call: its start, its voice bursts, its end and the end of its session.

## Why a state machine

One burst does not give a call. A radio can join a call late, a burst can be lost, and a call ends at a terminator or when its bursts stop. A [voice.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) holds the state of the call on one logical channel and returns an event for each change. (ETSI TS 102 361-1 clause 5.1.2, Annex G, ETSI TS 102 361-2 clause 5.2)

The Receiver does no I/O and reads no clock. You give it the time of each burst, and it appends its events to a slice that you give it. [I/O-free services](../concepts/services.md) gives this rule for every machine of the library.

## The code

The capture holds no time stamps, so the example puts the bursts of the time slot 60 ms apart, as they are on air:

<!-- dmrgo:code file=v2/voice/example_test.go symbol=ExampleReceiver -->
<!-- /dmrgo:code -->

Run it from the `v2` directory:

```sh
go test -run '^ExampleReceiver$' -v ./voice
```

It prints:

<!-- dmrgo:output test=voice.ExampleReceiver -->
<!-- /dmrgo:output -->

## What the code does

- `voice.NewReceiver(nil)` makes a Receiver with the default config. [voice.ReceiverConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#ReceiverConfig) sets the color code, the timers and the other options.
- `rx.Burst(now, &raw, layer2.Hint{}, events[:0])` takes one 33-byte burst. The `layer2.Hint` holds the data of a transport about the burst besides its bits, such as the slot, the burst kind and a sequence number. A capture of bare bursts has none of these, so the hint is empty.
- `events` has the capacity `voice.MaxEvents`, so the slice never grows and the call does not allocate. See [Allocation](../concepts/allocation.md).
- `EventCallStart` carries the `Call`: the source, the target and the origin of the call identity. Here the origin is the Voice LC Header. On a late entry it is the embedded LC of the voice bursts.
- `EventVoice` comes once for each voice burst, with its superframe position and its AMBE+2 frames. A codec, such as a DVSI chip or a software codec, turns the frames into speech. See the package [vocoder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder).
- `EventCallEnd` carries the reason and the statistics of the call.
- After the call, `rx.Deadline()` returns the time of the next timer. `rx.Tick(at, ...)` at that time ends the hang time of the session with `EventSessionEnd`.

The [voice.Receiver](../machines/voice-receiver.md) page gives the states, the config and the timers of the machine.

Next: [Send an SMS](04-send-an-sms.md).
