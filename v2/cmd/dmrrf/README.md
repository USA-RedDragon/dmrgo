# dmrrf

`dmrrf` turns DMR bursts built with dmrgo into I/Q or FM modulator audio, and decodes DMR from I/Q or FM discriminator audio. It reads and writes files and pipes in the sample formats of common software radios. It does not drive a radio: a separate tool of your radio does the transmit or the capture.

- `dmrrf tx`: writes a DMR transmission. DMR 4FSK at 4800 symbols/s, ±648/±1944 Hz deviation, RRC alpha 0.2 (ETSI TS 102 361-1 clause 10.2), one TDMA slot per 60 ms frame by default. As I/Q it is the FM signal at `-offset-hz` from the tuned frequency; as audio it is the deviation signal for the modulation input of an FM transmitter, with full scale at the peak deviation (the tool prints the scale).
- `dmrrf rx`: decodes every burst with the receiver of package [rf](../../rf), reassembles data packets and voice superframes, and reports the carrier offset and deviation. It is a streaming receiver: it estimates the carrier from the spectrum, locks to the first SYNC it finds, and then follows the slot timing and the carrier through the transmission. It prints each burst as it decodes it and a summary at the end, so it can read a live pipe. `-tier` selects the tier of package rf: 0 slices the FM discriminator output, 1 adds the soft decoders, 2 detects the bursts again coherently with a trellis over 3 symbols that carries the carrier phase with each survivor, 3 (the default for I/Q) carries one of 52 carrier phases in the trellis state, in two passes, and takes each voice frame from it or from tier 2 beside it, for offline decoding. Audio takes tiers 0 to 3 and uses the default of package rf when `-tier` is not given. Every tier decodes each FEC field with soft decisions. A voice superframe starts at a burst A, at a slot near the voice SYNC where the call timing expects a burst A, or before a burst B or E with a clean EMB, when the SYNC of its burst A is within 16 bits of the voice SYNC.
- `dmrrf rx -ambe <backend> -pcm <file>` also decodes the AMBE+2 voice and writes it as 8 kHz 16-bit little-endian PCM (`-pcm -` writes it to standard output, and the text then goes to standard error). The backend is one of:
  - `software`: the software codec of the build. The stock build has none; a build that adds one replaces `software.go` with a file whose `newSoftware` returns its `vocoder.Codec`.
  - `serial:<device>[@<baud>]`: a DVSI AMBE-3000R or AMBE-3003 on a serial port, such as a ThumbDV or a DVstick (Linux only). The default is 460800 baud; a USB-3003 runs at 921600.
  - `ambeserver:<host>[:<port>]`: an AMBEserver. The default port is 2460.
- `dmrrf cal`: reports the frequency offset of an unmodulated carrier in I/Q, such as one that `tx -cw` wrote and another radio received. Divide by the frequency and multiply by 1e6 for the ppm figure that `-ppm` takes.

## Sample formats

`-i` (rx, cal) and `-o` (tx) take a file name or `-` for standard input or output. `-format` selects the format. Without it, dmrrf takes the format from a WAV header, or else from the file extension in the last column. Raw files have no header, so `-rate` is required for them; a WAV file gives its rate, and `-rate` overrides it.

| `-format` | Samples | Used by | Extension |
|-----------|---------|---------|-----------|
| `cu8` | I/Q, unsigned 8-bit | rtl_sdr, rx_sdr `-F CU8` | `.cu8` |
| `cs8` | I/Q, signed 8-bit | hackrf_transfer, rx_sdr and tx_sdr `-F CS8` | `.cs8` |
| `cs16le` (or `cs16`) | I/Q, signed 16-bit little-endian | rx_sdr and tx_sdr `-F CS16` (SoapySDR: LimeSDR, USRP, PlutoSDR, Airspy), `airspy_rx -t 2`, UHD `rx_samples_to_file --type short` | `.cs16` |
| `cf32le` (or `cf32`) | I/Q, 32-bit float little-endian | GNU Radio file sink and source (complex), rx_sdr `-F CF32`, `airspy_rx -t 0`, UHD `--type float` | `.cf32`, `.fc32`, `.cfile` |
| `s16le` | FM discriminator or modulator audio, signed 16-bit little-endian, mono | the discriminator tap or modulation input of a radio through a sound card (`arecord -f S16_LE`, `sox -t raw`) | none |
| `wav` | WAV: tx writes 16-bit mono audio; rx reads 8-bit or 16-bit PCM or 32-bit float, 1 channel (audio) or 2 channels (I/Q) | sound cards, SDR recorders | `.wav` |

