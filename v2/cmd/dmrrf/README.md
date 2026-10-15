# dmrrf

`dmrrf` turns DMR bursts built with dmrgo into I/Q for a HackRF, and decodes DMR from `rtl_sdr` captures. It only reads and writes files. Transmitting is a separate, manual `hackrf_transfer` step for a licensed operator.

- `dmrrf tx`: writes signed 8-bit I/Q (the `hackrf_transfer -t` format). DMR 4FSK at 4800 symbols/s, ±648/±1944 Hz deviation, RRC alpha 0.2 (ETSI TS 102 361-1 clause 10.2), one TDMA slot per 60 ms frame by default.
- `dmrrf rx`: reads `rtl_sdr` unsigned 8-bit I/Q, decodes every burst, reassembles data packets and voice superframes, and reports the carrier offset and deviation.
- `dmrrf cal`: reports the frequency offset of a carrier in an `rtl_sdr` capture.

Sequences for `tx -seq`. An MS-sourced sequence starts with `-wakeups` BS_Dwn_Act CSBKs. The data packets carry `-msg` from `-src` to `-dst` (`-group` for a talkgroup).

| Sequence | Bursts |
|----------|--------|
| `wakeup` | MS-sourced BS_Dwn_Act CSBKs (TS 102 361-2 Table 7.5a) |
| `data12` | wakeups, then a confirmed data packet at rate 1/2 |
| `data34` | wakeups, then a confirmed data packet at rate 3/4 |
| `data1`  | wakeups, then a confirmed data packet at rate 1 |
| `data12u`, `data34u`, `data1u` | wakeups, then the same message as an unconfirmed packet at rate 1/2, 3/4 or 1 |
| `ack` | wakeups, then an ACK response packet from `-src` to `-dst` |
| `sr` | wakeups, then a selective retry (SACK) response from `-src` to `-dst` for an 8-block packet, asking again for blocks 1, 4 and 6 (C_RDATA `b5ffffffffffffff` plus CRC) |
| `preamble` | wakeups, `-preambles` Pre_CSBKs (opcode 0x3D, TS 102 361-2 clause 7.1.2.5, CSBK Blocks to Follow counting down to the data), then a confirmed rate 1/2 packet |
| `voice` | wakeups, voice LC header, `-superframes` superframes of bursts A to F with EMB and embedded LC, terminator with LC |
| `pi` | as `voice`, with a PI header after the voice LC header (ALGID 0x21, MFID 0x10, key ID 0x01, MI 0x12345678) and the privacy service option set; the voice is not encrypted |
| `rc` | wakeups, then a standalone MS-sourced reverse channel burst with the Cease Transmission command |
| `mbc` | `-bs-sourced` only: TV_GRANT as an MBC: header with physical channel 0xFFF, then a CG_AP continuation (channel 1, 446 MHz + 50 x 125 Hz) |
| `tier3` | C_ALOHA (system code 0x1234), TV_GRANT and C_MOVE CSBKs, then the `mbc` grant; `-bs-sourced` only, for a Tier III decoder such as dsd-fme |

Voice flags:
- The voice carries a test pattern: AMBE+2 frame `n` is a distinct, known 49-bit value. `tx` prints each frame it sends as `ambe tx seq=<n> 0x<13 hex digits>`, and `rx` prints each frame it receives with the same `seq` (counted from the voice LC header), so a relayed call can be compared bit for bit. `-ambe-silence` sends AMBE+2 silence instead.
- Voice needs `-slot 0` or `-slot 1`: bursts B to F have no SYNC, so `rx` cuts them 60 ms apart after burst A.

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

`ts=1` or `ts=2` is the TDMA channel of a BS-sourced burst, and `ts=-` when there is no CACH. It comes from the slot time, which alternates every 30 ms, aligned with the CACH TC bits by majority: the Hamming(7,4,3) code of the TACT corrects every pattern to some codeword, so a TACT with two bit errors decodes with a wrong TC and `fec=fixed:1`, never `fec=bad`. `fec=ok`, `fec=fixed:<n>` (n bits corrected) and `fec=bad` report the FEC. For example:

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

Goal: send confirmed rate 3/4 and rate 1 data into a MOTOTRBO repeater over RF and capture what the repeater sends on IPSC. Values in angle brackets are yours: `<RXF>` is the repeater's receive (input) frequency in Hz, `<TXF>` its transmit (output) frequency, `<CC>` its color code, `<RID>` its radio ID, `<ID>` your radio ID.

Build once: `go build -o dmrrf ./cmd/dmrrf` from the `v2` directory.

