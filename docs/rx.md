# Receiver

<!-- rfparams:rx-intro:begin -->
Package rf recovers DMR bursts from the I/Q samples of a software radio and from the output of an FM discriminator: 4FSK detection, SYNC search and slot timing, soft-decision decoding of each FEC field, and the superframe position of voice bursts. It also turns bursts back into I/Q samples and into the deviation signal of an FM radio.

NewReceiver makes a Receiver from an RxConfig, for I/Q at RxConfig.Rate in RxConfig.Format: CU8 as rtl_sdr writes it, CS8 as hackrf_transfer writes it, CS16 or CF32. NewAudioReceiver makes an AudioReceiver from an AudioRxConfig, for the samples of a discriminator. Init sets up either one in Tier0 in memory of the caller, for a microcontroller.

The callback of the config picks the mode. With OnBursts set, Write takes samples and hands the bursts that they complete to OnBursts, and Flush ends the stream. With OnBursts nil, AppendBursts appends the bursts to a slice, and AppendFlush ends the stream.

A Monitor follows the TDMA channels of the bursts, as a scanner does: it finds the TDMA channel of each burst from the CACHs of its stream, decodes it, and keeps the LC of the call on each channel. Burst.Hint gives the layer2.Hint of a burst for the voice and data machines.

OnShortLC of the config receives the Short LCs that the CACHs of the bursts carry. It decodes them as layer2.ShortLCAssembler does, and from Tier1 up decodes a Short LC that fails again from the soft values of its four fragments, with ShortLC.Soft set. The bursts are the same with it and without it.

The Tier of the config sets the detection and its cost, from Tier0, which fits a microcontroller, to Tier3, for offline decoding. Every tier follows a BS-sourced stream on both TDMA slots, and Tier0 to Tier2 also follow the MS-sourced bursts of both slots, each slot with its own SYNC timing, symbol clock, levels and carrier offset, as on the inbound of a repeater or a duplex hotspot with a radio on each slot. The constructors allocate every buffer of the receiver, and Write and Flush allocate nothing. docs/rx.md gives the sensitivity, compute and RAM of each tier, and docs/tx.md the limits and cost of the transmitter.
<!-- rfparams:rx-intro:end -->

[v2/cmd/dmrrf](../v2/cmd/dmrrf/README.md) decodes DMR from I/Q or discriminator audio in the file formats of common software radios, and writes test transmissions in the same formats.

## Features

<!-- rfparams:rx-features:begin -->
- `Receiver`: Receiver recovers DMR bursts from a stream of I/Q samples. It locks to the slot timing of the first SYNC it finds, follows the timing and the carrier through the transmission, and gives every burst it recovers. Make it with NewReceiver, or set it up with Init in memory of the caller. A Receiver is not safe for concurrent use, and it must not be copied after Init or NewReceiver.
- `AudioReceiver`: AudioReceiver recovers DMR bursts from the output of an FM discriminator, as the radio chips of hotspots and MMDVM modems give it: real samples in any units, with any gain and DC offset, which the receiver measures itself. It takes audio of either polarity. Above Tier0, an AudioReceiver from NewAudioReceiver outside TinyGo also takes audio that a high-pass filter cut below about 300 Hz, as the speaker output of a scanner gives it. With AudioRxConfig.PreEmphasis, every tier also takes the de-emphasized audio of the speaker or accessory output of a radio. With Tier0 it keeps about 1.4 KB of state, for microcontrollers; on a Cortex-M4F at 9.6 kHz it needs up to 5.0 MHz and 2,296 B with its stack, built with TinyGo -opt z, over 2 KB by the accepted cost of the detection of standalone RC bursts and of the MS bursts of both TDMA slots and of the clock rate tracking. An AudioReceiver that Init sets up in memory of the caller, such as a package variable, uses no heap in stream mode. An AudioReceiver is not safe for concurrent use, and it must not be copied after Init or NewAudioReceiver.
- `Monitor`: Monitor follows the TDMA channels of the bursts of one Receiver or AudioReceiver, as a scanner does. It finds the TDMA channel of each BS-sourced burst from the CACHs of its stream, decodes the burst, keeps the superframe position and the embedded LC of each channel, and keeps the LC of the call on each channel from its voice LC header, embedded LC and terminator. The zero Monitor is ready to use, and it does not allocate. A Monitor is not safe for concurrent use.
- Tiers:
  - `TierDefault`: TierDefault is the zero value: Tier1 for NewReceiver and NewAudioReceiver, and Tier0 for Init and under TinyGo.
  - `Tier0`: Tier0 slices the output of an FM discriminator and its matched filter. On I/Q from 48 kHz an FM discriminator at 12 kHz comes before it, and the receiver keeps about 19 KB. On I/Q at 9.6 or 12 kHz that a radio chip filtered to the channel it keeps about 1.4 KB, and Init sets it up in memory of the caller, for a microcontroller. On I/Q from 48 kHz Tier0 to Tier3 find a carrier up to 6.25 kHz off, half the channel spacing, from the spectrum before they lock; a carrier further off is taken for the next channel.
  - `Tier1`: Tier1 adds to Tier0 the soft decoders: a BPTC(196,96) block whose check fails by ordered statistics, C0 and C1 of each voice frame by soft ML, the embedded LC of each superframe over its four fragments and the earlier superframes of the call, and LookbackSlots slots before the first lock decoded again. On I/Q it keeps the input samples of those slots, and when it tuned to the carrier before the lock it filters them again at the new tune before it decodes them. It detects the bursts that they decode, and each rate 3/4 block, from the symbols of the discriminator with the correlation of their noise. It keeps about 35 KB on audio and 108 KB on I/Q, 58 KB of that for the input samples.
  - `Tier2`: Tier2 adds to the soft decoders of Tier1 the detection again of the bursts with a trellis over three symbols that carries the carrier phase with each survivor, from the SYNC or EMB of the burst, within a compute budget per slot: on I/Q from the samples at 24 kHz, on audio from the carrier phase that it sums from the discriminator samples. It holds each burst one slot longer and keeps about 550 KB on audio and 565 KB on I/Q, 75 KB of that for the input samples.
  - `Tier3`: Tier3 detects the bursts of Tier2 again by log-MAP over a window of three symbols with one of 52 carrier phases in the trellis state and no compute budget, and detects each voice burst once more with the bits that its FEC fixed. On I/Q it also filters the samples of the trellis to the channel width, fits the burst timing over slots and keeps 16 lookback slots; on audio it makes four passes over each data burst. It needs about 20 times the compute of Tier2 on I/Q and 10 times on audio, and keeps about 2.3 MB on I/Q and 1.3 MB on audio, for decoding on a PC.
- I/Q formats:
  - `IQFormatDefault`: IQFormatDefault is the zero value: IQFormatCS16.
  - `IQFormatCU8`: IQFormatCU8 is unsigned 8-bit, as rtl_sdr writes it.
  - `IQFormatCS8`: IQFormatCS8 is signed 8-bit, as hackrf_transfer writes it.
  - `IQFormatCS16`: IQFormatCS16 is signed 16-bit little-endian, as PlutoSDR, LimeSDR, USRP and the radio chips give it.
  - `IQFormatCF32`: IQFormatCF32 is little-endian float32, as GNU Radio gives it, with full scale at 1.
<!-- rfparams:rx-features:end -->

## Key specifications

<!-- rxbench:specs:begin -->
### Sensitivity

Eb/N0 at which the tier recovers 90% of the frames exactly. Lower is better. C/N in 12.5 kHz is 1.1 dB lower.

| Tier | Voice | Data rate 1/2 | Data rate 3/4 | CSBK | Embedded LC |
|------|-------|---------------|---------------|------|-------------|
| Best possible (4FSK floor) | -1.0 dB | -3.0 dB | 0.3 dB | -4.1 dB | < -6.9 dB |
| Audio tier 0 | 9.6 dB | 6.5 dB | 8.3 dB | 6.4 dB | 9.5 dB |
| Audio tier 1 | 8.2 dB | 6.4 dB | 7.9 dB | 5.4 dB | 4.3 dB |
| Audio tier 2 | 4.9 dB | 4.4 dB | 6.2 dB | 3.8 dB | 2.4 dB |
| Audio tier 3 | 4.4 dB | 3.5 dB | 5.7 dB | 2.7 dB | 2.4 dB |
| Low-rate I/Q tier 0 | 10.3 dB | 6.6 dB | 8.3 dB | 6.5 dB | 9.6 dB |
| I/Q tier 0 | 9.3 dB | 5.4 dB | 7.6 dB | 5.4 dB | 9.2 dB |
| I/Q tier 1 | 8.0 dB | 5.4 dB | 7.0 dB | 3.6 dB | 2.5 dB |
| I/Q tier 2 | 4.9 dB | 4.4 dB | 5.5 dB | 3.2 dB | 2.0 dB |
| I/Q tier 3 | 2.5 dB | 1.8 dB | 4.8 dB | 1.1 dB | 0.1 dB |

### Operating characteristics

| Tier | Input | Carrier offset | Sample clock error | Late entry at C/N 8 dB | Late entry at C/N 20 dB | CPU at C/N 6 dB | Worst-case CPU | RAM |
|------|-------|----------------|--------------------|------|------|------|------|-----|
| Audio tier 0 | FM audio, 9.6 kHz, core | -4.5 to 4 kHz | ±2,000 ppm | 59 ms | 62 ms | 0.34 ms | 0.41 ms | 1,696 B |
| Audio tier 1 | FM audio, 9.6 kHz, soft decoding, noise prediction | -4.5 to 4 kHz | ±2,000 ppm | 47 ms | 45 ms | 0.93 ms | 2.5 ms | 48 KiB |
| Audio tier 2 | FM audio, 9.6 kHz, coherent detection | ±5 kHz | -3,000 to 2,000 ppm | 46 ms | 45 ms | 8.6 ms | 13 ms | 589 KiB |
| Audio tier 3 | FM audio, 9.6 kHz, exact phase trellis, data passes | ±4.5 kHz | -3,000 to 2,000 ppm | 46 ms | 45 ms | 168 ms | 320 ms | 1.3 MiB |
| Low-rate I/Q tier 0 | I/Q, 9.6 kHz, discriminator core | ±2 kHz | ±2,000 ppm | 60 ms | 61 ms | 0.39 ms | 0.44 ms | 1,672 B |
| I/Q tier 0 | I/Q, discriminator core | ±6 kHz | ±2,000 ppm | 53 ms | 61 ms | 1.3 ms | 1.5 ms | 19 KiB |
| I/Q tier 1 | I/Q, soft decoding, noise prediction | ±6 kHz | -2,000 to 3,000 ppm | 44 ms | 44 ms | 1.7 ms | 3.5 ms | 106 KiB |
| I/Q tier 2 | I/Q, coherent detection | ±6 kHz | -3,000 to 2,000 ppm | 44 ms | 44 ms | 8.4 ms | 11 ms | 603 KiB |
| I/Q tier 3 | I/Q, exact phase trellis | ±6 kHz | ±3,000 ppm | 47 ms | 44 ms | 284 ms | 386 ms | 2.3 MiB |