Any sample rate works. rx and cal resample to the rate of the receiver internally. tx writes at `-rate`: required for I/Q, 48000 by default for audio.

Frequency correction: `-freq` is the tuned frequency in Hz, `-ppm` the frequency error of the radio in ppm (positive when it runs high), and `-offset-hz` the offset of the DMR carrier from the tuned frequency, as with an offset tune that keeps the carrier away from the DC spike of a zero-IF radio. tx moves the carrier to `-offset-hz` minus the error; rx and cal move it back to 0 Hz before the receiver. They apply to I/Q only.

After an I/Q file, tx prints an example transmit command for formats that a common tool takes: `hackrf_transfer` for `cs8` and `tx_sdr` for `cs16le`. It is only a hint; use the tool and the gain of your radio.

## Examples

Build once: `go build -o dmrrf ./cmd/dmrrf` from the `v2` directory. `<F>` is the frequency in Hz.

Receive:

- RTL-SDR, from a file or live: `rtl_sdr -f <F> -s 2048000 cap.cu8`, then `./dmrrf rx -i cap.cu8 -rate 2048000 -freq <F>`; or `rtl_sdr -f <F> -s 2048000 - | ./dmrrf rx -i - -format cu8 -rate 2048000`.
- HackRF: `hackrf_transfer -r - -f <F> -s 2000000 | ./dmrrf rx -i - -format cs8 -rate 2000000`.
- Any SoapySDR radio (LimeSDR, PlutoSDR, Airspy, USRP): `rx_sdr -f <F> -s 1000000 -F CS16 - | ./dmrrf rx -i - -format cs16le -rate 1000000`.
- USRP with UHD: `rx_samples_to_file --freq <F> --rate 1e6 --type float --file cap.cf32`, then `./dmrrf rx -i cap.cf32 -rate 1000000`.
- GNU Radio: a File Sink of complex samples writes `cf32le`: `./dmrrf rx -i cap.cfile -rate <rate>`.
- Offset tune, 100 kHz below the carrier: `./dmrrf rx -i cap.cs16 -rate 1000000 -offset-hz 100000`.
- Discriminator audio from a radio or modem at 48 kHz: `arecord -f S16_LE -r 48000 -c 1 -t raw | ./dmrrf rx -i - -format s16le -rate 48000`, or a WAV recording: `./dmrrf rx -i disc.wav`.
- Voice through a DVSI dongle to the speaker: `./dmrrf rx -i cap.cu8 -rate 2048000 -ambe serial:/dev/ttyUSB0 -pcm - | aplay -f S16_LE -r 8000 -c 1`.

Transmit (into a dummy load or an attenuator first):

- HackRF: `./dmrrf tx -seq voice -slot 0 -rate 2000000 -freq <F> -o voice.cs8`, then `hackrf_transfer -t voice.cs8 -f <F> -s 2000000 -x 0`.
- Any SoapySDR radio: `./dmrrf tx -seq data12 -rate 1000000 -o - -format cs16le | tx_sdr -f <F> -s 1000000 -F CS16 -`.
- GNU Radio: `./dmrrf tx -seq sms -rate 1000000 -o sms.cfile`, read with a File Source of complex samples.
- FM transmitter with a flat modulation input (a 9600 baud data port): `./dmrrf tx -seq data12 -o data.wav -rate 48000`, then play `data.wav` into the input.
- A radio with a frequency error: measure it with `tx -cw 5 -o cw.cs8 -rate 2000000`, a capture with a radio you trust and `./dmrrf cal -i cw.cu8 -rate 2048000 -freq <F>`, then give the ppm figure to `-ppm` with `-freq`.