1. Identify the RTL-SDR: `rtl_eeprom`. RTL-SDR Blog V3 and V4 sticks have a 1 ppm TCXO; others can be 20 ppm or more.
2. Measure the RTL-SDR against the repeater. Key the repeater (any traffic from DMRHub), then:
   `rtl_sdr -f <TXF> -s 2048000 -n 8192000 rep.cu8` and `./dmrrf rx -i rep.cu8 -freq <TXF>`.
   The repeater's oscillator is accurate, so the printed offset is about minus the RTL-SDR's error. Call it `R` (Hz). The `rx` output also shows the repeater's bursts, color code and timing.
3. Measure the HackRF. Keep the HackRF and RTL-SDR close with antennas off or through an attenuator, at the lowest gain:
   `./dmrrf tx -cw 5 -o cw.iq`, then run `rtl_sdr -f <RXF> -s 2048000 -n 8192000 cw.cu8` and, while it records, `hackrf_transfer -t cw.iq -f <RXF> -s 2000000 -x 0`.
   `./dmrrf cal -i cw.cu8 -freq <RXF>` prints `C` (Hz). The HackRF error is `C - R` Hz; divide by `<RXF>` and multiply by 1e6 for `-ppm` (positive means it transmits high).
4. Loopback check of the real waveform: `./dmrrf tx -seq data12 -freq <RXF> -ppm <P> -o d12.iq`, record it with `rtl_sdr` as in step 3, and confirm `./dmrrf rx -i d12.cu8 -freq <RXF>` prints `packet complete`.
5. On the DMRHub host, capture IPSC: `sudo tcpdump -i any -w ipsc.pcap udp port <IPSC port>`.
6. Start a monitor of the repeater output: `rtl_sdr -f <TXF> -s 2048000 rep-out.cu8` (stop it with Ctrl-C afterwards).
7. Transmit to the repeater input, rate 1/2 first because its IPSC layout is known and proves the path:
   `./dmrrf tx -seq data12 -slot 0 -wakeups 1 -repeat 270 -stretch 120 -freq <RXF> -ppm <P> -cc <CC> -bs <RID> -src <ID> -dst <ID> -o d12.iq`
   `hackrf_transfer -t d12.iq -f <RXF> -s 2000000 -a 0 -x <gain>`
   Raise `<gain>` from 0 in small steps until the repeater keys up (it sends IPSC opcode 0x85 when an over-the-air wakeup wakes it).
8. Repeat with `-seq data34` and `-seq data1`.
9. Keep `ipsc.pcap`, the `rx` outputs and the exact `tx` command lines; they are what the IPSC layout gets derived from.

Notes:
- Inbound TDMA timing: an idle repeater accepts an unaligned wakeup, but after it wakes it only decodes bursts that line up with its own slot timing. A half-duplex HackRF cannot follow that timing, so `-repeat` with `-stretch` sends the sequence again and again while the slow symbol clock moves the bursts across the slot. At 120 ppm, each 0.18 s repetition moves about 0.1 symbol, and 270 s cover a whole 30 ms slot. Repetitions that land far from the slot centre still pass the sync but arrive with bit errors.
- `-slot 0` keeps every burst of a repetition in one timeslot. With `-slot -1` the header and its blocks alternate timeslots and the repeater does not join them.
- Step 2 matters: an RTL-SDR Blog V3 measured 3 ppm high here, and calibrating the HackRF against it alone put the signal 1.4 kHz low.

What a MOTOTRBO XPR 8400 sends on IPSC for confirmed MS data (opcode 0x84, captured 2026-10-15):

| Block | Burst type (byte 30) | Bit count (bytes 36-37) | Payload from byte 38 |
|-------|----------------------|-------------------------|----------------------|
| Data header | 0x06 | 0x0060 | the 12 decoded octets |
| Rate 1/2 block | 0x07 | 0x0060 | the 12 decoded octets, as on air |
| Rate 3/4 block | 0x08 | 0x0090 | the 18 decoded octets with the 2 DBSN and CRC-9 octets moved from the start to the end |
| Undecodable block | 0x1F | 0x00C4 | the raw 196 information bits (burst bits 0 to 97, then 166 to 263) |

This repeater does not handle rate 1: a clean rate 1 block goes out as a 0x06 packet with a meaningless 96-bit BPTC decode, and a marginal one as 0x1F raw bits.
- Operate within your license, on frequencies you are authorized to use, and identify your station as your rules require; the DMR bursts carry only your radio ID.
