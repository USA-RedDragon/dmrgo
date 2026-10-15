# dmrrf

`dmrrf` turns DMR bursts built with dmrgo into I/Q for a HackRF, and decodes DMR from `rtl_sdr` captures. It only reads and writes files. Transmitting is a separate, manual `hackrf_transfer` step for a licensed operator.

- `dmrrf tx`: writes signed 8-bit I/Q (the `hackrf_transfer -t` format). DMR 4FSK at 4800 symbols/s, ±648/±1944 Hz deviation, RRC alpha 0.2 (ETSI TS 102 361-1 clause 10.2), one TDMA slot per 60 ms frame by default.
- `dmrrf rx`: reads `rtl_sdr` unsigned 8-bit I/Q, decodes every burst, reassembles data packets, and reports the carrier offset and deviation.
- `dmrrf cal`: reports the frequency offset of a carrier in an `rtl_sdr` capture.

Sequences for `tx -seq`:

| Sequence | Bursts |
|----------|--------|
| `wakeup` | MS-sourced BS_Dwn_Act CSBKs (TS 102 361-2 Table 7.5a) |
| `data12` | wakeups, then a confirmed data packet at rate 1/2 |
| `data34` | wakeups, then a confirmed data packet at rate 3/4 |
| `data1`  | wakeups, then a confirmed data packet at rate 1 |

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
