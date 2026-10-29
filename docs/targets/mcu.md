# TinyGo

The library builds with TinyGo 0.41 and 0.43 for microcontrollers (for example RP2040, RP2350, ESP32-C3) and for WebAssembly. CI uses TinyGo 0.43, which needs Go 1.25 to 1.27 installed. TinyGo 0.41 needs Go 1.24 to 1.26.

```sh
cd v2
tinygo flash -target=pico ./examples/tinygo
```

The example prints its counts on the serial console once a second.

- CI builds [v2/examples/tinygo](../../v2/examples/tinygo/main.go) for the Raspberry Pi Pico and runs the library tests with TinyGo, except the host sweeps of `rf`.
- CI builds [v2/examples/rxaudio](../../v2/examples/rxaudio/main.go), the Tier 0 receiver in static memory, for a Cortex-M4 board with `-gc=none`. The link fails if the receiver allocates.
- Decode and encode make no heap garbage at the default stack size of these targets (8 KB). TinyGo puts each local variable larger than 256 bytes on the heap, and the library has no such local on those paths. TinyGo also puts on the heap a local of the caller whose address goes into the library, so keep such values in package-level or heap storage, as [Allocation](../concepts/allocation.md) shows.
- `vocoder/codec2` always uses its stub, so `codec2.New` returns `codec2.ErrUnavailable`.
