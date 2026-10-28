# Receiver


Package `rf` receives DMR from FM discriminator audio, from the I/Q samples of a software radio, or from I/Q at 9.6 or 12 kHz that a radio chip filtered to the channel. Its tiers trade CPU and RAM for sensitivity, from a microcontroller to a PC. [v2/cmd/dmrrf](../../v2/cmd/dmrrf/README.md) decodes DMR from I/Q or discriminator audio in the file formats of common software radios, and writes test transmissions in the same formats.

<!-- rxbench:begin -->
| Tier | Input | Voice | Data rate 1/2 | CSBK | Data rate 3/4 | CPU per second of signal | RAM |
|------|-------|-------|---------------|------|---------------|--------------------------|-----|
| Best possible (4FSK floor) | - | -1.0 dB | -3.0 dB | -4.1 dB | 0.3 dB | - | - |
| Audio tier 0 | FM audio, 9.6 kHz, core | 9.6 dB | 6.4 dB | 6.3 dB | 8.2 dB | 0.33 ms | 1,440 B |
| Audio tier 1 | FM audio, 9.6 kHz, soft decoding, noise prediction | 8.2 dB | 6.4 dB | 5.3 dB | 7.9 dB | 1.1 ms | 50 KiB |
| Audio tier 2 | FM audio, 9.6 kHz, coherent detection | 5.0 dB | 4.4 dB | 3.9 dB | 6.3 dB | 8.2 ms | 550 KiB |
| Audio tier 3 | FM audio, 9.6 kHz, coherent detection, data passes | 5.0 dB | 3.9 dB | 3.2 dB | 6.3 dB | 8.1 ms | 1.2 MiB |
| Low-rate I/Q tier 0 | I/Q, 9.6 kHz, discriminator core | 10.3 dB | 6.5 dB | 6.4 dB | 8.2 dB | 0.41 ms | 1,424 B |
| I/Q tier 0 | I/Q, discriminator core | 9.4 dB | 5.5 dB | 5.4 dB | 7.7 dB | 2.2 ms | 18 KiB |
| I/Q tier 1 | I/Q, soft decoding, noise prediction | 8.0 dB | 5.4 dB | 3.7 dB | 7.1 dB | 2.8 ms | 50 KiB |
| I/Q tier 2 | I/Q, coherent detection | 4.9 dB | 4.4 dB | 3.2 dB | 5.5 dB | 11 ms | 491 KiB |
| I/Q tier 3 | I/Q, phase trellis with Tier 2 beside it | 2.6 dB | 1.8 dB | 1.7 dB | 4.9 dB | 727 ms | 35.5 MiB |

