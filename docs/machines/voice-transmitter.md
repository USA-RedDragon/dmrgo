# voice Transmitter

This page covers the voice Transmitter, the state machine that makes the bursts of one voice call on one logical channel.

## What it does

A [Transmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter) makes the bursts of one voice transmission, one burst per Tick, 60 ms apart. It works in two modes. In source mode, [Start](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Start) begins a call and you queue its vocoder frames. In relay mode, [Relay](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Relay) takes the events of a [Receiver](voice-receiver.md) and sends the call again with the local color code and SYNC. A call has Voice LC Headers, an optional PI Header, superframes of voice bursts A to F with the embedded LC, and Terminators with LC. A source call follows the channel access policy: a polite call waits for an idle channel. A call that starts in the hang time of a repeater can first send the channel authorization and wait for the repeater to repeat it. A Tier 2 reverse channel cease command ends the call. The transmit timeout ends a call that runs too long. (ETSI TS 102 361-1 clause 5.2, ETSI TS 102 361-2 clauses 5.1.1.6, 6.1 and 6.3)

## When to use it

Use a Transmitter when you send voice: a radio or a hotspot that keys up with frames from a vocoder, a bridge or repeater that sends a received call again, or a test tool that makes reference bursts. Use one Transmitter for each logical channel. To regenerate a call that a Receiver decoded, use Relay: the Transmitter corrects the AMBE+2 FEC, fills lost bursts, and sends the embedded LC again.

## Config

Pass a [TransmitterConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#TransmitterConfig) to [NewTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#NewTransmitter). The Slot must be set, so a nil config returns an error. Set Source to the SYNC source of your side: a repeater outbound or a radio. A radio that transmits on air sets Access, and ChanAuth when it starts calls in hang time. A sender that fills an RF FIFO ahead of air time sets Lead.

<!-- dmrgo:config pkg=voice type=TransmitterConfig -->
<!-- /dmrgo:config -->

Each call can also take [CallOptions](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#CallOptions) in Start: a PI Header, a talker alias, GPS, and the flags for a call in progress or in hang time.

## States and transitions

[State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.State) returns the current state.

<!-- dmrgo:states pkg=voice type=TransmitterState -->
<!-- /dmrgo:states -->

Start moves an idle Transmitter to the headers. A polite call on a busy channel first waits for [Channel](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Channel) to report an idle channel; when the idle search time runs out, the call ends as denied. A call with CallOptions.HangTime and a ChanAuth time first sends the channel authorization. The repeater then repeats it, or a Voice LC Header with another source denies the call, or no outbound burst comes and the call ends so that you can wake the repeater and start again. After the headers the Transmitter sends voice. [Stop](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Stop) sends the queued voice, pads the superframe to burst F with silence, and sends the terminators. After the transmit timeout the Transmitter sends the terminators and then waits for Stop, the release of the PTT. (ETSI TS 102 361-1 clause 5.2.2, ETSI TS 102 361-2 clauses 5.1.1.6.2 and 6.1)

## Inputs and outputs

Inputs in source mode:

- [Start](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Start) begins a call with a [Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Call) and optional CallOptions.
- [Frame](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Frame) and [Frames](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Frames) queue AMBE+2 frames. [VocoderSlot](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.VocoderSlot) and [VocoderSlots](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.VocoderSlots) queue 72-bit slots of any codec. [Room](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Room) returns the free space in the queue.
- [Stop](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Stop) ends the call.
- [Channel](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Channel) reports whether the channel is busy, for polite access.
- [Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Burst), [Decoded](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Decoded) and [Lost](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Lost) take the outbound bursts of the repeater during the channel authorization.
- [ReverseChannel](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.ReverseChannel) takes a Tier 2 RC command from the reverse channel and returns whether it ends the call.

Input in relay mode:

- [Relay](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Relay) takes each [Event](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Event) of a Receiver. [Dropped](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Dropped) returns the relayed voice bursts that did not fit.

Both modes:

- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Tick) appends the next [Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Output). Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Deadline) returns. A late caller calls Tick until Deadline is in the future, because each Tick returns at most one burst.
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter.Reset) ends the transmission without an output.

Each Output has an [OutputKind](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#OutputKind): a burst to send, with its Due time on air and a [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) for the transport, or the end of the transmission with its [EndReason](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#EndReason). The end comes in the same Tick as the last terminator. Give each method a slice with capacity <!-- dmrgo:const pkg=voice name=MaxOutputs --><!-- /dmrgo:const --> so that it does not allocate.

## Timers

- Burst period: one burst every 60 ms from the Due time of the first burst.
- Transmit timeout T_TO: the default is <!-- dmrgo:const pkg=constants name=TTO --><!-- /dmrgo:const -->. TransmitterConfig.Timeout changes it, and a negative value turns it off. (ETSI TS 102 361-2 clause 6.1)
- Idle search T_IdleSrch, the wait for an idle channel before a polite call: the default is <!-- dmrgo:const pkg=voice name=DefaultIdleSearch --><!-- /dmrgo:const -->. (ETSI TS 102 361-1 clause 5.2.2.2.5)
- Channel authorization T_MS_ChanAuth: off by default. The recommended values are <!-- dmrgo:const pkg=constants name=TMSChanAuth --><!-- /dmrgo:const --> and <!-- dmrgo:const pkg=constants name=TMSChanAuthMultiSite --><!-- /dmrgo:const --> for multi-site systems. (ETSI TS 102 361-2 clause 5.1.1.6.2)
- Random holdoff before a new channel authorization try: up to <!-- dmrgo:const pkg=voice name=DefaultHoldoffMax --><!-- /dmrgo:const --> by default. TransmitterConfig.Seed seeds the generator, so the same inputs give the same outputs. (ETSI TS 102 361-2 clause 5.1.1.6.2)
- Lead: Tick returns each burst this long before its Due time.

## Worked example

This example sends a short call as its source. It queues two bursts of silence frames, stops at once, and ticks until the Transmitter has nothing more to send.

<!-- dmrgo:code file=v2/voice/example_test.go symbol=ExampleTransmitter -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=voice.ExampleTransmitter -->
<!-- /dmrgo:output -->

The two queued bursts go out as voice. The rest of the superframe is silence, marked as fill, and the Terminator with LC follows.

The next example shows polite access. The call waits while the channel is busy and starts when it becomes idle. A second call is denied when the channel stays busy for the idle search time.

<!-- dmrgo:code file=v2/voice/access_example_test.go symbol=ExampleTransmitter_Channel -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=voice.ExampleTransmitter_Channel -->
<!-- /dmrgo:output -->

## Errors

Start and Relay return an error while a call runs, and a configuration error when the call does not make an LC. The queue methods return an error when the queue is full, when no call takes voice, and after the transmit timeout. NewTransmitter returns a configuration error for a field out of range. Match the errors with errors.Is or errors.As.

<!-- dmrgo:errors pkg=voice type=Transmitter -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [voice Receiver](voice-receiver.md): the source of the events that Relay takes.
- [voice Interrupter](voice-interrupter.md): sends the RC command that ReverseChannel takes.
- [voice Jitter](voice-jitter.md): set its Lead to the Lead of the Transmitter that relays its packets.
- bs Repeater (docs/machines/bs-repeater.md): runs a Transmitter in relay mode for each slot.
- csbk Wakeup (docs/machines/csbk-wakeup.md): wakes a repeater after a call ends with no outbound bursts.

Guides:

- Rewrite and relay a voice call
- Transcode voice: AMBE+2 FEC, Codec2, DVSI
- Transmit within a region's limits
