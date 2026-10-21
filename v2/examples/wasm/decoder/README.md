# DMR decoder in the browser

This example runs the receivers of package `rf` in a web page, compiled to WebAssembly. It decodes DMR from:

- a microphone or line-in, such as the discriminator output of a scanner, through WebAudio;
- a dropped WAV file of discriminator audio, or a 2-channel WAV of I/Q;
- a dropped raw I/Q file: `.cu8` (rtl_sdr), `.cs8` (hackrf_transfer), `.cs16`, or `.cf32` and `.cfile` (GNU Radio);
- a live RTL-SDR with an R820T or R820T2 tuner, over WebUSB.

The page shows each burst with its slot, color code, SYNC, data type, FEC result and bit error rate. It also shows the voice LC (source, destination, call type), CSBKs, data headers, completed data packets, SMS text, GPS positions from LRRP or NMEA packets and from the embedded LC, talker alias, and a count of voice frames with their AMBE+2 FEC bit error rate. It does not decode AMBE+2 to audio.

The page uses no framework, no npm and no CDN: `index.html`, `app.js`, a Web Worker (`worker.js`) that runs the WebAssembly decoder, an AudioWorklet (`worklet.js`), and the WebUSB driver (`rtlsdr.js`).

## Build and serve

From the `v2` directory:

```sh
go run ./examples/wasm/decoder/serve
```

This builds `decoder.wasm` with `GOOS=js GOARCH=wasm`, and serves the page on http://localhost:8080/ with the `wasm_exec.js` of your Go installation. The microphone and WebUSB work only on localhost or HTTPS. Flags:

- `-tinygo` builds with TinyGo and serves the `wasm_exec.js` of TinyGo. TinyGo 0.41 needs Go 1.19 to 1.26 on the `PATH`.
- `-files DIR` serves a directory of recordings under `/files/`.
- `-addr` sets the listen address.

To build by hand:

```sh
cd examples/wasm/decoder
GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o decoder.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
```

The page also decodes a URL without user input, for tests: `http://localhost:8080/?src=/testdata/xpr8400_bs_voice_weak.cu8&format=cu8&rate=256000&iqTier=2`. When the decode completes, `window.result` resolves to the statistics.

## Inputs and tiers

| Input | Receiver | Default tier | Choices |
|---|---|---|---|
| Audio, any rate | `rf.DiscReceiver` at 24 kHz | 1 | 0 to 2 (TinyGo: 0) |
| `.cu8`, `.cs8` at 48 kHz or more | `rf.Receiver.Write` | 2 | 0 to 3 |
| `.cs16`, `.cf32`, or 8-bit below 48 kHz | `rf.Receiver.WriteBaseband` at 48 kHz | 2 | 0 to 3 |
| RTL-SDR | `rf.Receiver.Write` at 1.024 Msps | 2 | 0 to 3 |

The decoder resamples audio to 24 kHz and 16-bit or float I/Q to 48 kHz itself, with a copy of the resampler of `cmd/dmrrf`. Tier 3 is for offline use: it is too slow for live input.

## RTL-SDR over WebUSB

Chrome and Edge have WebUSB; Firefox and Safari do not. `rtlsdr.js` sets up the RTL2832U and the R820T tuner as librtlsdr does, then streams 8-bit I/Q from the bulk endpoint to the decoder. Other tuners, including the R828D of the RTL-SDR Blog V4, are not supported.

- **Linux:** the kernel DVB driver (`dvb_usb_rtl28xxu`) must not be bound to the device. Blacklist it as the rtl-sdr packages do. Your user also needs write access to the USB device node, for example from the udev rules of the rtl-sdr package.
- **Windows:** install the WinUSB driver for the device with Zadig.

The RTL2832U stalls I2C transfers to the tuner when its I2C repeater stays on while the bus is idle, so the driver turns the repeater off before each wait.

## Real-time factor

The real-time factor is the decoder CPU time divided by the length of the input: 0.1 decodes 10 seconds of input in 1 second. Measured on 2026-10-21 on an AMD Ryzen 9 7950X, with 20 s of a 2.048 Msps RTL-SDR capture of a repeater outbound stream, and the same stream converted to cs16 and to 48 kHz discriminator audio:

| Input | Tier | Go, Node 22 | Go, Chromium 153 | TinyGo, Chromium 153 |
|---|---|---|---|---|
| cu8, 2.048 Msps | 1 | 0.083 | 0.066 | 0.042 |
| cu8, 2.048 Msps | 2 | 0.115 | 0.098 | 0.051 |
| cu8, 2.048 Msps | 3 | 1.15 | | |
| cs16, 2.048 Msps | 2 | 0.208 | 0.203 | 0.159 |
| WAV, 48 kHz audio | 0 | | | 0.015 |
| WAV, 48 kHz audio | 1 | 0.016 | 0.017 | |
| WAV, 48 kHz audio | 2 | 0.053 | 0.050 | |
| RTL-SDR live, 1.024 Msps, no signal (Chrome 155) | 2 | | 0.123 | |

The size of `decoder.wasm` is 6.5 MB from Go (1.5 MB with zstd) and 1.7 MB from TinyGo (0.55 MB with zstd).

## Tests

```sh
go test ./examples/wasm/decoder
GOOS=js GOARCH=wasm go test -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./examples/wasm/decoder
```

The tests decode the I/Q fixtures of `cmd/dmrrf/testdata` through the decoder and check that it returns the same bursts as `rf.Receiver` and `rf.DiscReceiver` called directly, for cu8, cs16, WAV and float audio. They also check that a write allocates nothing.

`node_run.js` runs `decoder.wasm` under Node and prints the events and the real-time factor:

```sh
node node_run.js "$(go env GOROOT)/lib/wasm/wasm_exec.js" decoder.wasm ../../../cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8 cu8 256000
```

`browser_run.js` drives a headless Chromium through the DevTools protocol: it opens a page URL, waits for `window.result`, prints it, and can save a screenshot:

```sh
chromium --headless=new --remote-debugging-port=9333 &
node browser_run.js 9333 'http://localhost:8080/?src=/testdata/xpr8400_bs_voice_weak.cu8&format=cu8&rate=256000' '' shot.png
```

Built natively, the decoder decodes a file and prints the same events as the page, one JSON object per line:

```sh
go run ./examples/wasm/decoder -rate 256000 cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8
```