| Sequence | Bursts |
|----------|--------|
| `wakeup` | MS-sourced BS_Dwn_Act CSBKs (TS 102 361-2 Table 7.5a) |
| `data12` | wakeups, then a confirmed data packet at rate 1/2 |
| `data34` | wakeups, then a confirmed data packet at rate 3/4 |
| `data1`  | wakeups, then a confirmed data packet at rate 1 |
| `data12u`, `data34u`, `data1u` | wakeups, then the same message as an unconfirmed packet at rate 1/2, 3/4 or 1 |
| `ack` | wakeups, then an ACK response packet from `-src` to `-dst` |
| `sr` | wakeups, then a selective retry (SACK) response from `-src` to `-dst` for an 8-block packet, requesting blocks 1, 4 and 6 again (C_RDATA `adffffffffffffff` plus CRC, in the bit order of ETSI TS 102 361-1 Figure 8.17) |
| `preamble` | wakeups, `-preambles` Pre_CSBKs (opcode 0x3D, TS 102 361-2 clause 7.1.2.5, CSBK Blocks to Follow counting down to the data), then a confirmed rate 1/2 packet |
| `voice` | wakeups, voice LC header, `-superframes` superframes of bursts A to F with EMB and embedded LC, terminator with LC |
| `pi` | as `voice`, with a PI header after the voice LC header (ALGID 0x21, MFID 0x10, key ID 0x01, MI 0x12345678) and the privacy service option set; the voice is not encrypted |
| `rc` | wakeups, then a standalone MS-sourced reverse channel burst with the Cease Transmission command |
| `sms` | wakeups, `-preambles` Pre_CSBKs, then a DMR-format SMS: an unconfirmed short data defined packet (DD_HEAD) to SAP short data, with `-msg` in UTF-16LE and a zero terminator at rate 1/2, in the Defined Data format `-dd` (default 21, UTF-16LE) |
| `mbc` | `-bs-sourced` only: TV_GRANT as an MBC: header with physical channel 0xFFF, then a CG_AP continuation (channel 1, 446 MHz + 50 x 125 Hz) |
| `tier3` | C_ALOHA (system code 0x1234), TV_GRANT and C_MOVE CSBKs, then the `mbc` grant; `-bs-sourced` only, for a Tier III decoder such as dsd-fme |
| `t3site` | `-bs-sourced` only: a simulated Tier III site in one file with two carriers (see below) |

`t3site` writes a control channel at the tuned frequency (446.00625 MHz, LCN 1) and a traffic channel 25 kHz above it (LCN 2), with the MS transmit frequencies 5 MHz below:
- Control channel, TS1: C_ALOHA and C_BCAST Chan_Freq MBCs that give the absolute frequencies of LCN 1 and 2 (system code Small, NET 21, SITE 3), then a TV_GRANT from `-src` to talkgroup `-dst` on LCN 2 TS2, the same grant as an MBC with a CG_AP block, and late entry TV_GRANTs every 6 frames during the call. TS2 is idle. Every CACH carries a C_SYS_Parms Short LC.
- Traffic channel: the carrier keys up 3 frames after the grant and carries the call on TS2 (voice LC header, `-superframes` superframes, terminator) with Activity Update Short LCs.
- `-wakeups` and `-repeat` do not apply. Each carrier gets half of `-level`.

Voice flags:
- The voice carries a test pattern: AMBE+2 frame `n` is a distinct, known 49-bit value. `tx` prints each frame it sends as `ambe tx seq=<n> 0x<13 hex digits>`, and `rx` prints each frame it receives with the same `seq` (counted from the voice LC header), so a relayed call can be compared bit for bit. `-ambe-silence` sends AMBE+2 silence instead.
- Voice needs `-slot 0` or `-slot 1`: bursts B to F have no SYNC, so `rx` finds them on the slot timing, 60 ms apart on one channel.

