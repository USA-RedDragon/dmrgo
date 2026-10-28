# Transmitter

Package `rf` turns DMR bursts into a signal for a radio:

- `Transmitter` makes I/Q for a software radio, in the formats CU8, CS8, CS16 and CF32 of the receiver.
- `AudioTransmitter` makes the deviation signal that drives the modulator input of an FM radio, as codes of a converter of 8 to 16 bits, signed or offset binary (`Unsigned`).

Both use the config fields of the receivers: `Tier`, `Rate`, `Format` or `Bits`, and `OnSamples`. With `OnSamples` set, `Write` and `Flush` send the samples to it in chunks (stream mode). With `OnSamples` nil, `AppendSamples` and `AppendFlush` append them to a slice of the caller (append mode). `Init` sets up Tier 0 in memory of the caller and does not allocate. [v2/examples/txiq](../../v2/examples/txiq/main.go) and [v2/examples/txaudio](../../v2/examples/txaudio/main.go) are example programs for each.

## Bursts and keying

- Each `Burst` goes on the 30 ms slot grid by its `Stream` and `Slot`. The slot of a burst must increase within its stream, or `Write` returns `ErrBurstOrder`.
- A BS-sourced burst keeps the carrier up into the next slot. Any other burst is an MS burst, with the power ramps in its guard time.
- A gap of one slot within a `Stream` is an idle slot with the carrier down, as an MS sends on one TDMA channel.
- A new `Stream`, or a gap of more than one slot, keys down. The next burst keys up at once, so the caller sets the time between transmissions.
- `Active` is true from the key-up of the first burst to the key-down after the last. For `AudioTransmitter` it is the push-to-talk of the radio.

## Tiers

| Tier | Modulator | I/Q rates | Audio rates |
| --- | --- | --- | --- |
| Tier 0 | Fixed-point multiply and accumulate over tables in flash, for a microcontroller. The default of `Init` and of TinyGo builds. | 96 kHz | 24 and 48 kHz |
| Tier 1 | Pulse shaping in float32 over eight symbols on each side, I/Q from an interpolated cosine table. The default of `NewTransmitter` and `NewAudioTransmitter`. | 48 kHz to 20 MHz, 2.4 MHz by default | 8 to 96 kHz, 48 kHz by default |
| Tier 2 | Pulse shaping in float64 over sixteen symbols on each side, I/Q from float64 sine and cosine. | as Tier 1 | as Tier 1 |

Tier 3 has no modulator of its own, and both constructors return a `*dmrgo.ConfigError` for it.

## Formats and rates

- `TxConfig.Rate` 0 is 2.4 MHz. Tier 0 makes 96 kHz only: I/Q at 192 kHz takes 7.4 MHz of a Cortex-M4, over the 4 MHz budget of the tier.
- I/Q under 62.5 kHz puts the image of the main lobe within two channels of the carrier, where no limit can count on a filter of the radio. The limits do not accept it.
- `TxConfig.OffsetHz` moves the carrier from the center of the I/Q. It must be under `Rate`/4 in size. Tier 0 rounds it to whole Hz.
- `AudioTxConfig.Rate` 0 is 48 kHz. Tier 1 and Tier 2 make a rate from 8 kHz whose ratio to 4800 Hz has a numerator of at most 1024 in lowest terms, such as 8000, 22050 or 44100 Hz. The limits are measured up to 96 kHz.
- `AudioTxConfig.Bits` 0 is 16. Each sample holds one code in its low bits.
- `AudioTxConfig.Level` is the code of a run of symbol +3 (1944 Hz of deviation) from the center, from 1 to 16000. 0 is the most that fits in `Bits` with `Offset` over every run of symbols. `Offset` moves the center code.

## Limits

`Region`, `Service` and `Power` state the limits that the signal must keep.

- `Region`: `RegionUS` (47 CFR 90.210 Emission Mask D for land mobile, 47 CFR 97.307 for amateur), `RegionCEPT` (ETSI EN 300 113 for land mobile, ERC/REC 74-01 for amateur), `RegionChina` (the 2025 MIIT notice, land mobile), `RegionJapan` (the Radio Equipment Regulations, land mobile). The zero value `RegionWorld` keeps the strictest limit of each region at once. `RegionUS` does not cover the 800 MHz bands of FCC Mask H or the 90.691 EA mask.
- `Service` is `dmrgo.Service`, the type that package `privacy` also takes. The zero value `ServiceUnknown` keeps the land mobile and amateur limits at once.
- `Power` is the most power of the station in dBm, from 1 to 63. 0 is the highest legal power of the service in the region. The limits use the power classes 1 W (up to 30 dBm), 10 W (40 dBm), 100 W (50 dBm) and 1000 W (60 dBm).