- Carrier offset is the range around zero, on the grid of [its graph](#carrier-offset), in which the tier recovers at least 90% of the voice frames at C/N 15.0 dB. Sample clock error is the same on the grid of [its graph](#sample-clock-error), at C/N 15.0 dB. "At least" marks a range that reaches the ends of the grid.
- Late entry is the median time from the first sample to the end of the first burst that the tier recovers exactly, when the input starts at a random point of a call. A count gives the runs that recovered a burst within 1500 ms, when some did not.
- CPU is per second of signal, measured on an AMD Ryzen 9 7950X 16-Core Processor. Worst-case CPU is the highest over C/N 0 to 40 dB. RAM is the state of the receiver. No tier allocates after it starts.

### Tier 0 on a Cortex-M4F

| Tier | Rate | CPU | RAM with stack | Flash |
|------|------|-----|----------------|-------|
| Audio tier 0 | 9.6 kHz | 5.0 MHz | 2,296 B | 39.4 KB |
| Audio tier 0, `PreEmphasis` | 9.6 kHz | 5.3 MHz | 2,352 B | 39.6 KB |
| Low-rate I/Q tier 0 | 9.6 kHz | 5.3 MHz | 2,304 B | 39.9 KB |

Built with TinyGo 0.44.0 `-opt z -gc=none` and run in QEMU (`mps2-an386`, Cortex-M4F) by `cmd/mcubench`. CPU is the most, over the test signals, of the cycles that the receiver takes per second of signal. RAM is the static data of the program less the bench, with the most stack. Flash is the code and constant data of the whole program.

<!-- rxbench:specs:end -->

## Absolute limits and operating conditions

<!-- rfparams:rx-limits:begin -->
| Field | Rule |
|-------|------|
| `RxConfig.Tier` | Tier selects the detection. The zero value TierDefault is Tier1 for NewReceiver, and Tier0 for Init and under TinyGo. |
| `RxConfig.Rate` | Rate is the sample rate of the I/Q, in Hz. 0 means 2400000. Tier0 to Tier3 take a Rate from 48000 to 1e9, and Tier0 and Tier1 also one from 24000, which runs their receiver with a complex front end in place of the FM discriminator, with about 2 KB of state for Tier0. Tier0 also takes 9600 and 12000, for I/Q that a radio chip filtered to the channel, with about 1.4 KB of state; Init sets up only these. |
| `RxConfig.Format` | Format is the layout of the I/Q samples. 0 means IQFormatCS16. |
| `RxConfig.OffsetHz` | OffsetHz is the frequency of the carrier relative to the center of the I/Q, in Hz, when the oscillator of the radio is known to be off, as for an RTL-SDR without ppm correction (rtl_fm -p): the offset that OffsetHz of a Receiver reported for that radio. The receiver tunes there before it searches 6.25 kHz around it for the carrier. It must be under 24000 in size, or under Rate/2 for a Rate under 48000. 0 means a centered carrier. |
| `RxConfig.OnBursts` | OnBursts, when not nil, selects stream mode: Write and Flush hand the bursts to it. Each call carries the bursts that are ready at the same point of the stream, in storage of the receiver that is valid only during the call: usually one, and after a late lock the LookbackSlots bursts decoded again. When nil, the receiver is in append mode, for AppendBursts and AppendFlush. |
| `RxConfig.LookbackSlots` | LookbackSlots is the number of 30 ms slots before the first SYNC it locks to that the receiver keeps and decodes: 0 means 4 for Tier1, 8 for Tier2, and 16 for Tier3, and at most MaxLookbackSlots. Tier0 keeps none, and loses the start of a stream that it enters late. |
| `RxConfig.OnShortLC` | OnShortLC, when not nil, receives each Short LC that the CACH of the bursts carries and that passes its CRC-8, in stream and in append mode, in storage of the receiver that is valid only during the call: usually one. The receiver calls it as it outputs the burst of the last fragment, before that burst reaches OnBursts or the slice of AppendBursts. It decodes the fragments of the Decoded CACH as layer2.ShortLCAssembler does, and from Tier1 up decodes those that fail again from their soft values. Tier0 gives the hard decode only. Init and TinyGo builds take no OnShortLC and return a *dmrgo.ConfigError for it. |
| `AudioRxConfig.Tier` | Tier is Tier0, Tier1, Tier2 or Tier3. The zero value TierDefault is Tier1 for NewAudioReceiver, and Tier0 for Init, which does not allocate the stages of the higher tiers, and under TinyGo, which has neither. Tier2 detects the bursts again coherently, on the carrier phase that it rebuilds from the samples, at about 5 times the CPU time of Tier1. Tier3 detects the bursts of Tier2 again over 52 carrier phases with no compute budget and makes four passes over each data burst, at about 10 times the CPU time of Tier2, 1.3 MB and the delay of Tier2. |
| `AudioRxConfig.Rate` | Rate is the sample rate of the discriminator samples, in Hz, from 8000 to 1000000, and for Tier0 to 48000. 0 means 48000. The receiver runs at 48000, as sound cards sample, 24000, as MMDVM modems sample, 12000 or 9600: at Rate when it is one of them, else at the lowest of them at or above Rate, or 48000 above 48000, through a resampler. Tier0, also under TinyGo, resamples in fixed point with a table in flash. A lower rate costs less: the receiver halves 48 kHz and then 24 kHz with a half-band filter before its matched filter, which runs at 12 kHz or 9.6 kHz. |
| `AudioRxConfig.Bits` | Bits is the resolution of the converter, from 8 to 16. 0 means 16. Each sample holds one code of Bits bits in its low bits. |
| `AudioRxConfig.Unsigned` | Unsigned selects offset binary codes, from 0 to 1<<Bits-1, in place of two's complement codes. |
| `AudioRxConfig.OnBursts` | OnBursts, when not nil, selects stream mode: Write and Flush hand the bursts to it. Each call carries the bursts that are ready at the same point of the stream, in storage of the receiver that is valid only during the call: usually one, and after a late lock the bursts of the slots before the lock, decoded again. When nil, the receiver is in append mode, for AppendBursts and AppendFlush. |
| `AudioRxConfig.PreEmphasis` | PreEmphasis, when not nil, states that the radio de-emphasizes its discriminator output from 300 Hz to 3 kHz, as the speaker and accessory outputs of FM radios do, so that the receiver inverts it with the filter that it holds before it detects the bursts: a RxPreEmphasis of this receiver alone, set up by Init or NewRxPreEmphasis. Nil is a flat discriminator output, as the radio chips of hotspots and MMDVM modems give it. Every Tier and Rate takes it. A RxPreEmphasis that is not set up gives a *dmrgo.ConfigError. |
| `AudioRxConfig.OnShortLC` | OnShortLC, when not nil, receives each Short LC that the CACH of the bursts carries and that passes its CRC-8, in stream and in append mode, in storage of the receiver that is valid only during the call: usually one. The receiver calls it as it outputs the burst of the last fragment, before that burst reaches OnBursts or the slice of AppendBursts. It decodes the fragments of the Decoded CACH as layer2.ShortLCAssembler does, and from Tier1 up decodes those that fail again from their soft values. Tier0 gives the hard decode only. Init and TinyGo builds take no OnShortLC and return a *dmrgo.ConfigError for it. |

Probes of the constructors: `AudioRxConfig.Bits` from 8 to 16 is accepted, and `RxConfig.OffsetHz` up to 24 kHz in size at a `Rate` of 48 kHz. A field out of range gets a `*dmrgo.ConfigError` with one of these values of `Field`: `AudioRxConfig.Bits`, `AudioRxConfig.OnBursts`, `AudioRxConfig.OnShortLC`, `AudioRxConfig.PreEmphasis`, `AudioRxConfig.Rate`, `AudioRxConfig.Tier`, `RxConfig.Format`, `RxConfig.LookbackSlots`, `RxConfig.OffsetHz`, `RxConfig.OnBursts`, `RxConfig.OnShortLC`, `RxConfig.Rate`, `RxConfig.Tier`.
<!-- rfparams:rx-limits:end -->

<!-- rxbench:limits:begin -->
| Tier | I/Q rates accepted | Audio rates accepted |
|------|------|------|
| Tier 0 | 9,600, 12,000, 24,000 to 1,000,000,000 Hz | 8,000 to 48,000 Hz |
| Tier 1 | 24,000 to 1,000,000,000 Hz | 8,000 to 1,000,000 Hz |
| Tier 2 | 48,000 to 1,000,000,000 Hz | 8,000 to 1,000,000 Hz |
| Tier 3 | 48,000 to 1,000,000,000 Hz | 8,000 to 1,000,000 Hz |

A span gives the first and last rate of the probe that the constructor accepts in a row. It returns an error for the other rates of the probe. The probe tries 9,599, 9,600, 9,601, 11,999, 12,000, 12,001, 23,999, 24,000, 47,999, 48,000, 96,000, 250,000, 2,400,000, 20,000,000, 1,000,000,000, 1,000,000,001 Hz for I/Q and 7,999, 8,000, 9,600, 11,025, 12,000, 22,050, 24,000, 44,100, 48,000, 48,001, 96,000, 1,000,000, 1,000,001 Hz for audio.

<!-- rxbench:limits:end -->

## Typical performance characteristics

[Test conditions](#test-conditions) gives the method of each graph and table.

<!-- rfparams:rx-eye:begin -->
Each eye diagram shows the receiver input after the DMR matched filter (root raised cosine, roll-off 0.2), at 48 kHz, folded on `Burst.Seconds` of each burst that the tier recovered. The dotted lines are the four symbol levels.
<!-- rfparams:rx-eye:end -->

<!-- rxbench:begin -->
### Error rate, FM discriminator audio

Error rate against Eb/N0 for each tier, with the 4FSK floor dashed. Points with no errors are left out.

![Voice frame, FM discriminator audio: frame error rate against Eb/N0, references and tiers](rx/audio/ambe.svg)
![Data rate 1/2, FM discriminator audio: frame error rate against Eb/N0, references and tiers](rx/audio/rate12.svg)
![Data rate 3/4, FM discriminator audio: frame error rate against Eb/N0, references and tiers](rx/audio/rate34.svg)
![CSBK, FM discriminator audio: frame error rate against Eb/N0, references and tiers](rx/audio/csbk.svg)
![Embedded LC, FM discriminator audio: frame error rate against Eb/N0, references and tiers](rx/audio/elc.svg)
![Data rate 1 payload bits, FM discriminator audio: bit error rate against Eb/N0, references and tiers](rx/audio/rate1_bit.svg)

### Error rate, I/Q at 9.6 kHz

Error rate against Eb/N0 for each tier, with the 4FSK floor dashed. Points with no errors are left out.

![Voice frame, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](rx/iqlow/ambe.svg)
![Data rate 1/2, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](rx/iqlow/rate12.svg)
![Data rate 3/4, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](rx/iqlow/rate34.svg)
![CSBK, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](rx/iqlow/csbk.svg)
![Embedded LC, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](rx/iqlow/elc.svg)
![Data rate 1 payload bits, I/Q at 9.6 kHz: bit error rate against Eb/N0, references and tiers](rx/iqlow/rate1_bit.svg)

### Error rate, I/Q

Error rate against Eb/N0 for each tier, with the 4FSK floor dashed. Points with no errors are left out.

![Voice frame, I/Q: frame error rate against Eb/N0, references and tiers](rx/iq/ambe.svg)
![Data rate 1/2, I/Q: frame error rate against Eb/N0, references and tiers](rx/iq/rate12.svg)
![Data rate 3/4, I/Q: frame error rate against Eb/N0, references and tiers](rx/iq/rate34.svg)
![CSBK, I/Q: frame error rate against Eb/N0, references and tiers](rx/iq/csbk.svg)
![Embedded LC, I/Q: frame error rate against Eb/N0, references and tiers](rx/iq/elc.svg)
![Data rate 1 payload bits, I/Q: bit error rate against Eb/N0, references and tiers](rx/iq/rate1_bit.svg)

### Eye diagrams

On a clean signal, the symbol centers of every tier fall on `Burst.Seconds` plus whole symbols, within the search step of 0.01 symbol (2.1 µs).

![Audio tier 0, FM discriminator audio: eye diagrams at three C/N](rx/audio/eye_tier0.svg)
![Audio tier 1, FM discriminator audio: eye diagrams at three C/N](rx/audio/eye_tier1.svg)
![Audio tier 2, FM discriminator audio: eye diagrams at three C/N](rx/audio/eye_tier2.svg)
![Audio tier 3, FM discriminator audio: eye diagrams at three C/N](rx/audio/eye_tier3.svg)
![Low-rate I/Q tier 0, I/Q at 9.6 kHz: eye diagrams at three C/N](rx/iqlow/eye_tier0.svg)
![I/Q tier 0, I/Q: eye diagrams at three C/N](rx/iq/eye_tier0.svg)
![I/Q tier 1, I/Q: eye diagrams at three C/N](rx/iq/eye_tier1.svg)
![I/Q tier 2, I/Q: eye diagrams at three C/N](rx/iq/eye_tier2.svg)
![I/Q tier 3, I/Q: eye diagrams at three C/N](rx/iq/eye_tier3.svg)

### Carrier offset

![FM discriminator audio: voice frames recovered against carrier offset](rx/audio/offset.svg)
![I/Q at 9.6 kHz: voice frames recovered against carrier offset](rx/iqlow/offset.svg)
![I/Q: voice frames recovered against carrier offset](rx/iq/offset.svg)

### Sample clock error

![FM discriminator audio: voice frames recovered against sample clock error](rx/audio/clock.svg)
![I/Q at 9.6 kHz: voice frames recovered against sample clock error](rx/iqlow/clock.svg)
![I/Q: voice frames recovered against sample clock error](rx/iq/clock.svg)

### Late entry

![FM discriminator audio: late entry time against C/N](rx/audio/late_entry.svg)
![I/Q at 9.6 kHz: late entry time against C/N](rx/iqlow/late_entry.svg)
![I/Q: late entry time against C/N](rx/iq/late_entry.svg)

### Sensitivity per payload type

Eb/N0 in dB for 90% recovery. [The floors](#the-floors) defines each floor.

| Payload | 4FSK floor | DMR floor | Code cost | Code-aware |
|---------|------------|-----------|-----------|------------|
| Voice frame (AMBE+2, 49 bits) | -1.0 dB | at least 1.3 dB | 2.3 to 2.9 | 1.9 dB |
| Voice frame, protected bits (C0 and C1) | -5.3 dB | at least -4.9 dB | 0.4 to 4.6 | -0.7 dB |
| Data block, rate 1/2 | -3.0 dB | - | at most 3.5 | 0.5 dB |
| Data block, rate 3/4 | 0.3 dB | at least 1.3 dB | 0.9 to 4.3 | 4.6 dB |
| Data block, rate 1 | 5.3 dB | 6.5 dB | 1.2 | 6.6 dB |
| CSBK | -4.1 dB | - | at most 4.5 | 0.3 dB |
| Voice LC header and terminator | -4.3 dB | - | at most 5.0 | 0.7 dB |
| Data header | -3.9 dB | - | at most 4.4 | 0.5 dB |
| Embedded LC | < -6.9 dB | - | - | -2.0 dB |
| EMB | < -6.9 dB | < -6.9 dB | - | -3.4 dB |
| Slot type | < -6.9 dB | < -6.9 dB | - | -2.5 dB |
| SYNC detection | < -6.9 dB | < -6.9 dB | - | < -6.9 dB |

| Payload | Audio tier 0 | Audio tier 1 | Audio tier 2 | Audio tier 3 | Low-rate I/Q tier 0 | I/Q tier 0 | I/Q tier 1 | I/Q tier 2 | I/Q tier 3 |
|---------|------|------|------|------|------|------|------|------|------|
| Voice frame (AMBE+2, 49 bits) | 9.6 dB | 8.2 dB | 4.9 dB | 4.4 dB | 10.3 dB | 9.3 dB | 8.0 dB | 4.9 dB | 2.5 dB |
| Voice frame, protected bits (C0 and C1) | 5.9 dB | 5.2 dB | 3.9 dB | 3.2 dB | 5.4 dB | 4.6 dB | 3.7 dB | 3.1 dB | 1.6 dB |
| Data block, rate 1/2 | 6.5 dB | 6.4 dB | 4.4 dB | 3.5 dB | 6.6 dB | 5.4 dB | 5.4 dB | 4.4 dB | 1.8 dB |
| Data block, rate 3/4 | 8.3 dB | 7.9 dB | 6.2 dB | 5.7 dB | 8.3 dB | 7.6 dB | 7.0 dB | 5.5 dB | 4.8 dB |
| Data block, rate 1 | 11.3 dB | 11.3 dB | 8.3 dB | 8.0 dB | 12.4 dB | 11.1 dB | 11.3 dB | 7.8 dB | 7.6 dB |
| CSBK | 6.4 dB | 5.4 dB | 3.8 dB | 2.7 dB | 6.5 dB | 5.4 dB | 3.6 dB | 3.2 dB | 1.1 dB |
| Voice LC header and terminator | 6.4 dB | 5.5 dB | 4.3 dB | 3.3 dB | 6.5 dB | 5.7 dB | 4.1 dB | 3.4 dB | 1.1 dB |
| Data header | 6.4 dB | 5.1 dB | 3.9 dB | 2.8 dB | 6.5 dB | 5.5 dB | 3.8 dB | 3.3 dB | 1.1 dB |
| Embedded LC | 9.5 dB | 4.3 dB | 2.4 dB | 2.4 dB | 9.6 dB | 9.2 dB | 2.5 dB | 2.0 dB | 0.1 dB |
| EMB | 3.9 dB | 3.8 dB | 2.0 dB | 1.4 dB | 2.7 dB | 2.4 dB | 1.9 dB | 1.0 dB | -0.1 dB |
| Slot type | 4.0 dB | 4.0 dB | 1.3 dB | 1.5 dB | 3.1 dB | 2.7 dB | 2.5 dB | 0.7 dB | -0.2 dB |
| SYNC detection | 3.7 dB | 3.6 dB | 0.8 dB | 0.9 dB | 2.6 dB | 2.2 dB | 2.0 dB | -0.7 dB | -1.5 dB |

### Real recordings

Voice frames recovered exactly from captures of a Motorola XPR 8400 repeater with an RTL-SDR.

| Recording | C/N | Frames | I/Q tier 0 | I/Q tier 1 | I/Q tier 2 | I/Q tier 3 |
|-----------|-----|--------|------|------|------|------|
| MS voice relayed, HackRF on the uplink | 9.3 dB | 2013 | 1474 | 1520 | 1828 | 1865 |
| Late entry voice relayed, HackRF on the uplink | 11.1 dB | 1890 | 1463 | 1499 | 1622 | 1632 |
| Voice with PI header relayed, HackRF on the uplink | 17.3 dB | 1818 | 1818 | 1818 | 1818 | 1818 |
| Mixed voice and data, network to air, run 1 | 9.7 dB | 144 | 114 | 119 | 122 | 126 |
| Mixed voice and data, network to air, run 2 | 10.6 dB | 144 | 117 | 121 | 125 | 126 |
| Voice, network to air | 10.0 dB | 90 | 88 | 90 | 90 | 90 |
| Voice, network to air | 11.6 dB | 90 | 82 | 82 | 90 | 90 |
| Gain sweep, RTL gain 0 dB | 9.9 dB | 51 | 41 | 47 | 50 | 50 |
| Gain sweep, RTL gain 9.7 dB | 7.3 dB | 51 | 33 | 44 | 48 | 48 |
| Gain sweep, RTL gain 19.7 dB | 10.4 dB | 51 | 50 | 51 | 51 | 51 |
| Gain sweep, RTL gain 29.7 dB | 11.2 dB | 51 | 44 | 50 | 50 | 50 |

<!-- rxbench:end -->

## Application information

### Choosing a tier

[Key specifications](#key-specifications) gives the sensitivity, CPU and RAM of each tier, and [Features](#features) what each tier does. Pick the lowest tier whose sensitivity meets the link budget, on the hardware that runs it.

### Wiring to a radio

- Audio tiers take FM discriminator audio, as from a radio, of either polarity. The discriminator output of a radio, or its flat data port, has the low frequencies of the signal and no de-emphasis. It is the best input for `AudioReceiver`.
- The speaker and accessory outputs of a radio de-emphasize the audio. [De-emphasized audio](#de-emphasized-audio) gives the setup for them.
- I/Q tiers take the samples of a software radio, and low-rate I/Q tier 0 takes I/Q at 9.6 or 12 kHz that a radio chip filtered to the channel. When the oscillator of a software radio is off, set `RxConfig.OffsetHz` to its known offset.

### Levels and calibration

Set the gain of the sound card so that the audio of the strongest signal does not clip, and keep the I/Q samples of CU8 and CS8 from clipping. These methods give what the receiver measured:

<!-- rfparams:rx-levels:begin -->
- `AudioReceiver.Level`: Level returns the outer level of the last locked stream: the code of a run of symbol +3 (1944 Hz) less the DC code.
- `AudioReceiver.Offset`: Offset returns the DC code of the last locked stream: the code of the carrier, from its offset and the offset of the converter.
- `Receiver.OffsetHz`: OffsetHz returns the carrier offset of the last stream that ended, in Hz. Tier2 and Tier3 return the carrier of their coherent fit, which also follows the stream in progress.
- `Receiver.DeviationHz`: DeviationHz returns the outer deviation of the last locked stream, in Hz. Every tier reads it from the sync symbol levels and divides out the response of its channel filter and discriminator at the carrier offset from the tuned frequency. For a clean 1944 Hz signal the reading is 1 to 1.5% low and within 3% at every offset. The lookup uses the carrier offset of the samples of each sync fit, so a retune after a fit does not change it. A sync fit made before the timing settles can move the reading by a few percent more. At a Rate of 9600 the reading is up to 3% low, by the sub-sample phase of the bursts, and it does not include the channel filter of the radio chip.
<!-- rfparams:rx-levels:end -->

### Low-rate I/Q carrier range

The carrier of low-rate I/Q tier 0 must be less than 2.5 kHz from the center: the 9.6 kHz I/Q holds 4.8 kHz on each side, and at 2.5 kHz off 9.8 dB of the signal power, at 4 kHz 3.7 dB, falls outside the channel filter of the chip. On carriers 2.4 to 5 kHz off, tier 0 loses 28 cells of the comparison with the external receivers. 6 of them no receiver can win: a genie receiver given every parameter, which decodes C0 and C1 jointly gets at most 0.85 to 0.894 of the AMBE frames exact (upper 95% bound, against the bar of 0.90) at 4.8, +5 and -5 kHz, MS and BS. The other 22 need a coherent receiver. One that was measured needed about 5 KB of RAM, 42 KB of flash and 27 to 114 MHz on the Cortex-M4F and still won only 5 of the 12 cells at 2.4 and ±3 kHz, so tier 0 does not take that cost and these 22 cells are accepted as lost: 16 at carriers 2.4 to 4.8 kHz off, and 6 that an exact genie wins with 0.996 to 1.000 of the frames: the frames with C0 and C1 right at +5 and -5 kHz, and the exact frames at 4 kHz, MS and BS.

### Accepted Tier 0 costs

Tier 0 needs more than 2 KB of RAM with its stack on a Cortex-M4F; [Tier 0 on a Cortex-M4F](#tier-0-on-a-cortex-m4f) gives the measured cost. These parts of Tier 0 were accepted at their cost when they were added: the detection of standalone RC bursts, the MS bursts of both TDMA slots with their own timing and levels, the burst-edge gain hold, the CACH replay, the clock rate tracking, the I/Q gain cap, the lookback replay checks, and `PreEmphasis` on audio tier 0.

### De-emphasized audio

The speaker and accessory outputs of an FM radio de-emphasize the discriminator audio from 300 Hz to 3 kHz. That tilts the 4FSK levels by up to 20 dB, and no audio tier decodes it. `AudioRxConfig.PreEmphasis` takes a `RxPreEmphasis`, the receive twin of the transmit `PreEmphasis`: a first-order filter that inverts the de-emphasis before the receiver detects the bursts. Set one up with `Init` in memory of the caller, or with `NewRxPreEmphasis`, one for each receiver. Nil is a flat discriminator output, as the radio chips of hotspots and MMDVM modems give it, and the receiver is then the same, bit for bit, as without the field. Every tier and rate takes it. A program that sets up no `RxPreEmphasis` links none of its filter.

The sweep job field `radio: "deemph"` gives the audio tiers the audio of a radio that de-emphasizes it from 300 Hz to 3 kHz, and `pre_emphasis: true` sets up a `RxPreEmphasis`. Frames recovered exactly, 4 streams per point, C/N in 12.5 kHz:

| Tier | Rate | C/N | Voice, flat | Voice, de-emphasized | Voice, de-emphasized with `PreEmphasis` | Rate 3/4, flat | Rate 3/4, de-emphasized | Rate 3/4, de-emphasized with `PreEmphasis` |
|------|------|-----|------|------|------|------|------|------|
| 0 | 9.6 kHz | 6 dB | 214/432 | 0/432 | 193/432 | 18/24 | 0/24 | 17/24 |
| 0 | 9.6 kHz | 8 dB | 369/432 | 0/432 | 351/432 | 21/24 | 0/24 | 20/24 |
| 0 | 9.6 kHz | 10 dB | 426/432 | 0/432 | 415/432 | 24/24 | 0/24 | 24/24 |
| 1 | 9.6 kHz | 6 dB | 324/432 | 0/432 | 307/432 | 19/24 | 0/24 | 18/24 |
| 1 | 9.6 kHz | 8 dB | 413/432 | 0/432 | 411/432 | 23/24 | 0/24 | 23/24 |
| 1 | 9.6 kHz | 10 dB | 426/432 | 0/432 | 416/432 | 24/24 | 0/24 | 24/24 |
| 2 | 9.6 kHz | 6 dB | 428/432 | 0/432 | 420/432 | 23/24 | 0/24 | 22/24 |
| 2 | 9.6 kHz | 8 dB | 430/432 | 0/432 | 430/432 | 24/24 | 0/24 | 24/24 |
| 2 | 9.6 kHz | 10 dB | 432/432 | 0/432 | 429/432 | 24/24 | 0/24 | 24/24 |
| 3 | 9.6 kHz | 6 dB | 428/432 | 0/432 | 420/432 | 23/24 | 0/24 | 22/24 |
| 3 | 9.6 kHz | 8 dB | 430/432 | 0/432 | 430/432 | 24/24 | 0/24 | 24/24 |
| 3 | 9.6 kHz | 10 dB | 432/432 | 0/432 | 429/432 | 24/24 | 0/24 | 24/24 |
| 0 | 48 kHz | 6 dB | 228/432 | 0/432 | 226/432 | 18/24 | 0/24 | 19/24 |
| 0 | 48 kHz | 8 dB | 366/432 | 0/432 | 366/432 | 21/24 | 0/24 | 21/24 |
| 0 | 48 kHz | 10 dB | 425/432 | 0/432 | 425/432 | 24/24 | 0/24 | 24/24 |
| 1 | 48 kHz | 6 dB | 320/432 | 0/432 | 313/432 | 19/24 | 0/24 | 18/24 |
| 1 | 48 kHz | 8 dB | 409/432 | 0/432 | 409/432 | 23/24 | 0/24 | 23/24 |
| 1 | 48 kHz | 10 dB | 425/432 | 0/432 | 425/432 | 24/24 | 0/24 | 24/24 |
| 2 | 48 kHz | 6 dB | 429/432 | 0/432 | 427/432 | 23/24 | 0/24 | 23/24 |
| 2 | 48 kHz | 8 dB | 431/432 | 0/432 | 431/432 | 24/24 | 0/24 | 24/24 |
| 2 | 48 kHz | 10 dB | 432/432 | 0/432 | 432/432 | 24/24 | 0/24 | 24/24 |
| 3 | 48 kHz | 6 dB | 429/432 | 0/432 | 427/432 | 23/24 | 0/24 | 23/24 |
| 3 | 48 kHz | 8 dB | 431/432 | 0/432 | 431/432 | 24/24 | 0/24 | 24/24 |
| 3 | 48 kHz | 10 dB | 432/432 | 0/432 | 432/432 | 24/24 | 0/24 | 24/24 |

- The filter runs at 9.6, 12 or 24 kHz, the rate of the receiver after its half-band filters. With the de-emphasis it is flat within 0.1 dB to 2.4 kHz at 24 kHz, 0.4 dB at 12 kHz and 0.7 dB at 9.6 kHz. A first-order filter cannot follow the 3 kHz corner so close to the Nyquist rate of 9.6 kHz, so at 9.6 kHz the tiers get most of the loss back but not all of it.
- [Tier 0 on a Cortex-M4F](#tier-0-on-a-cortex-m4f) gives the cost of audio tier 0 with `PreEmphasis`. The option is off by default.

### Short LC

The CACH of BS-sourced bursts carries the Short LC in four fragments. `OnShortLC` of `RxConfig` and `AudioRxConfig` receives each Short LC that passes its CRC-8, with the stream, slot and time of the burst of the last fragment. The receiver decodes the fragments of the `Decoded` CACH as `layer2.ShortLCAssembler` does. From tier 1 up, when that decode fails, it decodes the Short LC again from the soft values of the four fragments, and sets `ShortLC.Soft`. It accepts a soft decode only when the decode flips few bits and its soft cost is low. I/Q tier 3 takes a lower soft cost limit than the other tiers, because at the limit of the other tiers its soft decodes pass the CRC-8 with wrong bits more often than its hard decodes do. Tier 0 gives the hard decode only, and `Init` and TinyGo builds take no `OnShortLC`. The bursts, `FixedCACH` and `layer2.ShortLCAssembler` are the same with it and without it.

<!-- shortlc:begin -->
| Tier | Input | Hard decode | `OnShortLC` | Wrong, hard decode | Wrong, `OnShortLC` | On noise, hard decode | On noise, `OnShortLC` |
|------|-------|-------------|-------------|--------------------|--------------------|-----------------------|-----------------------|
| Audio tier 0 | FM audio, 9.6 kHz, core | 7.6 dB | 7.6 dB | 0 of 3,168 | 0 of 3,168 | 0.0 per hour | 0.0 per hour |
| Audio tier 1 | FM audio, 9.6 kHz, soft decoding, noise prediction | 7.6 dB | 6.5 dB | 0 of 2,801 | 0 of 3,619 | 0.0 per hour | 0.0 per hour |
| Audio tier 2 | FM audio, 9.6 kHz, coherent detection | 7.5 dB | 6.3 dB | 0 of 2,552 | 1 of 3,382 | 0.0 per hour | 0.0 per hour |
| Audio tier 3 | FM audio, 9.6 kHz, exact phase trellis, data passes | 5.5 dB | 4.5 dB | 0 of 2,872 | 2 of 3,617 | 0.0 per hour | 0.0 per hour |
| Low-rate I/Q tier 0 | I/Q, 9.6 kHz, discriminator core | 7.9 dB | 7.9 dB | 0 of 5,867 | 0 of 5,867 | 0.0 per hour | 0.0 per hour |
| I/Q tier 0 | I/Q, discriminator core | 7.0 dB | 7.0 dB | 0 of 5,637 | 0 of 5,637 | 0.0 per hour | 0.0 per hour |
| I/Q tier 1 | I/Q, soft decoding, noise prediction | 7.0 dB | 5.2 dB | 0 of 3,415 | 0 of 4,660 | 0.0 per hour | 0.0 per hour |
| I/Q tier 2 | I/Q, coherent detection | 7.1 dB | 5.3 dB | 0 of 3,096 | 0 of 4,297 | 0.0 per hour | 0.0 per hour |
| I/Q tier 3 | I/Q, exact phase trellis | 4.2 dB | 2.7 dB | 0 of 3,676 | 2 of 4,515 | 0.0 per hour | 0.0 per hour |

- Hard decode and `OnShortLC` are the Eb/N0 at which 90% of the Short LCs of the sweep stream are right, from 4 streams per point. Hard decode is `layer2.ShortLCAssembler` on the `Decoded` CACH of the bursts.
- Wrong counts the Short LCs that pass the CRC-8 with other bits than the stream sent, of all that pass. On noise gives the Short LCs that the receivers report on white Gaussian noise alone.

The Motorola XPR 8400 idle capture (`motorola-xpr8400-idle-424075000-2048k.cu8`) with noise added to its CU8 codes. Short LCs, hard decode / `OnShortLC` (of them soft):

| Noise sigma | I/Q tier 0 | I/Q tier 1 | I/Q tier 2 | I/Q tier 3 |
|-------------|------|------|------|------|
| 0 | 46 / 46 (0) | 46 / 46 (0) | 46 / 46 (0) | 46 / 46 (0) |
| 16 | 17 / 17 (0) | 18 / 38 (20) | 20 / 42 (22) | 44 / 46 (2) |
| 20 | 0 / 0 (0) | 0 / 9 (9) | 0 / 2 (2) | 25 / 42 (17) |
| 24 | 0 / 0 (0) | 1 / 1 (0) | 0 / 0 (0) | 7 / 17 (10) |

<!-- shortlc:end -->

`TestShortLCBench` in `v2/rf` writes `v2/rf/testdata/rxbench_shortlc.json` with `DMRRF_SLCBENCH` set to the path of the capture, and `go run -C cmd/rxbench . -update-readme -no-sweep` writes the tables.

### A carrier off the tuned frequency

I/Q tier 0 runs at 12 kHz. A carrier more than 2.5 kHz off is outside its AFC range and partly outside its channel filter, so the receiver must retune before it can lock. When the input power rises, the spectrum hunt estimates the carrier from the next three FFT frames, about 8 ms, and retunes. Tier 0 keeps no samples to decode again, so the symbols before the retune are lost, and at 4 kHz the first burst comes out with its first symbols wrong.

The AFC of I/Q tiers 0 to 2 takes the angle of the lag-1 product of the channel filter output. This is not an unbiased frequency estimate: for a carrier off the oscillator, the filter passes less of the far side of the FM spectrum, so the estimate reads closer to the oscillator. With no noise, before the lock, it reads a carrier 1400 Hz off as about 740 Hz at Tier 0, 1040 Hz at Tier 1 and 830 Hz at Tier 2. Noise in the filter moves it a further 80 to 100 Hz toward the oscillator at 6 dB C/N. The spectrum hunt corrects this: before the lock it also retunes to an estimate from 12 or more FFT frames, about 32 ms, that is 300 Hz or more from the oscillator. That estimate weights each bin by its power over the noise floor, so the filter shape does not bias it. Its spread comes from the symbols of the burst: about 130 Hz over 32 ms and 260 Hz over 8 ms. After the lock, the AFC tunes to the frequency offset of the discriminator when the lag-1 estimate agrees with it within 300 Hz or within half the offset, since the lag-1 estimate reads short of the offset. Tiers 1 and 2 do not use the fast estimate after a power rise, so after a noise lead they can lock with the oscillator at the old frequency, and the locked AFC then follows a carrier 1.4 kHz off within 200 ms.

An I/Q DC offset, as of an RTL-SDR, puts a spur at 0 Hz. The DC block of the front end removes it after about 0.1 s, and until then it pulled the spectrum estimate toward 0 Hz. The hunt takes the three bins around 0 Hz as at most the interpolation of their neighbors, so a spur, which only adds power there, does not move the estimate.

A carrier exactly on the tuned frequency with no modulation, as a CW lead, and a DC offset are the same signal: both are a constant complex value c added to the noise, x[n] = c + w[n]. An estimator has only x as its input, so while the CW alone is present from the first sample, no receiver can separate the part of c that is offset from the part that is carrier. The DC block takes all of it as offset. When the modulation starts, the old mean stays in the DC estimate as a tone at the carrier until the estimate follows. On the matrix row with a 500 ms CW lead at 0 Hz, the leftover was 107 of 126 LSB after 20 ms, 47 after 50 ms and 23 at the lock, 120 ms after the CW ended. It cost the first burst and read the deviation 4 to 5% low. The DC block now starts its estimate again when a mean that held more than half of the power turns into a constant envelope whose phase moves away from it: that is a carrier that started its modulation, while a DC offset under a signal leaves the envelope varying. On that row the leftover falls to 4 LSB within 10 ms, and each tier finds the first burst, with 1 bit wrong.

On the XPR 8400 capture (`v2/cmd/dmrrf/testdata/xpr8400_gain_sweep.cu8`), the carrier is about +1.4 kHz: 1364 Hz is the power centroid and 1440 Hz the mirror center of its spectrum. The oscillator of each tier at the lock:

| Tier | Before | Now |
|------|--------|-----|
| 0 | 422 Hz, still 422 Hz 200 ms later | 1373 Hz |
| 1 | 658 Hz, 1340 Hz 200 ms later | 1373 Hz |
| 2 | 512 Hz, 1318 Hz 200 ms later | 1373 Hz |
| 3 | 1187 Hz | 1373 Hz |

- Over 64 start points Tier 0 now decodes 1608 of 3072 frames exact, and 1655 before. With the oscillator held fixed, it decodes 1561 at 422 Hz, 1602 at 1373 Hz and 1594 to 1637 from 800 to 1800 Hz, so Tier 0 has no offset bias. The 1655 came from the path of the oscillator before this change, 0 Hz for 60 ms and then 422 Hz, which beats every fixed frequency on this 1 s capture. On synthetic I/Q, Tier 0 with the oscillator held fixed decodes the most frames at the carrier.
- On 300 synthetic streams with a carrier 1.4 kHz off and a DC spur, each decoded by the receiver before and after the change with the same noise, Tier 0 decodes 8016 of 10800 frames exact, before 7395, at 7 dB C/N in 12.5 kHz. At 10 dB it decodes 10619, before 10344. The 64 start points of the capture share one second of samples, so they are not independent; the 300 streams are.
- `TestIQLockOffsetXPR8400` and `TestIQLockOffsetDCSpur` in `v2/rf` check the oscillator of each tier at and after the lock.

First burst after a noise lead, I/Q tier 0 at 2.048 Msps, 30 starts per offset (3 sequences, 5 noise leads, 2 noise seeds), C/N 47 dB in 12.5 kHz:

| Offset | Found, before | Found | Raw bits exact, before | Raw bits exact |
|--------|---------------|-------|------------------------|----------------|
| +1.5 kHz | 30/30 | 30/30 | 30/30 | 30/30 |
| -1.5 kHz | 30/30 | 30/30 | 30/30 | 30/30 |
| +2.5 kHz | 30/30 | 30/30 | 30/30 | 30/30 |
| -2.5 kHz | 30/30 | 30/30 | 18/30 | 18/30 |
| +3 kHz | 30/30 | 30/30 | 27/30 | 24/30 |
| -3 kHz | 30/30 | 30/30 | 30/30 | 30/30 |
| +4 kHz | 30/30 | 30/30 | 0/30 | 0/30 |
| -4 kHz | 30/30 | 30/30 | 0/30 | 0/30 |

- Before is the receiver without the small-offset retune and the DC spur handling of this section. The noise leads are 0.2, 0.2131, 0.2517, 0.3011 and 0.3377 s.
- At +3 kHz the retune from 32 ms of spectrum moves the oscillator again during the first burst in 3 more of the 30 starts.
- On 1,200 s of noise at 48 kHz, 600 s steady and 600 s with 2x steps of the noise power every 0.3 s, Tier 0 retuned 0 times, before and now.
- `TestRxTier0FirstBurstAtOffset` in `v2/rf` checks that the first burst is found at 2.5, 3 and 4 kHz.

### The deviation reading

`Receiver.DeviationHz` gives the outer deviation of the last locked stream. The spec value is 1944 Hz. Every tier reads it from the sync symbol levels. Two parts of the receiver make those levels low:

- The discriminator measures the phase step over one sample, so it averages the frequency over that sample. At 12 kHz this takes 2.6% off the levels, at 24 kHz 0.9%.
- The channel filter cuts the edges of the FM spectrum. On the tuned frequency this takes 2.3 to 3.2% more off the levels, and more when the carrier is off the tuned frequency.

The receiver divides the reading by the response of its own filter and discriminator at the carrier offset from the tuned frequency. `go generate` derives the response, `devGainIQ` and `devGainLow` in `v2/rf/tables_gen.go`: it sends a spec signal through the same filters and fits the sync levels at the best strobe time.

Clean signal, 3 bursts, I/Q at 48 kHz and 2.048 Msps, reading as a fraction of 1944 Hz:

| Input | Before | Now |
|-------|--------|-----|
| I/Q tiers 0 to 2, on the tuned frequency | 0.935 to 0.947 | 0.986 to 0.992 |
| I/Q tiers 0 to 2, 700 Hz to 4 kHz off | 0.923 to 0.952 | 0.971 to 1.022 |
| I/Q tier 3 | 1.000 | 1.000 |
| Low-rate I/Q tier 0, 9.6 kHz | 0.937 | 0.975 to 0.994 |
| Low-rate I/Q tier 0, 12 kHz | 0.969 | 0.991 to 0.994 |

- About 1% stays below tier 3: the strobe and the level steps of the core.
- At 9.6 kHz the reading follows the sub-sample phase of the bursts, the same sawtooth as the burst time.
- The table holds the response for a carrier up to 2.75 kHz from the tuned frequency at tiers 0 and 1, and 3.5 kHz at tier 2. Further off, the filter distorts the signal and the receiver uses the response at that limit.
- The levels keep most of the first sync fit of a stream. A first fit made before the timing settles can read high, 2.5% in a measured tier 0 case 1.5 kHz off. Each sync fit keeps the carrier offset of its samples, and the receiver looks the response up at the average of those offsets, so an AFC retune after a fit does not change its correction. `TestDeviationAfterRetune` checks this with a retune of 1.4 kHz.
- Low-rate I/Q comes from the channel filter of a radio chip. The reading does not include that filter, so it reads low when the filter cuts the signal.
- `TestIQDeviationReading` in `v2/rf` checks the reading within 3% at tiers 0 to 3 and 12 kHz, and within 4% at 9.6 kHz. The scenario matrix checks 0.95 to 1.03 at the I/Q tiers below tier 3 and 0.90 to 1.03 at 9.6 kHz.

## Comparison with other receivers

<!-- extcmp:begin -->
The tiers and other receivers decode the same noisy signals. Each cell is the Eb/N0 at which the receiver recovers 90% of the frames exactly. Lower is better.

- Receivers: dsd-fme (commit 587181f4), MMDVM firmware (commit 046e1df1), MMDVM firmware, end of burst fixed (commit 046e1df1).
- Each point decodes 192 noise seeds at each C/N from 0 to 20 dB in steps of 1 dB. Every receiver gets the same seeds.
- The FM audio comes from the discriminator model of the sweep with a 95-tap IF filter: a Kaiser-windowed sinc with its -6 dB point at 6 kHz and at least 84 dB of attenuation from 7.5 kHz, as the IF of a 12.5 kHz radio. The audio tiers get it at 9.6 kHz, dsd-fme at 48 kHz and the MMDVM firmware at 24 kHz, with the DC offset of the carrier and no other.
- dsd-fme decodes from `-fs -i - -o null -Z`. Its voice frames come from its AMBE+2 printout and its data blocks from one print line added after its FEC, which changes no decision. A frame counts when it equals the next frame of the truth in order. The MMDVM firmware is its DMO receiver (`DMRDMORX.cpp`) on the host with the 0.2 RRC filter of `IO.cpp`, and its burst bits go through the FEC of dmrgo. The tiers and the MMDVM firmware count a frame only at its own slot.
- The MMDVM DMO receiver takes each burst one sample before its last symbol arrives. `DMRDMORX.cpp` sets the end at `m_endPtr = m_dataPtr + DMR_SLOT_TYPE_LENGTH_SAMPLES / 2U + DMR_INFO_LENGTH_SAMPLES / 2U - 1U` (lines 327 and 358), 269 samples after the last SYNC symbol, but the last symbol of the burst is 270 samples after it, and `samplesToBits` reads it from `m_endPtr + DMO_BUFFER_LENGTH_SAMPLES - DMR_FRAME_LENGTH_SAMPLES + DMR_RADIO_SYMBOL_LENGTH + 1U` (line 126) on, which reaches the sample of 60 ms before. So the last two bits of every burst are random, and its voice and rate 3/4 cells do not reach 90% at any C/N. The second MMDVM row drops the `- 1U` and the `+ 1U`. Only MS bursts carry the SYNC patterns that the MMDVM DMO receiver looks for.
- On a Cortex-M4F the MMDVM DMO receiver with its RRC filter (`arm_fir_fast_q15` of CMSIS-DSP in blocks of 2 samples, as `IO.cpp` runs it), built with arm-none-eabi-gcc 13.2 -Os and run in QEMU mps2-an386, Cortex-M4 on the MS stream at C/N 6.0 and 40.0 dB, needs up to 19.3 MHz by the cycle model of the Tier 0 numbers, 6,256 B of RAM (5,820 B of receiver state, 140 B of filter state and 296 B of stack) and 10,798 B of flash.

### BS outbound, both slots, carrier 1400 Hz off

| Receiver | Voice | Voice C0 C1 | Rate 1/2 | Rate 3/4 | Rate 1 | CSBK | LC header | Data header | Embedded LC | EMB | Slot type | CPU at C/N 6 dB |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Audio tier 0 | 9.6 dB | 6.0 dB | 6.6 dB | 8.2 dB | 11.6 dB | 6.5 dB | 6.7 dB | 6.6 dB | 9.6 dB | 4.4 dB | 4.8 dB | - |
| Audio tier 1 | 8.3 dB | 5.3 dB | 6.6 dB | 8.0 dB | 11.6 dB | 5.4 dB | 5.8 dB | 5.4 dB | 5.8 dB | 4.1 dB | 4.5 dB | - |
| Audio tier 2 | 5.0 dB | 4.0 dB | 4.4 dB | 6.2 dB | 8.5 dB | 3.9 dB | 4.1 dB | 4.0 dB | 3.8 dB | 2.3 dB | 2.2 dB | - |
| Low-rate I/Q tier 0 | 10.3 dB | 5.5 dB | 6.7 dB | 8.2 dB | 12.7 dB | 6.6 dB | 6.6 dB | 6.9 dB | 9.7 dB | 4.0 dB | 4.4 dB | - |
| I/Q tier 0 | 9.4 dB | 4.8 dB | 5.7 dB | 7.6 dB | 11.3 dB | 5.5 dB | 5.9 dB | 5.9 dB | 9.3 dB | 3.3 dB | 3.8 dB | - |
| I/Q tier 1 | 8.0 dB | 3.9 dB | 5.5 dB | 7.1 dB | 11.4 dB | 3.9 dB | 4.1 dB | 4.0 dB | 4.1 dB | 3.0 dB | 3.2 dB | - |
| I/Q tier 2 | 4.7 dB | 3.1 dB | 4.2 dB | 5.4 dB | 8.0 dB | 3.1 dB | 3.1 dB | 3.3 dB | 3.0 dB | < 1.1 dB | 1.3 dB | - |
| dsd-fme | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | - | - | 30 ms |

### MS bursts, one slot, carrier 1400 Hz off

| Receiver | Voice | Voice C0 C1 | Rate 1/2 | Rate 3/4 | Rate 1 | CSBK | LC header | Data header | Embedded LC | EMB | Slot type | CPU at C/N 6 dB |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Audio tier 0 | 9.7 dB | 6.1 dB | 6.5 dB | 8.7 dB | 11.6 dB | 6.5 dB | 7.1 dB | 6.4 dB | 9.6 dB | 5.8 dB | 4.7 dB | - |
| Audio tier 1 | 9.7 dB | 5.9 dB | 6.5 dB | 8.6 dB | 11.6 dB | 5.4 dB | 6.1 dB | 5.1 dB | 6.1 dB | 5.7 dB | 4.6 dB | - |
| Audio tier 2 | 5.9 dB | 5.4 dB | 4.1 dB | 6.4 dB | 8.5 dB | 3.9 dB | 5.9 dB | 3.9 dB | 5.7 dB | 5.3 dB | 3.6 dB | - |
| Low-rate I/Q tier 0 | 10.4 dB | 6.1 dB | 6.5 dB | 8.7 dB | 12.6 dB | 6.6 dB | 10.8 dB | 6.3 dB | 9.8 dB | 5.7 dB | 4.1 dB | - |
| I/Q tier 0 | 9.6 dB | 5.5 dB | 5.5 dB | 8.0 dB | 11.3 dB | 5.7 dB | 8.4 dB | 5.4 dB | 9.7 dB | 5.0 dB | 3.5 dB | - |
| I/Q tier 1 | 9.1 dB | 4.9 dB | 5.4 dB | 7.4 dB | 11.4 dB | 3.8 dB | 5.5 dB | 3.8 dB | 5.1 dB | 4.7 dB | 3.1 dB | - |
| I/Q tier 2 | 4.9 dB | 4.2 dB | 2.9 dB | 5.5 dB | 7.8 dB | 2.3 dB | 5.2 dB | 2.4 dB | 4.7 dB | 4.0 dB | < 1.1 dB | - |
| dsd-fme | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | > 21.1 dB | - | - | 28 ms |
| MMDVM firmware | > 21.1 dB | 10.0 dB | 10.5 dB | > 21.1 dB | 11.4 dB | 10.3 dB | 10.4 dB | 10.3 dB | 10.1 dB | 10.0 dB | 10.3 dB | 1.5 ms |
| MMDVM firmware, end of burst fixed | 9.9 dB | 9.0 dB | 10.0 dB | 10.1 dB | 11.4 dB | 9.8 dB | 9.9 dB | 9.7 dB | 9.5 dB | 9.0 dB | 9.8 dB | 1.5 ms |

### BS outbound, both slots, carrier 0 Hz off

| Receiver | Voice | Voice C0 C1 | Rate 1/2 | Rate 3/4 | Rate 1 | CSBK | LC header | Data header | Embedded LC | EMB | Slot type | CPU at C/N 6 dB |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Audio tier 0 | 9.6 dB | 5.8 dB | 6.2 dB | 8.3 dB | 11.4 dB | 6.1 dB | 6.2 dB | 6.4 dB | 9.7 dB | 4.1 dB | 4.4 dB | - |
| Audio tier 1 | 8.2 dB | 5.0 dB | 6.2 dB | 7.9 dB | 11.4 dB | 5.0 dB | 5.3 dB | 5.1 dB | 5.1 dB | 4.0 dB | 4.1 dB | - |
| Audio tier 2 | 5.0 dB | 3.8 dB | 4.2 dB | 6.3 dB | 8.3 dB | 3.8 dB | 4.0 dB | 3.9 dB | 3.7 dB | 2.4 dB | 2.4 dB | - |
| Low-rate I/Q tier 0 | 9.7 dB | 5.0 dB | 6.1 dB | 8.0 dB | 11.5 dB | 6.0 dB | 6.0 dB | 6.1 dB | 9.6 dB | 3.4 dB | 3.7 dB | - |
| I/Q tier 0 | 9.3 dB | 4.8 dB | 5.6 dB | 7.9 dB | 11.4 dB | 5.4 dB | 5.6 dB | 5.7 dB | 9.3 dB | 3.1 dB | 3.3 dB | - |
| I/Q tier 1 | 8.0 dB | 3.9 dB | 5.4 dB | 7.1 dB | 11.5 dB | 3.9 dB | 4.0 dB | 3.9 dB | 4.1 dB | 2.9 dB | 3.0 dB | - |
| I/Q tier 2 | 4.7 dB | 3.1 dB | 4.2 dB | 5.5 dB | 7.9 dB | 3.1 dB | 3.1 dB | 3.3 dB | 3.0 dB | < 1.1 dB | 1.3 dB | - |
| dsd-fme | 20.5 dB | 9.6 dB | 12.9 dB | 15.3 dB | > 21.1 dB | 11.3 dB | 11.7 dB | 15.6 dB | 19.0 dB | - | - | 37 ms |

### MS bursts, one slot, carrier 0 Hz off

| Receiver | Voice | Voice C0 C1 | Rate 1/2 | Rate 3/4 | Rate 1 | CSBK | LC header | Data header | Embedded LC | EMB | Slot type | CPU at C/N 6 dB |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Audio tier 0 | 9.7 dB | 6.1 dB | 6.1 dB | 8.4 dB | 11.6 dB | 6.1 dB | 7.0 dB | 6.1 dB | 9.6 dB | 5.8 dB | 4.4 dB | - |
| Audio tier 1 | 9.7 dB | 5.9 dB | 6.1 dB | 8.2 dB | 11.6 dB | 4.9 dB | 6.0 dB | 5.0 dB | 6.1 dB | 5.7 dB | 4.2 dB | - |
| Audio tier 2 | 6.0 dB | 4.6 dB | 3.9 dB | 6.5 dB | 8.5 dB | 3.3 dB | 5.1 dB | 3.5 dB | 4.8 dB | 4.0 dB | 2.3 dB | - |
| Low-rate I/Q tier 0 | 9.8 dB | 5.7 dB | 6.1 dB | 8.4 dB | 11.7 dB | 6.0 dB | 7.1 dB | 6.0 dB | 9.8 dB | 5.1 dB | 3.9 dB | - |
| I/Q tier 0 | 9.5 dB | 5.2 dB | 5.5 dB | 7.9 dB | 11.1 dB | 5.4 dB | 6.2 dB | 5.5 dB | 9.3 dB | 4.8 dB | 3.3 dB | - |
| I/Q tier 1 | 9.1 dB | 4.7 dB | 5.3 dB | 7.5 dB | 11.4 dB | 3.8 dB | 4.9 dB | 3.8 dB | 4.9 dB | 4.5 dB | 3.0 dB | - |
| I/Q tier 2 | 4.1 dB | 3.0 dB | 2.9 dB | 5.4 dB | 7.9 dB | 2.3 dB | 3.4 dB | 2.4 dB | 3.1 dB | 2.5 dB | < 1.1 dB | - |
| dsd-fme | 16.6 dB | 8.9 dB | 11.1 dB | 14.6 dB | 18.0 dB | 11.3 dB | > 21.1 dB | 11.3 dB | 14.8 dB | - | - | 35 ms |
| MMDVM firmware | > 21.1 dB | 10.0 dB | 10.4 dB | > 21.1 dB | 11.4 dB | 10.4 dB | 10.2 dB | 10.2 dB | 10.1 dB | 10.0 dB | 10.3 dB | 1.5 ms |
| MMDVM firmware, end of burst fixed | 10.0 dB | 9.1 dB | 10.0 dB | 10.1 dB | 11.4 dB | 10.0 dB | 9.8 dB | 9.8 dB | 9.7 dB | 9.1 dB | 9.9 dB | 1.5 ms |

### Cells won and lost

A cell is one payload of one stream at one C/N. On each seed the tier and the other receiver decode the same signal, and the difference of their exact frames is taken. The tier wins the cell when the mean difference over the seeds is more than two standard errors above 0, and is behind when it is more than two standard errors below 0. The cells between are ties, which count as lost. Cells where both recover every frame, or both none, are left out.

| Tier | dsd-fme: won, tied, behind | MMDVM firmware: won, tied, behind | MMDVM firmware, end of burst fixed: won, tied, behind |
|---|---|---|---|
| Audio tier 0 | 618, 32, 0 | 227, 35, 0 | 180, 46, 3 |
| Audio tier 1 | 639, 32, 0 | 239, 31, 0 | 196, 39, 2 |
| Audio tier 2 | 694, 28, 0 | 273, 25, 0 | 236, 27, 0 |
| Low-rate I/Q tier 0 | 637, 35, 0 | 230, 47, 17 | 179, 54, 35 |
| I/Q tier 0 | 654, 35, 0 | 246, 37, 1 | 201, 44, 7 |
| I/Q tier 1 | 679, 33, 0 | 264, 32, 0 | 221, 36, 8 |
| I/Q tier 2 | 707, 23, 0 | 284, 19, 0 | 247, 21, 0 |

<details><summary>Cells where a tier is behind</summary>

Frames recovered by the tier against the other receiver, and the mean difference a seed with its standard error.

- Audio tier 0 against MMDVM firmware, end of burst fixed:
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 9.0 dB: 1093 against 1113 of 1152, -0.104 ± 0.042 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 11.0 dB: 20607 against 20647 of 20736, -0.208 ± 0.061 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 12.0 dB: 20708 against 20722 of 20736, -0.073 ± 0.028 a seed
- Audio tier 1 against MMDVM firmware, end of burst fixed:
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 11.0 dB: 20605 against 20647 of 20736, -0.219 ± 0.060 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 12.0 dB: 20708 against 20722 of 20736, -0.073 ± 0.028 a seed
- Low-rate I/Q tier 0 against MMDVM firmware:
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 9.0 dB: 196 against 233 of 384, -0.193 ± 0.062 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 10.0 dB: 281 against 335 of 384, -0.281 ± 0.048 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 10.0 dB: 353 against 371 of 384, -0.094 ± 0.032 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 11.0 dB: 337 against 370 of 384, -0.172 ± 0.035 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 11.0 dB: 366 against 379 of 384, -0.068 ± 0.025 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 12.0 dB: 358 against 381 of 384, -0.120 ± 0.027 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 12.0 dB: 372 against 383 of 384, -0.057 ± 0.018 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 12.0 dB: 5364 against 5372 of 5376, -0.042 ± 0.019 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 13.0 dB: 376 against 384 of 384, -0.042 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 13.0 dB: 374 against 384 of 384, -0.052 ± 0.016 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 13.0 dB: 5366 against 5375 of 5376, -0.047 ± 0.017 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 14.0 dB: 380 against 384 of 384, -0.021 ± 0.010 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 14.0 dB: 377 against 384 of 384, -0.036 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 14.0 dB: 5369 against 5376 of 5376, -0.036 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 15.0 dB: 378 against 384 of 384, -0.031 ± 0.013 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 15.0 dB: 5370 against 5376 of 5376, -0.031 ± 0.013 a seed
  - MS bursts, one slot, carrier 0 Hz off, Rate 1, C/N 11.0 dB: 364 against 374 of 384, -0.052 ± 0.024 a seed
- Low-rate I/Q tier 0 against MMDVM firmware, end of burst fixed:
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 8.0 dB: 961 against 999 of 1152, -0.198 ± 0.095 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 9.0 dB: 18383 against 19228 of 20736, -4.401 ± 0.301 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 9.0 dB: 196 against 249 of 384, -0.276 ± 0.059 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 9.0 dB: 330 against 355 of 384, -0.130 ± 0.039 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 9.0 dB: 1071 against 1113 of 1152, -0.219 ± 0.048 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 10.0 dB: 19651 against 20321 of 20736, -3.490 ± 0.195 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 10.0 dB: 281 against 335 of 384, -0.281 ± 0.048 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 10.0 dB: 353 against 371 of 384, -0.094 ± 0.032 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 11.0 dB: 20268 against 20647 of 20736, -1.974 ± 0.116 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 11.0 dB: 337 against 370 of 384, -0.172 ± 0.035 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 11.0 dB: 366 against 379 of 384, -0.068 ± 0.025 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 12.0 dB: 20547 against 20722 of 20736, -0.911 ± 0.069 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 12.0 dB: 358 against 381 of 384, -0.120 ± 0.027 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 12.0 dB: 372 against 383 of 384, -0.057 ± 0.018 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 12.0 dB: 5364 against 5372 of 5376, -0.042 ± 0.019 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 13.0 dB: 20659 against 20733 of 20736, -0.385 ± 0.042 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 13.0 dB: 376 against 384 of 384, -0.042 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 13.0 dB: 374 against 384 of 384, -0.052 ± 0.016 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 13.0 dB: 5366 against 5375 of 5376, -0.047 ± 0.017 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 14.0 dB: 20694 against 20736 of 20736, -0.219 ± 0.033 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Rate 1, C/N 14.0 dB: 380 against 384 of 384, -0.021 ± 0.010 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 14.0 dB: 377 against 384 of 384, -0.036 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 14.0 dB: 5369 against 5376 of 5376, -0.036 ± 0.014 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 15.0 dB: 20713 against 20736 of 20736, -0.120 ± 0.025 a seed
  - MS bursts, one slot, carrier 1400 Hz off, LC header, C/N 15.0 dB: 378 against 384 of 384, -0.031 ± 0.013 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Slot type, C/N 15.0 dB: 5370 against 5376 of 5376, -0.031 ± 0.013 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 16.0 dB: 20725 against 20736 of 20736, -0.057 ± 0.017 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 17.0 dB: 20731 against 20736 of 20736, -0.026 ± 0.012 a seed
  - MS bursts, one slot, carrier 0 Hz off, Embedded LC, C/N 9.0 dB: 1076 against 1099 of 1152, -0.120 ± 0.055 a seed
  - MS bursts, one slot, carrier 0 Hz off, Embedded LC, C/N 10.0 dB: 1126 against 1138 of 1152, -0.062 ± 0.029 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 11.0 dB: 20548 against 20606 of 20736, -0.302 ± 0.073 a seed
  - MS bursts, one slot, carrier 0 Hz off, Rate 1, C/N 11.0 dB: 364 against 374 of 384, -0.052 ± 0.024 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 12.0 dB: 20673 against 20713 of 20736, -0.208 ± 0.047 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 13.0 dB: 20720 against 20736 of 20736, -0.083 ± 0.020 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 14.0 dB: 20732 against 20736 of 20736, -0.021 ± 0.010 a seed
- I/Q tier 0 against MMDVM firmware:
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 10.0 dB: 1126 against 1139 of 1152, -0.068 ± 0.026 a seed
- I/Q tier 0 against MMDVM firmware, end of burst fixed:
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 9.0 dB: 1083 against 1113 of 1152, -0.156 ± 0.045 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 10.0 dB: 20250 against 20321 of 20736, -0.370 ± 0.133 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Embedded LC, C/N 10.0 dB: 1126 against 1139 of 1152, -0.068 ± 0.026 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 11.0 dB: 20583 against 20647 of 20736, -0.333 ± 0.066 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 12.0 dB: 20687 against 20722 of 20736, -0.182 ± 0.035 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 13.0 dB: 20723 against 20733 of 20736, -0.052 ± 0.018 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 13.0 dB: 20730 against 20736 of 20736, -0.031 ± 0.013 a seed
- I/Q tier 1 against MMDVM firmware, end of burst fixed:
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 10.0 dB: 20263 against 20321 of 20736, -0.302 ± 0.147 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 11.0 dB: 20542 against 20647 of 20736, -0.547 ± 0.077 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 12.0 dB: 20667 against 20722 of 20736, -0.286 ± 0.044 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 13.0 dB: 20713 against 20733 of 20736, -0.104 ± 0.024 a seed
  - MS bursts, one slot, carrier 1400 Hz off, Voice, C/N 14.0 dB: 20729 against 20736 of 20736, -0.036 ± 0.014 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 11.0 dB: 20576 against 20606 of 20736, -0.156 ± 0.077 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 12.0 dB: 20680 against 20713 of 20736, -0.172 ± 0.040 a seed
  - MS bursts, one slot, carrier 0 Hz off, Voice, C/N 13.0 dB: 20724 against 20736 of 20736, -0.062 ± 0.018 a seed

</details>

<!-- extcmp:end -->

## Test conditions

### The sweep

<!-- rfparams:rx-sweep:begin -->
- The test stream is a BS outbound stream: a voice call on one slot and data, CSBKs and headers on the other.
- White Gaussian noise is added at each C/N in 12.5 kHz. Eb/N0 is C/N plus 1.1 dB.
- The audio tiers get the audio of an FM discriminator at 9.6 kHz. The sweep makes it from the same noisy signal with the IF filter of a 12.5 kHz radio, so their Eb/N0 is that of the signal at the antenna.
- The I/Q tiers get the noisy samples at 48 kHz. Low-rate I/Q tier 0 gets them at 9.6 kHz, through the decimation filter of a radio chip: a 255-tap Kaiser-windowed sinc with its -6 dB point at 4.32 kHz.
<!-- rfparams:rx-sweep:end -->
- Each point runs more streams for each payload until it has 20 errors or enough frames to show 20 errors at an error rate of 1e-4, the bottom of the graphs. A payload stops after two points in a row under 1e-4. The 90% point is a linear interpolation between C/N points.
- CPU is the CPU time of the receiver alone to receive one second of signal, on one goroutine locked to its thread, as the median of five runs. Before and after each run the test times a reference: audio tier 2 on two superframes at C/N 6 dB. It scales the run by 14.95 ms over the mean reference time, the reference time on the test machine (an AMD Ryzen 9 7950X) with no other load. Load on the machine slows the reference and the receiver alike, so the scaled time moves by less than 1 ms per second where the raw time moves by 8 ms. The discriminator model of the sweep is outside the timed part. The test measures it at C/N 0, 2, 4, 6, 8, 12, 20 and 40 dB on one noise seed. The worst case is the highest of these. RAM is what the receiver keeps in stream mode, with `OnBursts` set. For audio tier 0 that is the size of `rf.AudioReceiver`, and for low-rate I/Q tier 0 the size of the `rf.Receiver` of a TinyGo build: their whole state, which `Init` can place in static memory. For the other tiers it is the heap after `NewReceiver` or `NewAudioReceiver`.
- The data is in `v2/rf/testdata/rxbench.json`. `TestRxBench` in `v2/rf` writes it with `DMRRF_RXBENCH=full`. `DMRRF_RXBENCH=cpu` measures only the CPU again.

### The eye, offset, clock and late entry tests

- The eye diagrams use the sweep stream with no carrier offset at three C/N. The tier receives the noisy signal, and the test folds the output of the DMR matched filter on `Burst.Seconds` of each burst that the tier gives. The audio tiers fold the audio of the discriminator, and the I/Q tiers the discriminator of the I/Q, at 48 kHz.
- The offset test moves the carrier of the stream over the span of its graph. The clock test resamples the noisy I/Q to a sample clock that is off by each error of its graph, and scores each burst at its drifted time. The audio tiers get the audio of the discriminator of that signal.
- The late entry test starts the input at a random point of the call, and runs several inputs at each C/N.
- The rate probe calls the constructors with each rate of the probe and each tier.
- The data is in `v2/rf/testdata/rxsheet.json`. `TestRxSheet` in `v2/rf` writes it with `DMRRF_RXSHEET=1`.

### The floors

Both floors give a genie receiver the timing, the carrier, the noise level and every transmitted bit outside the payload. It also gets the side information that the tiers use: the color code for the EMB and slot type, and the embedded LC of the earlier superframes of the call.

- 4FSK floor (any code): the converse bound of Verdu and Han on the information density of the payload symbols. No code of that size on those symbols, and no decoder, can do better. For the rate 1 payload bits it is Fano's inequality on the same densities.
- DMR floor (ETSI codes): maximum likelihood decoding of the DMR code of the payload.
  - For the EMB, the slot type, the SYNC and the rate 1 payload, the test covers all values.
  - For the voice frame and the rate 3/4 block, a search finds the values, so the floor is a lower bound ("at least").
  - For the BPTC payloads and the embedded LC it is not computed. Maximum likelihood over 2^72 to 2^96 values is out of reach.
- Code cost: the DMR floor less the 4FSK floor. Where the DMR floor is a lower bound, the cost is a range up to the code-aware receiver.
- Code-aware: the genie receiver detects the payload bits exactly, bit by bit, and decodes them with the decoders of the coherent tiers. It is a receiver, not a bound.

The floors use the same streams and noise seeds as the tiers. Their generator is `v2/rf/floor_gen_test.go`, and the code-aware receiver is `v2/rf/floor_aware_test.go`.

### The graphs

- The graphs show the frame error rate per payload, and the bit error rate of the rate 1 payload bits.
- A rate 1 block that a tier misses counts all of its bits as wrong.
- Points with no errors are left out.
- The graphs leave out the DMR floor. For codes with 2^49 to 2^96 values, the phase memory of 4FSK keeps the searches from bounding it.

### The recordings

The real recordings are captures of a Motorola XPR 8400 repeater with an RTL-SDR. The score is the number of voice frames recovered exactly, in order. The data is in `v2/rf/testdata/rxbench_recordings.json`, and its generator is `v2/rf/recordings_test.go`.

- The truth is the AMBE+2 frames that went to the repeater: the IPSC voice packets that the repeater relayed for the HackRF uplink captures, and the packets that the bridge injected for the network to air captures.
- Each tier streams the whole capture. The score is the longest common subsequence of the received frames and the truth, so a frame counts only when it is exact and in order.
- A gain sweep capture is a 1 s slice of one call. Its score is against the whole call, and its frame count is the span of the call that the tiers received, at least the 51 frames of 1 s.
- C/N is in 12.5 kHz. The noise density comes from the carrier-off blocks of the capture, or from 40 to 60 kHz off the carrier when the capture has less than 0.2 s with no carrier.

## Generating this page

`go run -C cmd/rxbench . -update-readme` runs the sweep, which takes hours, then the tests of the eye diagrams, offset, clock and late entry, and writes the tables and graphs of this page. `-update-readme -sheet` runs only the second set, in minutes, and `-update-readme -no-sweep` only renders the committed data. `-check-readme` fails when the page or a graph differs from the data in `v2/rf/testdata`. `DMRGO_DOCPARAMS=write go test ./rf -run TestDocParams` in v2 writes the facts, limits and test parameters of this page from the code, and fails without the variable when they differ. `go run -C cmd/mcubench . -update` measures the Cortex-M4F numbers.
