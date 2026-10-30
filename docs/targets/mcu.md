# Microcontrollers

This page shows how to build dmrgo with TinyGo for a microcontroller, which boards CI builds, and the memory and CPU limits of the Tier 0 radio code.

## What you need

- TinyGo 0.41 or 0.44. CI uses the TinyGo 0.44 image:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol=jobs.tinygo.container.image -->
```yaml
image: tinygo/tinygo:0.44.0@sha256:98126534a0653966913b803d26805ea79250a244217d921d74044cfedc1d24a2
```
<!-- /dmrgo:code -->

- The Go release that your TinyGo needs on the `PATH`: Go 1.25 to 1.27 for TinyGo 0.44, and Go 1.24 to 1.26 for TinyGo 0.41.
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
```text
bursts 15 voice 13 data 2 re-encoded 15 corrected 0
call from 3191868 to 9990 calls 1 end Terminator relayed 20
```
<!-- /dmrgo:output -->

To run that example on the host:

```sh
go test -run '^Example$' -v ./examples/tinygo
```

## Boards and builds in CI

The TinyGo job of CI builds these targets from the `v2` directory:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol="jobs.tinygo.steps[Build the example:Build the Tier 0 modulator examples without a heap]" -->
```yaml
- name: Build the example
  run: |
    tinygo build -target=pico -size short -o tinygo-pico.uf2 ./examples/tinygo
    for target in pico2 esp32c3-generic wasip1; do
      tinygo build -target="$target" -o "tinygo-$target.elf" ./examples/tinygo
    done

- name: Build the receiver and modulator examples for WebAssembly
  run: |
    for target in wasm wasip1; do
      for example in rxaudio txaudio txiq; do
        tinygo build -target="$target" -o "$example-$target.wasm" "./examples/$example"
      done
    done

- name: Build the Tier 0 receiver example without a heap
  run: tinygo build -target=feather-m4 -serial=uart -gc=none -scheduler=none -size short -o rxaudio.elf ./examples/rxaudio

- name: Build the Tier 0 modulator examples without a heap
  run: |
    tinygo build -target=feather-m4 -serial=uart -gc=none -scheduler=none -size short -o txaudio.elf ./examples/txaudio
    tinygo build -target=feather-m4 -serial=uart -gc=none -scheduler=none -size short -o txiq.elf ./examples/txiq
```
<!-- /dmrgo:code -->

- `pico`, `pico2` and `esp32c3-generic` are the Raspberry Pi Pico (RP2040), the Pico 2 (RP2350) and a generic ESP32-C3 board.
- `feather-m4` is a Cortex-M4F board. The builds with `-gc=none` have no heap: the link fails when the program allocates. [v2/examples/rxaudio](../../v2/examples/rxaudio/main.go) is the Tier 0 receiver in static memory. [v2/examples/txaudio](../../v2/examples/txaudio/main.go) and [v2/examples/txiq](../../v2/examples/txiq/main.go) are the Tier 0 modulators.
- `wasm` and `wasip1` are WebAssembly. See [Browser](browser.md).

The same job runs the library tests with TinyGo, except the commands, `rf` and the examples other than `examples/tinygo`, and runs the allocation tests at the stack size of the Pico:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol="jobs.tinygo.steps[Test the library:Check the heap allocation sites of the whole API for the pico and riscv-qemu]" -->
```yaml
- name: Test the library
  run: tinygo test $(go list ./... | grep -v -e /cmd/ -e /examples/ -e /rf$) ./examples/tinygo

- name: Test the example at the pico stack size
  run: tinygo test -stack-size=8KB ./examples/tinygo

- name: Test the allocations at the pico stack size and collector
  run: |
    tinygo test -gc=conservative ./internal/allocs
    tinygo test -stack-size=8KB -gc=conservative ./internal/allocs

- name: Check the heap allocation sites of the whole API for the pico and riscv-qemu
  run: DMRGO_TINYGO_ALLOCS=1 go test -run 'TestTinyGoHeapAllocations|TestTinyGoTablesInFlash' -v ./internal/allocgate
```
<!-- /dmrgo:code -->

## Tier 0 limits

Tier 0 is the tier of `rf` for a microcontroller. `Init` sets up a Tier 0 receiver or transmitter in memory of the caller, such as a package-level variable, and does not allocate. Under TinyGo, the default tier is Tier 0. The doc comments of the four types give their budgets.

[rf.Receiver.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Receiver.Init), for I/Q that a radio chip filtered to the channel:

