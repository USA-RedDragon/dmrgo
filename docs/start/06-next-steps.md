# Next steps

This page lists where to go after the first five steps: the concepts behind the library, the task guides, the state machines, the build targets and the reference.

## What you did

- [Install](01-install.md): you added the module and got the captures.
- [Decode a burst](02-decode-a-burst.md): `layer2` turned 33 bytes into a burst, with its FEC result and its PDU.
- [Follow a voice call](03-follow-a-voice-call.md): a [voice.Receiver](../machines/voice-receiver.md) turned bursts into call events.
- [Send an SMS](04-send-an-sms.md): `sms` and `data` read a text and sent a confirmed reply.
- [Receive on a radio](05-receive-on-a-radio.md): `rf` recovered the bursts from I/Q samples.

## Learn the concepts

- [The layer model](../concepts/layers.md): how the packages stack, from bursts to applications, networks and the radio.
- [I/O-free services](../concepts/services.md): the method set that every state machine shares, and how to drive it from your own I/O and clock.
- [Errors and the never-panic rule](../concepts/errors.md): what the library returns for bad input and how to match it.
- [Allocation](../concepts/allocation.md): where the library allocates, and how to keep TinyGo from allocating in your code.
- [Tiers](../concepts/tiers.md): the four receiver tiers of `rf`, from a microcontroller to offline decoding.

## Do a task

The guides each build one program on an example with recorded output. Start with these:

- [Decode and re-encode captures](../guides/decode-captures.md).
- [Follow calls on both slots of a bit stream](../guides/follow-both-slots.md).
- [Clean up bursts for a hotspot](../guides/clean-up-bursts.md).
- [Rewrite and relay a voice call](../guides/relay-voice-call.md).

Other guides cover voice transcoding, data with retries, TCP and UDP over DMR, location, registration and text services, CSBK services, trunking, network protocols, basic privacy, and the receive and transmit sides of `rf`.

## Look up a state machine

Each state machine has a page with its states, config, timers and errors. The machines of the steps are [voice.Receiver](../machines/voice-receiver.md), [data.Receiver](../machines/data-receiver.md) and [data.Sender](../machines/data-sender.md).

## Pick a target

- [PC](../targets/pc.md).
- [Browser](../targets/browser.md), with WebAssembly.
- [Microcontrollers](../targets/mcu.md), with TinyGo.
- [C API](../targets/c.md), libdmrgo.

## Reference

- The [Go API](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) on pkg.go.dev.
- The [Receiver](../rx/README.md) and [Transmitter](../tx/README.md) datasheets.
- [Examples](../reference/examples.md): the example programs, which run offline on captures.
- [ETSI coverage](../reference/etsi.md): the status of each clause of ETSI TS 102 361.
