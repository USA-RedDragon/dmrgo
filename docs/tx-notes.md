# Transmitter design notes

These notes record how parts of the transmitter were checked. [The transmitter datasheet](tx.md) gives the specifications.

## Audio correction on a flat radio

The `Corr` correction of Tier 1 and Tier 2 audio is fitted through the radio model of the audio optimizer: a hold converter and a two-pole low pass at 8 kHz. Applied as fitted, it drives the band from 9 to 12.5 kHz too hard by 1/h, where h is the response of that model (-8.7 dB at 12 kHz), and on a radio with a flat response there the content lands on the steep part of Emission Mask D. That was the cause of the 2.3 dB loss of Mask D on the ideal radio model against the external transmitters in the comparison of October 2026. The band pass of `Corr` now scales the correction by 2h/(1+h), which leaves the same error on a flat radio as on the hold radio. `TestCorrFlatRadio` in `v2/internal/fsktx` measures the Mask D margin from 5.625 to 12.5 kHz on both radios with and without `Corr`, and fails unless `Corr` raises it by at least 0.5 dB on each.

## Accepted Tier 0 costs

I/Q tier 0 needs more than 4 MHz on a Cortex-M4F; [Tier 0 on a Cortex-M4F](tx.md#tier-0-on-a-cortex-m4f) gives the measured cost. Its cost was accepted at 4.275 MHz in November 2026, with the `Transmitter` and its stack at 1,304 bytes.

## Transmitter pairs

<!-- txpairs:begin -->

Two pairs of cells of the comparison of the transmit tiers with external transmitters (October 2026) took each half of the bar from a different transmitter. The external captures of that comparison are gone, so `TestTxPairs` compares Tier 1 audio with a stand-in for the GNU Radio transmitter: a plain root raised cosine of 16 symbols each side, through the same measurements, with 16-bit audio and a hold converter.

| Cell | Tier 1 | Root raised cosine |
| --- | ---: | ---: |
| Adjacent channel power at 48 kHz, dBc | -67.77 | -58.81 |
| Deviation error at 48 kHz, % | 0.0216 | 0.2044 |
| Eye error at 96 kHz | 0.00636 | 0.00791 |
| Eye error at 48 kHz, 10 Hz AC-coupled radio | 0.24521 | 0.24430 |
| The same, without the fit of gain and offset | 0.35272 | 0.35214 |
| Mean eye error, 10 Hz AC-coupled radio, 64 random streams | 0.29055 | 0.29063 |

- Adjacent channel power against deviation error: Tier 1 is lower on both cells at once.
- AC-coupled eye against the 96 kHz eye: Tier 1 has the lower 96 kHz eye. On the test stream, without the fit of gain and offset, the AC-coupled eye error is the wander of the data through the 10 Hz high pass and differs by 0.00058 between the two; with the fit, the rest is the intersymbol error of the one symbol where the wander peaks. Over 64 random streams of the same length, each sent by both, the paired difference of Tier 1 less the root raised cosine is -0.00008 with a standard error of 0.00011, and they win 34 and 30 streams: a tie within two standard errors, so the one-stream cell is an artifact of one symbol pattern.

The alternate channel power of the comparison with the MMDVM firmware ties for every transmitter at 24 kHz. The same signals through the hold converter of that comparison and through an ideal converter, which interpolates the codes with no images:

| Signal | Rate | Hold converter | Ideal converter |
| --- | ---: | ---: | ---: |
| Tier 1 | 24 kHz | -70.02 dBc | -97.09 dBc |
| Root raised cosine | 24 kHz | -70.03 dBc | -97.44 dBc |
| Root raised cosine | 48 kHz | -121.19 dBc | -112.75 dBc |
| Root raised cosine | 96 kHz | -121.17 dBc | -114.73 dBc |

- Alternate channel power at 24 kHz: through the hold converter every waveform reads the same, and through an ideal converter or at a higher rate it is far lower. The tie is the image of the converter at 24 kHz, which no waveform at that rate changes.

`go test ./internal/fsktx -run TestTxPairs -update-txpairs` in v2 measures them into `v2/internal/fsktx/testdata/txpairs.json` and writes this block, and `DMRGO_LONG=1` measures them again.

<!-- txpairs:end -->