The zero config is the strictest setup: every region, every licensed service and the highest legal power of each. A setup that no measurement shows to keep its limits with margin gets a `*RegionError`, with the config field in `Field` and the cause in `Reason`. Each cause has one static `RegionError`, so a failed call does not allocate. Match it with `errors.As`. The library makes no compliance claim. It only keeps its signal from being the cause of a failure of the limits that it measured.

These setups get a `RegionError`:

| Setup | `Reason` |
| --- | --- |
| Every setup, until the signal is measured 10 dB under the spurious domain limits of its region | the signal is not measured 10 dB under the spurious domain limits |
| A rate under the least rate of the tables below | no measurement keeps the limits at this rate |
| `Power` above 60 dBm | the limits above 60 dBm are not measured |
| `Power` above the highest legal power of the service in the region | above the highest legal power of the service in the region |
| `ServiceLicenseFree` (PMR446, the China 409 MHz walkie-talkie with -70 dBc in the alternate channel) | the limits of the license-free service are not measured |
| `ServiceLicenseFree` in the US or Japan | the region has no license-free DMR service |
| `ServiceAmateur` or `ServiceUnknown` in China or Japan | the amateur limits of the region are not measured |
| `AudioTxConfig.PreEmphasis` | the limits with pre-emphasis are not measured |
| `AudioTxConfig.Level` under the measured level | under the level that the limits are measured at for Bits |

The spurious domain margin is not measured yet, so today no setup is accepted. The tables below give what the other checks accept.

### Input low pass

`AudioTxConfig.InputLowpassHz` states the cutoff of a low pass of at least two poles that the radio puts before its modulator input. 0 is none: the converter drives the modulator with no filter. The limits are measured with a two-pole Butterworth low pass at 8000, 5000 and 3000 Hz, and a stated cutoff takes the next looser of these. Thus 6000 Hz counts as 8000 Hz, and a cutoff above 8000 Hz counts as none. A low pass lowers the least rate that keeps the limits. A negative cutoff gets a `*dmrgo.ConfigError`.

### Level under 16 bits

The limits are measured at a `Level` of about half of full scale: 15000 codes at 16 bits, and at 8, 10 and 12 bits 65, 260 and 1040 codes. A depth between these takes the next lower one, with the level doubled for each bit over it. Tier 1 and Tier 2 at 24 kHz and above reach 2.12 times `Level`, so 1040 codes do not fit 12 bits. There the limits are measured at the most level that fits the bits at `Offset` 0, about 966 codes at 12 bits and 58 at 8 bits. A `Level` under the measured level gets a `RegionError`.

## Tier 0 cost

| Tier 0 transmitter | Cortex-M4F CPU | RAM with stack | Flash |
| --- | --- | --- | --- |
| I/Q at 96 kHz, CS16 | 4.8 MHz | 968 B | 34.1 KB |
| Audio at 48 kHz, 12-bit | 3.7 MHz | 724 B | 30.7 KB |
| Audio at 24 kHz, 12-bit | 3.7 MHz | 764 B | 30.5 KB |

Measured from TinyGo 0.43 `-opt z -gc=none` builds in QEMU (`mps2-an386`, Cortex-M4), with BS-sourced bursts on every slot and the spurious domain check open for the measurement. CPU counts the cycles of the `rf` and `fsktx` code and of the bench loop per second of signal. RAM is the `Transmitter` or `AudioTransmitter` and the most stack under it.

<!-- txdoc:begin -->

## Least sample rate

Each cell gives the least sample rate in kHz for the power classes 1 W, 10 W, 100 W and 1000 W, in that order. A dash is a setup that no rate of the grid keeps. `TestMinRatesGenerated` measures the grid, and these tables come from its output, `minrate_gen.go` in `v2/internal/fsktx`. Tier 0 makes audio at 24 and 48 kHz and I/Q at 96 kHz only, and audio with pre-emphasis at 48 kHz only.

### Tier 0