`-bs-sourced` sends a continuous outbound stream like a repeater instead: BS-sourced SYNC, every 30 ms slot is a CACH (24 bits) and a burst (264 bits), 144 symbols with the carrier on throughout. The sequence goes on `-slot` (0 or 1) and idle bursts fill the other slot. `-wakeups` sets the number of idle frames before the sequence, and two idle frames follow it. Each CACH carries a TACT with TC set to the channel of the burst that follows, AT set while that channel carries the sequence, and one quarter of an Activity Update Short LC (SLCO 1). The Short LC announces the activity on the active slot while any of its four slots carries the sequence, and no activity otherwise. `wakeup` and `rc` are MS-sourced only. With `-repeat`, the stream is repeated without a gap.

`rx` prints, besides each burst, one line per item with a fixed first word:

| First word | Content |
|------------|---------|
| `cach` | CACH of a BS-sourced burst: AT, TC, LCSS, TACT FEC and the 17-bit payload |
| `slc` | Short LC assembled from four CACHs |
| `emb` | EMB of voice bursts B to F: superframe position, color code, PI, LCSS |
| `ambe` | the three AMBE+2 frames of a voice burst as 49-bit hex, with their FEC result |
| `elc` | Full LC assembled from the embedded LC of bursts B to E, with its checksum |
| `lc` | voice LC header and terminator with LC |
| `pi` | PI header fields |
| `response` | a response packet: ACK, NACK or SACK with the blocks to retry |

Each burst line shows the bits that `rx` decodes, and `fixed <n>` when soft-decision decoding changed n of the detected bits. `ts=1` or `ts=2` is the TDMA channel of a BS-sourced burst, and `ts=-` when there is no CACH. It comes from the slot number in the locked stream, which alternates every 30 ms, aligned with the CACH TC bits by majority: the Hamming(7,4,3) code of the TACT corrects every pattern to some codeword, so a TACT with two bit errors decodes with a wrong TC and `fec=fixed:1`, never `fec=bad`. `fec=ok`, `fec=fixed:<n>` (n bits corrected) and `fec=bad` report the FEC. For example:

```
cach t=93.75ms at=0 tc=1 lcss=last fec=ok payload=11111011000110000
slc t=93.75ms slco=1 activity ts1=0x9 hash1=0x5a ts2=0x0 hash2=0x00 fec=ok
pi t=123.75ms ts=1 algid=0x21 mfid=0x10 keyid=0x01 mi=0x12345678 dst=314926 fec=ok
emb t=301.25ms ts=- pos=B cc=7 pi=0 lcss=first fec=ok
ambe t=301.25ms ts=- pos=B seq=3 n=0 0x1b8a8724c81ec fec=ok
elc t=481.25ms ts=- flco=0 fid=0x00 group src=3191868 dst=9990 privacy=0 emergency=0 checksum=ok fec=ok
lc t=601.25ms ts=- terminator flco=0 fid=0x00 group src=3191868 dst=9990 privacy=0 emergency=0 fec=ok
response t=61.25ms ts=- SACK class=2 type=0 status=0 3191868 -> 314926 retry=1,4,6
```

## Capturing the IPSC layout of rate 3/4 and rate 1 data

Notes:
- Inbound TDMA timing: an idle repeater accepts an unaligned wakeup, but after it wakes it only decodes bursts that line up with its own slot timing. A half-duplex radio cannot follow that timing, so `-repeat` with `-stretch` sends the sequence again and again while the slow symbol clock moves the bursts across the slot. At 120 ppm, each 0.18 s repetition moves about 0.1 symbol, and 270 s cover a whole 30 ms slot. Repetitions that land far from the slot center still pass the sync but arrive with bit errors.
- `-slot 0` keeps every burst of a repetition in one timeslot. With `-slot -1` the header and its blocks alternate timeslots, and a repeater does not join them.
- A burst that starts at the first sample of an audio input can be lost while the audio receiver measures the level; real discriminator audio has noise before the key-up.
- Operate within your license, on frequencies you are authorized to use, and identify your station as your rules require; the DMR bursts carry only your radio ID.
