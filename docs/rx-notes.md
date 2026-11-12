# Receiver design notes

These notes record how parts of the receiver were checked. [The receiver datasheet](rx.md) gives the specifications.

## Low-rate I/Q carrier range

The carrier of low-rate I/Q tier 0 must be less than 2.5 kHz from the center: the 9.6 kHz I/Q holds 4.8 kHz on each side, and at 2.5 kHz off 9.8 dB of the signal power, at 4 kHz 3.7 dB, falls outside the channel filter of the chip. On carriers 2.4 to 5 kHz off, tier 0 loses 28 cells of the comparison with the external receivers. 6 of them no receiver can win: a genie receiver given every parameter, which decodes C0 and C1 jointly gets at most 0.85 to 0.894 of the AMBE frames exact (upper 95% bound, against the bar of 0.90) at 4.8, +5 and -5 kHz, MS and BS. The other 22 need a coherent receiver. One that was measured needed about 5 KB of RAM, 42 KB of flash and 27 to 114 MHz on the Cortex-M4F and still won only 5 of the 12 cells at 2.4 and ±3 kHz, so tier 0 does not take that cost and these 22 cells are accepted as lost: 16 at carriers 2.4 to 4.8 kHz off, and 6 that an exact genie wins with 0.996 to 1.000 of the frames: the frames with C0 and C1 right at +5 and -5 kHz, and the exact frames at 4 kHz, MS and BS.

## Accepted Tier 0 costs