| Input | World, land mobile | World, amateur | US, land mobile | US, amateur | CEPT, land mobile | CEPT, amateur | China, land mobile | Japan, land mobile |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Audio 8-bit | 48 / - / - / - | 48 / 48 / - / - | 24 / 48 / - / - | 48 / 48 / - / - | 48 / - / - / - | 24 / 24 / 48 / - | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 8-bit, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 8-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 8-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 10-bit | 48 / - / - / - | 48 / 48 / - / - | 24 / 48 / - / - | 48 / 48 / - / - | 48 / - / - / - | 24 / 24 / 48 / - | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 10-bit, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 10-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 10-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 12-bit | 48 / - / - / - | 48 / 48 / - / - | 24 / 48 / - / - | 48 / 48 / - / - | 48 / - / - / - | 24 / 24 / 48 / - | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 12-bit, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 12-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 12-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 16-bit | 48 / - / - / - | 48 / 48 / - / - | 24 / 48 / - / - | 48 / 48 / - / - | 48 / - / - / - | 24 / 24 / 48 / - | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 16-bit, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 48 / 48 / 48 / 48 | 24 / 24 / 24 / 24 |
| Audio 16-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| Audio 16-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 |
| I/Q 16-bit | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 | 96 / 96 / 96 / 96 |
| I/Q 8-bit | - / - / - / - | - / - / - / - | - / - / - / - | 96 / - / - / - | - / - / - / - | 96 / 96 / 96 / 96 | - / - / - / - | 96 / 96 / 96 / 96 |
| Audio 8-bit, pre-emphasis | - / - / - / - | - / - / - / - | - / - / - / - | 48 / - / - / - | - / - / - / - | 48 / 48 / - / - | - / - / - / - | - / - / - / - |
| Audio 8-bit, pre-emphasis, 8000 Hz low pass | - / - / - / - | - / - / - / - | - / - / - / - | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 | - / - / - / - | - / - / - / - |
| Audio 8-bit, pre-emphasis, 5000 Hz low pass | - / - / - / - | - / - / - / - | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 |
| Audio 8-bit, pre-emphasis, 3000 Hz low pass | - / - / - / - | - / - / - / - | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 |
| Audio 10-bit, pre-emphasis | - / - / - / - | - / - / - / - | - / - / - / - | 48 / - / - / - | - / - / - / - | 48 / 48 / - / - | - / - / - / - | 48 / 48 / 48 / 48 |
| Audio 10-bit, pre-emphasis, 8000 Hz low pass | - / - / - / - | - / - / - / - | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 | - / - / - / - | 48 / 48 / 48 / 48 |
| Audio 10-bit, pre-emphasis, 5000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 10-bit, pre-emphasis, 3000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 12-bit, pre-emphasis | - / - / - / - | 48 / - / - / - | 48 / - / - / - | 48 / - / - / - | - / - / - / - | 48 / 48 / - / - | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 12-bit, pre-emphasis, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 12-bit, pre-emphasis, 5000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 12-bit, pre-emphasis, 3000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 16-bit, pre-emphasis | - / - / - / - | 48 / - / - / - | 48 / - / - / - | 48 / - / - / - | - / - / - / - | 48 / 48 / - / - | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 16-bit, pre-emphasis, 8000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 16-bit, pre-emphasis, 5000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |
| Audio 16-bit, pre-emphasis, 3000 Hz low pass | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 | 48 / 48 / 48 / 48 |

### Tier 1

| Input | World, land mobile | World, amateur | US, land mobile | US, amateur | CEPT, land mobile | CEPT, amateur | China, land mobile | Japan, land mobile |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Audio 8-bit | 96 / - / - / - | 96 / 96 / - / - | 48 / 48 / - / - | 25.5 / 45 / - / - | 96 / - / - / - | 8 / 8 / 38.5 / - | 96 / 96 / 96 / 96 | 19 / 19 / 19 / 19 |
| Audio 8-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 26.5 / 26.5 | 8 / 8 / 8 / 8 | 26 / 26 / 26 / 26 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 8-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 8-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 10-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22.5 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 10-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 10-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 10-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 12-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22.5 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 12-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 12-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 12-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 16-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22.5 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 16-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 16-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 16-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| I/Q 16-bit | 62.5 / 62.5 / 62.5 / 210.224 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 210.224 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 |
| I/Q 8-bit | - / - / - / - | 6000 / 6000 / - / - | 176.777 / - / - / - | 62.5 / 1024 / - / - | - / - / - / - | 62.5 / 500 / - / - | 6000 / 6000 / 6000 / 6000 | 62.5 / 62.5 / 62.5 / 62.5 |

### Tier 2

