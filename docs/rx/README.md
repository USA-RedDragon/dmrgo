# Receiver measurements

How `go run -C cmd/rxbench . -update-readme` measures the receiver tables and graphs in the [README](../../README.md#receiver).

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