Tier 0 needs more than 2 KB of RAM with its stack on a Cortex-M4F; [Tier 0 on a Cortex-M4F](rx.md#tier-0-on-a-cortex-m4f) gives the measured cost. These parts of Tier 0 were accepted at their cost when they were added: the detection of standalone RC bursts, the MS bursts of both TDMA slots with their own timing and levels, the burst-edge gain hold, the CACH replay, the clock rate tracking, the I/Q gain cap, the lookback replay checks, and `PreEmphasis` on audio tier 0.

## An audio level step after the CACH

Tiers 0 and 1 slice the output of a linear matched filter. When the level of an audio input steps by a gain ratio G, the filter output undershoots before the step by about 0.09·(G-1)·x(ts), where x(ts) is the signal at the step. At +20 dB with the step at the end of a CACH, that is about 2.4 inner symbol levels on the last CACH symbol, against a decision margin of 1. An ideal float RRC with the true timing and the true levels before the step gets that CACH wrong on 64 of 64 payloads; at +10 dB it gets 64 of 64 right, with at most 0.74 inner levels of error. At 10 dB the gain ratio is close to 3, so the outer level before the step and the inner level after it differ by 5%, and the symbols before the step cannot be told from the symbols after it by their values. A slicer that places the step at the split that fits the symbol values best, with the true levels and timing, gets the CACH wrong on 11 and 14 of 64 payloads in two draws; with the step time known it gets 1 and 0 of 64 wrong. This bounds receivers that split on the symbol values, not every receiver. So at Tiers 0 and 1 the CACH of a burst need not be exact when a step of 10 dB or more lands within the RRC half-span, 4 symbols, after it. Tiers 2 and 3 decide on the phase and get it exact. `TestRxLevelStep` checks this.

## A level step inside a burst

When the level of an audio input steps by 10 dB inside a burst, a clip of the loud part can make the receiver lower its gain. A clip drop lowers the gain; it drops no samples. The ring still holds at the rails the symbols that the step took past them before the drop, and the matched filter mixes the levels before and after the step in the symbols at it. When the drop falls after the CACH and by the end of the SYNC of a burst that the step splits, Tiers 0 and 1 drop that burst: the levels fitted before the step do not hold after it, and a payload sliced with them has wrong bits. Other bursts that a step splits are sliced again from the head with the levels before and after the step, as `sliceHead` does. A step before the SYNC of a burst, as in the scenario matrix row `gap.none.+2500.ms2.data1.0ppm.nom.a12000.40dB.step10dB`, loses that burst at Tiers 0 and 1 in the same way. This is a limit of the receiver: a burst that a level step splits is dropped rather than decoded wrong. A prototype that keeps the last 512 raw samples and slices again with the levels after the step changed 437 of 500 missing bursts of that row into bursts with wrong bits, not into correct ones, and cost 256 B of RAM and 0.1 MHz on an M4 audio Tier 0, so it was not built.

A +10 dB step in the payload of a Tier 3 burst can still come out with bit errors. The last attempt to slice the step burst again after the level settled broke `TestLevelsSkipClickedSymbols` and moved the I/Q Tier 2 and 3 digests, so it was not landed and the case is a limit. On 500 held-out runs of `TestRxLevelStep` it fails 27 times (5.4%, upper 95% bound 7.8%); the dominant cell is +10 dB at 0 Hz and 12 ms at Tier 3, with 19 of 500 (3.8%, upper bound 5.9%). The +10 dB, 0 Hz, 0 ms, Tier 3 cell fails 1 of 1000 runs (upper bound 0.6%). Every other failing cell is at or below 0.6%.

## A carrier off the tuned frequency

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

## The deviation reading

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

## The lock burst time

The lock burst `Seconds` comes from the zero of the timing error of the MM detector over the 24 sync symbols. These parts of the receiver move it with no noise, and `TestSecondsLockBurstExact` checks the mean and the RMS of the error over 64 locks within 0.6 us:

- The kept samples are stored as int8 in steps of 64 Hz. The rounding error follows the signal, so it moves the zero by an amount that depends on the DC and the levels modulo one step. Measured at tier 2 against unrounded samples, on a grid offset by 0, 1/4, 1/2 and 3/4 of a step: -0.37, +0.02, +0.25 and +0.15 us. The mean over the DC is near 0, and the RMS is about 0.24 us. The test draws the carrier offset for every lock, so this enters as scatter, as it does for real carriers.
- The sync window is read between samples at 2.5 samples per symbol with a cubic interpolator. Its error follows the payload: over 120 payloads with no noise the lock scatter at tiers 0 to 2 is 0.15 to 0.34 us, against 0.10 to 0.15 us with an ideal interpolator. A 6-point Lagrange interpolator took tiers 0 and 2 to 0.11 and 0.26 us, but no check needs it, and it costs 8 B of Tier 0 stack.
- At tiers 1 and 2 the narrow channel filter that the receiver puts in place at the lock cuts the FM spectrum, and the zero on its output moved 0.2 us with the sync pattern. The lock time is fit on the kept samples filtered again with the wide filter at the new tune.

At I/Q tier 0 the first burst is timed before the AFC tunes, so the carrier offset is a DC in the discriminator output. The mean of the 256-sample blocks of the front moves by a tenth of the step a block, and it took out only about 400 Hz of the offset before the sync. The kept samples hold the FM at up to about 97 of their 127 steps, so a DC from about 2 kHz clipped them at the sync, and a lock burst 2.4 kHz off read 2.8 us RMS and up to 4.6 us off. Before the lock, I/Q tier 0 now moves the DC to the mean of a quarter block (64 samples) when that mean is more than 750 Hz from it and the variance of the quarter is that of a signal and not of noise; a quarter that a converter tune split is skipped. Over 64 payloads with no noise the lock burst at -2.5 to +2.5 kHz now reads 0.2 to 0.7 us RMS and at most 1.5 us off. What is left 2 kHz or more off is the wide channel filter, which cuts the FM on one side: -0.3 to -0.5 us mean at +2 to +2.5 kHz, and 0 us with the filter centered on the carrier.

At I/Q tiers 2 and 3 the int8 path is not the last word on the lock time. With no noise and the carrier on the tuned frequency the lock burst read 0.47 us RMS, and 2 to 2.5 kHz off 0.56 to 0.68 us RMS with a -0.3 us mean. Each step was taken out against a float copy of the front: the cubic interpolator gave 0.47 against 0.33 us with a 6-point Lagrange one, the int8 halfband output took 0.36 to 0.17 us, and the int8 input of the matched filter took 0.17 to 0 us; the int8 kept samples added less than 0.01 us. Off frequency the wide channel filter cut one side of the FM, and with the receiver tuned onto the carrier the error was the same as on frequency. So when a sync fit places a lock while the receiver searches or hunts, tiers 2 and 3 take the MM zero again on the 72 newest samples run from the kept raw I/Q through a converter tuned onto the carrier of the sync (when it is 150 Hz or more off), the halfband and the matched filter in float, read with a 6-point Lagrange interpolator. The channel filter of that converter stays at the channel center when the carrier lies toward an adjacent channel that the spectrum hunt found, as the live filter does: with the filter on a carrier 1.5 kHz off toward a DMR channel 40 dB stronger, the lock read 5 us late on average and up to 19 us. Over 64 payloads with no noise the lock burst then reads 0.05 us RMS and at most 0.08 us off at every offset from -2.5 to +2.5 kHz, and at 30 dB C/N 0.25 to 0.36 us RMS, against 0.81 to 0.96 us before.

At 30 dB C/N the timing scatter of the lock at the other tiers is about 0.75 to 0.9 us RMS. The Cramer-Rao bound for 24 known sync symbols at that C/N is 0.23 us, and the same MM zero computed on unrounded samples gives 0.64 us, so the gap is in the estimator, not the noise.

A sync fit whose MM zero lies after the newest kept sample took the time of that sample. The lock then read up to 10 us early: at tier 2, 2 of 8 BS locks 2 kHz off in one `TestSecondsLockBurstOffset` run. The fit now keeps the zero and reads the levels at the newest sample.