| Input | World, land mobile | World, amateur | US, land mobile | US, amateur | CEPT, land mobile | CEPT, amateur | China, land mobile | Japan, land mobile |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Audio 8-bit | 96 / - / - / - | 96 / 96 / - / - | 48 / 48 / - / - | 25.5 / 45 / - / - | 96 / - / - / - | 8 / 8 / 38.5 / - | 96 / 96 / 96 / 96 | 19 / 19 / 19 / 19 |
| Audio 8-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 26.5 / 26.5 | 8 / 8 / 8 / 8 | 26 / 26 / 26 / 26 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 8-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 8-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 10-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 10-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 10-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 10-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 12-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22.5 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 12-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 12-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 12-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| Audio 16-bit | 45.5 / - / - / - | 34.5 / 45 / 96 / - | 22.5 / 40 / - / - | 25.5 / 45 / 96 / 96 | 45.5 / - / - / - | 8 / 8 / 38.5 / - | 34.5 / 34.5 / 34.5 / 34.5 | 19 / 19 / 19 / 19 |
| Audio 16-bit, 8000 Hz low pass | 28 / 28 / 28 / 28 | 28 / 28 / 28 / 28 | 18 / 18 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 28 / 28 / 28 / 28 | 17 / 17 / 17 / 17 |
| Audio 16-bit, 5000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 16.5 / 16.5 / 19.5 / 19.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 14.5 / 14.5 / 14.5 / 14.5 |
| Audio 16-bit, 3000 Hz low pass | 24 / 24 / 24 / 24 | 24 / 24 / 24 / 24 | 15 / 15 / 15.5 / 15.5 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 | 24 / 24 / 24 / 24 | 8 / 8 / 8 / 8 |
| I/Q 16-bit | 62.5 / 62.5 / 62.5 / 210.224 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 210.224 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 | 62.5 / 62.5 / 62.5 / 62.5 |
| I/Q 8-bit | - / - / - / - | - / - / - / - | 176.777 / - / - / - | 62.5 / 1024 / - / - | - / - / - / - | 62.5 / 500 / - / - | - / - / - / - | 62.5 / 62.5 / 62.5 / 62.5 |

## Measured margins