<!-- dmrgo:doc pkg=rf name=Receiver.Init -->
Init sets up r for c, in place of any state it holds, in Tier0 at a Rate of 9600 or 12000. It does not allocate, so that r can live in memory of the caller. On a Cortex-M4F at 9.6 kHz the receiver needs up to 5.0 MHz and 2,248 B with its stack, built with TinyGo -opt z, over 2 KB by the accepted cost of the detection of standalone RC bursts and of the MS bursts of both TDMA slots and of the clock rate tracking. Init returns dmrgo.ErrNilReceiver for a nil r, and a \*dmrgo.ConfigError, with r unchanged, when a field of c is out of range.
<!-- /dmrgo:doc -->

[rf.AudioReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioReceiver), for the output of an FM discriminator:

<!-- dmrgo:doc pkg=rf name=AudioReceiver -->
AudioReceiver recovers DMR bursts from the output of an FM discriminator, as the radio chips of hotspots and MMDVM modems give it: real samples in any units, with any gain and DC offset, which the receiver measures itself. It takes audio of either polarity. Above Tier0, an AudioReceiver from NewAudioReceiver outside TinyGo also takes audio that a high-pass filter cut below about 300 Hz, as the speaker output of a scanner gives it. With AudioRxConfig.PreEmphasis, every tier also takes the de-emphasized audio of the speaker or accessory output of a radio. With Tier0 it keeps about 1.4 KB of state, for microcontrollers; on a Cortex-M4F at 9.6 kHz it needs up to 4.9 MHz and 2,232 B with its stack, built with TinyGo -opt z, over 2 KB by the accepted cost of the detection of standalone RC bursts and of the MS bursts of both TDMA slots and of the clock rate tracking. An AudioReceiver that Init sets up in memory of the caller, such as a package variable, uses no heap in stream mode. An AudioReceiver is not safe for concurrent use, and it must not be copied after Init or NewAudioReceiver.
<!-- /dmrgo:doc -->

[rf.Transmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#Transmitter.Init), for I/Q:

<!-- dmrgo:doc pkg=rf name=Transmitter.Init -->
Init sets up t for c, in place of any state it holds, in Tier0 at a Rate of 96000. It does not allocate, so that t can live in memory of the caller. It returns dmrgo.ErrNilReceiver for a nil t, and a \*dmrgo.ConfigError or \*RegionError, with t unchanged, as NewTransmitter does.
<!-- /dmrgo:doc -->

[rf.AudioTransmitter.Init](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf#AudioTransmitter.Init), for the modulator input of an FM radio:

<!-- dmrgo:doc pkg=rf name=AudioTransmitter.Init -->
Init sets up t for c, in place of any state it holds, in Tier0 at a Rate of 24000 or 48000. It does not allocate, so that t can live in memory of the caller. It returns the errors of Transmitter.Init.
<!-- /dmrgo:doc -->

The [Receiver](../rx/README.md) datasheet gives the sensitivity of each tier, and the [Transmitter](../tx/README.md#tier-0-cost) datasheet gives the CPU, RAM and flash of the Tier 0 modulators on a Cortex-M4F.

## Heap rules

Decode and encode make no heap garbage at the default stack size of these targets (8 KB). TinyGo puts each local variable larger than 256 bytes on the heap, and the library has no such local on those paths.

TinyGo also puts a local variable of your code on the heap when its address goes into the library, such as the `Data` of [layer2.BuildBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildBurst). The [package doc of dmrgo](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) lists those calls. Keep such values in package-level variables, or in a struct that you allocate once. [v2/examples/rxaudio](../../v2/examples/rxaudio/main.go) keeps its receiver and its sample buffer at package level:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=rx -->
```go
var (
	rx     rf.AudioReceiver
	buf    [256]int16
	bursts int
)
```
<!-- /dmrgo:code -->

Its `main` then sets up the receiver in that memory with `Init` and writes the samples to it:

<!-- dmrgo:code file=v2/examples/rxaudio/main.go symbol=main -->
```go
func main() {
	if err := rx.Init(rf.AudioRxConfig{Rate: 9600, OnBursts: onBursts}); err != nil {
		return
	}
	for range 1000 {
		read(buf[:])
		if _, err := rx.Write(buf[:]); err != nil {
			return
		}
	}
	if err := rx.Flush(); err != nil {
		return
	}
	println("bursts:", bursts > 0)
}
```
<!-- /dmrgo:code -->

[Allocation](../concepts/allocation.md) lists each place where the library can allocate, and the tests that check it.

## Other limits

- [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2) always uses its stub under TinyGo, so [codec2.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#New) returns [codec2.ErrUnavailable](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#ErrUnavailable).
- CI does not run the tests of `rf` under TinyGo. The `feather-m4` builds above check that its Tier 0 code builds and links without a heap.