- Each cell is the Eb/N0 at which the tier recovers 90% of the frames. Lower is better. C/N in 12.5 kHz is 1.2 dB lower.
- CPU is measured on an AMD Ryzen 9 7950X 16-Core Processor. RAM is the state of the receiver. On a Cortex-M4F, audio tier 0 needs 2,008 B of RAM with its stack and up to 4.8 MHz, built with TinyGo -opt z, and low-rate I/Q tier 0 2,024 B and up to 4.5 MHz. No tier allocates after it starts.
- Audio tiers take FM discriminator audio, as from a radio. They also take inverted audio. I/Q tiers take samples at 48 kHz from a software radio, and low-rate I/Q tier 0 takes I/Q at 9.6 kHz that a radio chip filtered to the channel.
- [The sweep](#the-sweep) gives the method and [The floors](#the-floors) the bounds.

<details><summary>Sensitivity per payload type</summary>

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
| Voice frame (AMBE+2, 49 bits) | 9.6 dB | 8.2 dB | 5.0 dB | 5.0 dB | 10.3 dB | 9.4 dB | 8.0 dB | 4.9 dB | 2.6 dB |
| Voice frame, protected bits (C0 and C1) | 5.9 dB | 5.2 dB | 4.0 dB | 3.8 dB | 5.4 dB | 4.6 dB | 3.7 dB | 3.1 dB | 1.6 dB |
| Data block, rate 1/2 | 6.4 dB | 6.4 dB | 4.4 dB | 3.9 dB | 6.5 dB | 5.5 dB | 5.4 dB | 4.4 dB | 1.8 dB |
| Data block, rate 3/4 | 8.2 dB | 7.9 dB | 6.3 dB | 6.3 dB | 8.2 dB | 7.7 dB | 7.1 dB | 5.5 dB | 4.9 dB |
| Data block, rate 1 | 11.3 dB | 11.3 dB | 8.3 dB | 8.3 dB | 12.4 dB | 11.2 dB | 11.4 dB | 7.9 dB | 7.7 dB |
| CSBK | 6.3 dB | 5.3 dB | 3.9 dB | 3.2 dB | 6.4 dB | 5.4 dB | 3.7 dB | 3.2 dB | 1.7 dB |
| Voice LC header and terminator | 6.4 dB | 5.5 dB | 4.4 dB | 3.6 dB | 6.4 dB | 6.3 dB | 4.1 dB | 3.5 dB | 1.8 dB |
| Data header | 6.4 dB | 5.3 dB | 4.1 dB | 3.3 dB | 6.4 dB | 5.5 dB | 3.8 dB | 3.4 dB | 1.9 dB |
| Embedded LC | 9.5 dB | 4.4 dB | 2.6 dB | 2.3 dB | 9.6 dB | 9.3 dB | 2.6 dB | 2.1 dB | 0.4 dB |
| EMB | 3.9 dB | 3.8 dB | 2.2 dB | 1.9 dB | 2.7 dB | 2.4 dB | 1.9 dB | 1.2 dB | -0.1 dB |
| Slot type | 4.0 dB | 4.1 dB | 2.0 dB | 2.0 dB | 3.1 dB | 2.7 dB | 2.6 dB | 1.0 dB | -0.4 dB |
| SYNC detection | 3.7 dB | 3.6 dB | 1.1 dB | 1.1 dB | 2.6 dB | 2.2 dB | 2.0 dB | -0.6 dB | < -0.9 dB |

</details>

<details><summary>Real recordings</summary>

Voice frames recovered exactly from captures of a Motorola XPR 8400 repeater with an RTL-SDR.

| Recording | C/N | Frames | I/Q tier 0 | I/Q tier 1 | I/Q tier 2 | I/Q tier 3 |
|-----------|-----|--------|------|------|------|------|
| MS voice relayed, HackRF on the uplink | 9.3 dB | 2013 | 1470 | 1519 | 1821 | 1861 |
| Late entry voice relayed, HackRF on the uplink | 11.1 dB | 1890 | 1467 | 1501 | 1618 | 1630 |
| Voice with PI header relayed, HackRF on the uplink | 17.3 dB | 1818 | 1818 | 1818 | 1818 | 1818 |
| Mixed voice and data, network to air, run 1 | 9.7 dB | 144 | 114 | 117 | 122 | 123 |
| Mixed voice and data, network to air, run 2 | 10.6 dB | 144 | 118 | 121 | 125 | 125 |
| Voice, network to air | 10.0 dB | 90 | 88 | 90 | 90 | 90 |
| Voice, network to air | 11.6 dB | 90 | 84 | 84 | 90 | 90 |
| Gain sweep, RTL gain 0 dB | 9.9 dB | 51 | 42 | 46 | 50 | 50 |
| Gain sweep, RTL gain 9.7 dB | 7.3 dB | 54 | 31 | 41 | 44 | 49 |
| Gain sweep, RTL gain 19.7 dB | 10.4 dB | 51 | 50 | 51 | 51 | 51 |
| Gain sweep, RTL gain 29.7 dB | 11.2 dB | 54 | 44 | 49 | 50 | 50 |

</details>

<details><summary>Error rate graphs</summary>

Error rate against Eb/N0 for each tier, with the 4FSK floor dashed. Points with no errors are left out.

#### FM discriminator audio

![Voice frame, FM discriminator audio: frame error rate against Eb/N0, references and tiers](audio/ambe.svg)
![Data rate 1/2, FM discriminator audio: frame error rate against Eb/N0, references and tiers](audio/rate12.svg)
![Data rate 3/4, FM discriminator audio: frame error rate against Eb/N0, references and tiers](audio/rate34.svg)
![CSBK, FM discriminator audio: frame error rate against Eb/N0, references and tiers](audio/csbk.svg)
![Embedded LC, FM discriminator audio: frame error rate against Eb/N0, references and tiers](audio/elc.svg)
![Data rate 1 payload bits, FM discriminator audio: bit error rate against Eb/N0, references and tiers](audio/rate1_bit.svg)

#### I/Q at 9.6 kHz

![Voice frame, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](iqlow/ambe.svg)
![Data rate 1/2, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](iqlow/rate12.svg)
![Data rate 3/4, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](iqlow/rate34.svg)
![CSBK, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](iqlow/csbk.svg)
![Embedded LC, I/Q at 9.6 kHz: frame error rate against Eb/N0, references and tiers](iqlow/elc.svg)
![Data rate 1 payload bits, I/Q at 9.6 kHz: bit error rate against Eb/N0, references and tiers](iqlow/rate1_bit.svg)

#### I/Q

![Voice frame, I/Q: frame error rate against Eb/N0, references and tiers](iq/ambe.svg)
![Data rate 1/2, I/Q: frame error rate against Eb/N0, references and tiers](iq/rate12.svg)
![Data rate 3/4, I/Q: frame error rate against Eb/N0, references and tiers](iq/rate34.svg)
![CSBK, I/Q: frame error rate against Eb/N0, references and tiers](iq/csbk.svg)
![Embedded LC, I/Q: frame error rate against Eb/N0, references and tiers](iq/elc.svg)
![Data rate 1 payload bits, I/Q: bit error rate against Eb/N0, references and tiers](iq/rate1_bit.svg)

</details>

<!-- rxbench:end -->

`go run -C cmd/rxbench . -update-readme` runs the sweep and writes the tables and graphs above. `-check-readme` fails when they differ from the sweep data in `v2/rf/testdata`.

## The sweep

- The test stream is a BS outbound stream: a voice call on one slot and data, CSBKs and headers on the other.
- White Gaussian noise is added at each C/N in 12.5 kHz. Eb/N0 is C/N plus 1.2 dB.
- The audio tiers get the audio of an FM discriminator at 9.6 kHz. The sweep makes it from the same noisy signal with the IF filter of a 12.5 kHz radio, so their Eb/N0 is that of the signal at the antenna.
- The I/Q tiers get the noisy samples at 48 kHz. Low-rate I/Q tier 0 gets them at 9.6 kHz, through the decimation filter of a radio chip: a 255-tap Kaiser-windowed sinc with its -6 dB point at 4.32 kHz.
- Each point runs more streams until it has enough errors. The 90% point is a linear interpolation between C/N points.
- CPU is the process time to receive one second of signal. RAM is what the receiver keeps in stream mode, with `OnBursts` set. For audio tier 0 that is the size of `rf.AudioReceiver`, and for low-rate I/Q tier 0 the size of the `rf.Receiver` of a TinyGo build: their whole state, which `Init` can place in static memory. For the other tiers it is the heap after `NewReceiver` or `NewAudioReceiver`.
- The data is in `v2/rf/testdata/rxbench.json`. `TestRxBench` in `v2/rf` writes it with `DMRRF_RXBENCH=full`.

## The floors

Both floors give a genie receiver the timing, the carrier, the noise level and every transmitted bit outside the payload. It also gets the side information that the tiers use: the color code for the EMB and slot type, and the embedded LC of the earlier superframes of the call.

- 4FSK floor (any code): the converse bound of Verdu and Han on the information density of the payload symbols. No code of that size on those symbols, and no decoder, can do better. For the rate 1 payload bits it is Fano's inequality on the same densities.
- DMR floor (ETSI codes): maximum likelihood decoding of the DMR code of the payload.
  - For the EMB, the slot type, the SYNC and the rate 1 payload, the test covers all values.
  - For the voice frame and the rate 3/4 block, a search finds the values, so the floor is a lower bound ("at least").
  - For the BPTC payloads and the embedded LC it is not computed. Maximum likelihood over 2^72 to 2^96 values is out of reach.
- Code cost: the DMR floor less the 4FSK floor. Where the DMR floor is a lower bound, the cost is a range up to the code-aware receiver.
- Code-aware: the genie receiver detects the payload bits exactly, bit by bit, and decodes them with the decoders of the coherent tiers. It is a receiver, not a bound.

The floors use the same streams and noise seeds as the tiers. Their generator is `v2/rf/floor_gen_test.go`, and the code-aware receiver is `v2/rf/floor_aware_test.go`.

## The graphs

- The graphs show the frame error rate per payload, and the bit error rate of the rate 1 payload bits.
- A rate 1 block that a tier misses counts all of its bits as wrong.
- Points with no errors are left out.
- The graphs leave out the DMR floor. For codes with 2^49 to 2^96 values, the phase memory of 4FSK keeps the searches from bounding it.

## The recordings

The real recordings are captures of a Motorola XPR 8400 repeater with an RTL-SDR. The score is the number of voice frames recovered exactly, in order. The data is in `v2/rf/testdata/rxbench_recordings.json`.