Each cell is the margin of the signal to the limit, the worst over the measured points. A limit that changes with power has one value for each power class, 1 W, 10 W, 100 W and 1000 W, in that order. A positive value is inside the limit. A setup is accepted only with at least 3 dB on each limit in dB, except -0.01 dB on the power slope, and at least 0 on the others. Audio is at 48 kHz, I/Q at 96 kHz for Tier 0 and at 2.4 MHz for Tier 1 and Tier 2, with no input low pass. A dash is a limit that does not apply to the input. `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures them into `v2/internal/fsktx/testdata/txmargins.json` and writes these tables.

### Tier 0 margins

| Limit | Audio 8-bit | Audio 10-bit | Audio 12-bit | Audio 16-bit | I/Q 16-bit | I/Q 8-bit | Audio 8-bit, pre-emphasis | Audio 10-bit, pre-emphasis | Audio 12-bit, pre-emphasis | Audio 16-bit, pre-emphasis |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Deviation error, % under the 10% limit (TS 102 361-1 10.2.2.3) | +9.1 | +9.1 | +9.1 | +9.1 | +8.8 | +8.9 | +7.5 | +7.8 | +7.7 | +7.7 |
| Adjacent channel power, dB (EN 300 113 7.4) | +3.5 | +5.5 | +5.7 | +5.7 | +5.0 | -0.8 | -13.1 | -1.8 | +3.8 | +4.7 |
| Alternate channel power, dB (EN 300 113 7.4) | +22.9 | +28.9 | +29.7 | +29.8 | +29.9 | +0.4 | +3.4 | +14.8 | +24.3 | +27.0 |
| Transient adjacent channel power, dB (EN 300 113 7.9) | - | - | - | - | +27.0 | +12.2 | - | - | - | - |
| Transient alternate channel power, dB (China) | - | - | - | - | +22.2 | -1.2 | - | - | - | - |
| Attack time t_p, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 | - | - | - | - |
| Release time t_d, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 | - | - | - | - |
| Transient power slope, dB (EN 300 113 7.9) | - | - | - | - | -0.0 | -0.0 | - | - | - | - |
| Transient frequency error, kHz (EN 300 113 7.9) | - | - | - | - | +6.2 | +6.2 | - | - | - | - |
| Spurious emissions, dB (EN 300 113 7.5) | +4.1 / -5.9 / -15.9 / -25.9 | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.9 | +4.2 / -5.8 / -15.8 / -25.8 | - | - | -2.8 / -12.8 / -22.8 / -32.8 | -2.6 / -12.6 / -22.6 / -32.6 | -2.6 / -12.6 / -22.6 / -32.6 | -2.6 / -12.6 / -22.6 / -32.6 |
| Emission Mask D, dB (47 CFR 90.210(d)) | +8.5 / +6.3 / -3.7 / -3.7 | +12.9 / +6.2 / -3.8 / -3.8 | +15.6 / +6.2 / -3.8 / -3.8 | +15.4 / +6.2 / -3.8 / -3.8 | +13.9 / +13.9 / +13.9 / +13.9 | +0.8 / +0.8 / -1.4 / -1.4 | -11.5 / -11.5 / -12.4 / -12.4 | +0.3 / +0.3 / -9.5 / -9.5 | +9.4 / +0.4 / -9.6 / -9.6 | +10.4 / +0.4 / -9.6 / -9.6 |
| Amateur spurious emissions, dB (47 CFR 97.307(e)) | +14.2 / +4.2 / +0.2 / +0.2 | +14.2 / +4.2 / +0.2 / +0.2 | +14.2 / +4.2 / +0.2 / +0.2 | +14.2 / +4.2 / +0.2 / +0.2 | +49.4 / +39.4 / +35.4 / +35.4 | +8.9 / -1.1 / -5.1 / -5.1 | +6.6 / -3.4 / -7.4 / -7.4 | +7.8 / -2.2 / -6.2 / -6.2 | +7.8 / -2.2 / -6.1 / -6.1 | +7.8 / -2.2 / -6.1 / -6.1 |
| Amateur spurious emissions, dB (ERC/REC 74-01) | +26.8 / +16.8 / +6.8 / -0.2 | +26.8 / +16.8 / +6.8 / -0.2 | +26.8 / +16.8 / +6.8 / -0.2 | +26.8 / +16.8 / +6.8 / -0.2 | - | - | +19.9 / +9.9 / -0.1 / -7.1 | +20.4 / +10.4 / +0.4 / -6.6 | +20.4 / +10.4 / +0.4 / -6.6 | +20.4 / +10.4 / +0.4 / -6.6 |
| Alternate channel power, dB (China) | +13.0 | +25.4 | +35.6 | +39.7 | +35.3 | -9.6 | -6.6 | +4.8 | +15.0 | +19.2 |
| Occupied bandwidth, kHz under 8.5 kHz (China) | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 | +0.8 |
| Adjacent channel power, dB (Japan) | +13.9 | +18.5 | +19.3 | +19.3 | +22.2 | +8.4 | -4.5 | +7.2 | +16.0 | +18.0 |
| Occupied bandwidth, kHz under 11.5 kHz (Japan) | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 | +3.8 |

### Tier 1 margins

| Limit | Audio 8-bit | Audio 10-bit | Audio 12-bit | Audio 16-bit | I/Q 16-bit | I/Q 8-bit |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Deviation error, % under the 10% limit (TS 102 361-1 10.2.2.3) | +9.2 | +9.7 | +9.9 | +9.9 | +9.9 | +9.9 |
| Adjacent channel power, dB (EN 300 113 7.4) | +2.6 | +6.7 | +7.1 | +7.1 | +4.8 | +4.2 |
| Alternate channel power, dB (EN 300 113 7.4) | +24.5 | +29.3 | +29.9 | +30.0 | +30.0 | +12.4 |
| Transient adjacent channel power, dB (EN 300 113 7.9) | - | - | - | - | +26.9 | +15.2 |
| Transient alternate channel power, dB (China) | - | - | - | - | +27.0 | +4.5 |
| Attack time t_p, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 |
| Release time t_d, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 |
| Transient power slope, dB (EN 300 113 7.9) | - | - | - | - | -0.0 | -0.0 |
| Transient frequency error, kHz (EN 300 113 7.9) | - | - | - | - | +6.2 | +6.2 |
| Spurious emissions, dB (EN 300 113 7.5) | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +38.6 / +28.6 / +18.6 / +8.6 | -4.4 / -14.4 / -24.4 / -34.4 |
| Emission Mask D, dB (47 CFR 90.210(d)) | +4.8 / +4.8 / -3.7 / -3.7 | +13.5 / +6.3 / -3.7 / -3.7 | +14.8 / +6.3 / -3.7 / -3.7 | +15.4 / +6.3 / -3.7 / -3.7 | +14.6 / +14.6 / +14.6 / +14.6 | +11.7 / +1.6 / -8.3 / -8.3 |
| Amateur spurious emissions, dB (47 CFR 97.307(e)) | +14.2 / +4.2 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +42.7 / +32.7 / +28.8 / +28.8 | +15.6 / +5.6 / +1.6 / +1.6 |
| Amateur spurious emissions, dB (ERC/REC 74-01) | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +61.6 / +51.6 / +41.6 / +34.6 | +18.6 / +8.6 / -1.4 / -8.4 |
| Alternate channel power, dB (China) | +16.8 | +28.6 | +39.2 | +43.6 | +49.0 | +2.4 |
| Occupied bandwidth, kHz under 8.5 kHz (China) | +0.8 | +0.8 | +0.8 | +0.8 | +0.7 | +0.7 |
| Adjacent channel power, dB (Japan) | +11.3 | +21.2 | +24.4 | +24.8 | +26.1 | +19.9 |
| Occupied bandwidth, kHz under 11.5 kHz (Japan) | +3.8 | +3.8 | +3.8 | +3.8 | +3.7 | +3.7 |

### Tier 2 margins

| Limit | Audio 8-bit | Audio 10-bit | Audio 12-bit | Audio 16-bit | I/Q 16-bit | I/Q 8-bit |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Deviation error, % under the 10% limit (TS 102 361-1 10.2.2.3) | +9.2 | +9.8 | +9.9 | +9.9 | +9.6 | +9.6 |
| Adjacent channel power, dB (EN 300 113 7.4) | +2.6 | +6.7 | +7.1 | +7.1 | +5.0 | +4.4 |
| Alternate channel power, dB (EN 300 113 7.4) | +24.5 | +29.3 | +29.9 | +30.0 | +30.0 | +12.2 |
| Transient adjacent channel power, dB (EN 300 113 7.9) | - | - | - | - | +26.9 | +15.2 |
| Transient alternate channel power, dB (China) | - | - | - | - | +27.0 | +4.5 |
| Attack time t_p, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 |
| Release time t_d, ms over 0.1 ms (EN 300 113 7.9) | - | - | - | - | +0.3 | +0.3 |
| Transient power slope, dB (EN 300 113 7.9) | - | - | - | - | -0.0 | -0.0 |
| Transient frequency error, kHz (EN 300 113 7.9) | - | - | - | - | +6.2 | +6.2 |
| Spurious emissions, dB (EN 300 113 7.5) | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +4.2 / -5.8 / -15.8 / -25.8 | +38.6 / +28.6 / +18.6 / +8.6 | -4.3 / -14.3 / -24.3 / -34.3 |
| Emission Mask D, dB (47 CFR 90.210(d)) | +4.8 / +4.8 / -3.7 / -3.7 | +13.5 / +6.3 / -3.7 / -3.7 | +14.8 / +6.3 / -3.7 / -3.7 | +15.4 / +6.3 / -3.7 / -3.7 | +14.7 / +14.7 / +14.7 / +14.7 | +11.7 / +1.7 / -8.3 / -8.3 |
| Amateur spurious emissions, dB (47 CFR 97.307(e)) | +14.2 / +4.2 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +14.3 / +4.3 / +0.3 / +0.3 | +42.8 / +32.8 / +28.8 / +28.8 | +15.7 / +5.7 / +1.7 / +1.7 |
| Amateur spurious emissions, dB (ERC/REC 74-01) | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +26.9 / +16.9 / +6.9 / -0.1 | +61.6 / +51.6 / +41.6 / +34.6 | +18.7 / +8.7 / -1.3 / -8.3 |
| Alternate channel power, dB (China) | +16.8 | +28.6 | +39.2 | +43.6 | +49.0 | +2.2 |
| Occupied bandwidth, kHz under 8.5 kHz (China) | +0.8 | +0.8 | +0.8 | +0.8 | +0.7 | +0.7 |
| Adjacent channel power, dB (Japan) | +11.3 | +21.2 | +24.4 | +24.8 | +26.1 | +19.8 |
| Occupied bandwidth, kHz under 11.5 kHz (Japan) | +3.8 | +3.8 | +3.8 | +3.8 | +3.7 | +3.7 |

<!-- txdoc:end -->
