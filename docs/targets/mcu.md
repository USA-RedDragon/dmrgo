# Microcontrollers

This page shows how to build dmrgo with TinyGo for a microcontroller, which boards CI builds, and the memory and CPU limits of the Tier 0 radio code.

## What you need

- TinyGo 0.41 or 0.43. CI uses the TinyGo 0.43 image:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=226-226 -->
<!-- /dmrgo:code -->

- The Go release that your TinyGo needs on the `PATH`: Go 1.25 to 1.27 for TinyGo 0.43, and Go 1.24 to 1.26 for TinyGo 0.41.
- A board that TinyGo supports, such as a Raspberry Pi Pico, and a USB cable. The steps below use the Pico.
- A clone of the repository. The example embeds a capture from it.

## Build and flash

Connect the board, then from the `v2` directory of the clone:

```sh
tinygo flash -target=pico ./examples/tinygo
```

[v2/examples/tinygo](../../v2/examples/tinygo/main.go) decodes and encodes again each burst of a short voice call, follows the call with a [voice.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) and relays it with a [voice.Transmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Transmitter). It runs the capture once a second.

## Run

Open the serial console of the board, for example with `tinygo monitor`. Each second the example prints one line with its counts. The Go example of the program runs the same code once and prints these counts:

<!-- dmrgo:output test=examples/tinygo.Example -->
<!-- /dmrgo:output -->

To run that example on the host:

```sh
go test -run '^Example$' -v ./examples/tinygo
```

## Boards and builds in CI

The TinyGo job of CI builds these targets from the `v2` directory:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=240-261 -->
<!-- /dmrgo:code -->

- `pico`, `pico2` and `esp32c3-generic` are the Raspberry Pi Pico (RP2040), the Pico 2 (RP2350) and a generic ESP32-C3 board.
- `feather-m4` is a Cortex-M4F board. The builds with `-gc=none` have no heap: the link fails when the program allocates. [v2/examples/rxaudio](../../v2/examples/rxaudio/main.go) is the Tier 0 receiver in static memory. [v2/examples/txaudio](../../v2/examples/txaudio/main.go) and [v2/examples/txiq](../../v2/examples/txiq/main.go) are the Tier 0 modulators.
- `wasm` and `wasip1` are WebAssembly. See [Browser](browser.md).

The same job runs the library tests with TinyGo, except the commands, `rf` and the examples other than `examples/tinygo`, and runs the allocation tests at the stack size of the Pico:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=263-275 -->
<!-- /dmrgo:code -->

## Tier 0 limits

Tier 0 is the tier of `rf` for a microcontroller. `Init` sets up a Tier 0 receiver or transmitter in memory of the caller, such as a package-level variable, and does not allocate. Under TinyGo, the default tier is Tier 0. The doc comments of the four types give their budgets.

[rf.Receiver.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Receiver.Init), for I/Q that a radio chip filtered to the channel:

<!-- dmrgo:doc pkg=rf name=Receiver.Init -->
<!-- /dmrgo:doc -->

[rf.AudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver), for the output of an FM discriminator:

<!-- dmrgo:doc pkg=rf name=AudioReceiver -->
<!-- /dmrgo:doc -->

[rf.Transmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Transmitter.Init), for I/Q:

<!-- dmrgo:doc pkg=rf name=Transmitter.Init -->
<!-- /dmrgo:doc -->

[rf.AudioTransmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTransmitter.Init), for the modulator input of an FM radio:

<!-- dmrgo:doc pkg=rf name=AudioTransmitter.Init -->
<!-- /dmrgo:doc -->

The [Receiver](../rx/README.md) datasheet gives the sensitivity of each tier, and the [Transmitter](../tx/README.md#tier-0-cost) datasheet gives the CPU, RAM and flash of the Tier 0 modulators on a Cortex-M4F.

## Heap rules

Decode and encode make no heap garbage at the default stack size of these targets (8 KB). TinyGo puts each local variable larger than 256 bytes on the heap, and the library has no such local on those paths.

TinyGo also puts a local variable of your code on the heap when its address goes into the library, such as the `Data` of [layer2.BuildBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildBurst). The [package doc of dmrgo](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) lists those calls. Keep such values in package-level variables, or in a struct that you allocate once. [v2/examples/rxaudio](../../v2/examples/rxaudio/main.go) keeps its receiver and its sample buffer at package level:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=rx -->
<!-- /dmrgo:code -->

It then sets up the receiver in that memory with `Init`:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go lines=22-24 -->
<!-- /dmrgo:code -->

[Allocation](../concepts/allocation.md) lists each place where the library can allocate, and the tests that check it.

## Other limits

- [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2) always uses its stub under TinyGo, so [codec2.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#New) returns [codec2.ErrUnavailable](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#ErrUnavailable).
- CI does not run the tests of `rf` under TinyGo. The `feather-m4` builds above check that its Tier 0 code builds and links without a heap.
