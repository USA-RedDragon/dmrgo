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
   `./dmrrf tx -seq data12 -freq <RXF> -ppm <P> -cc <CC> -bs <RID> -src <ID> -dst <ID> -o d12.iq`
   `hackrf_transfer -t d12.iq -f <RXF> -s 2000000 -a 0 -x <gain>`
   Raise `<gain>` from 0 in small steps until `./dmrrf rx -i rep-out.cu8 -freq <TXF>` shows the repeater repeating the bursts.
8. Repeat with `-seq data34` and `-seq data1`. If the repeater does not respond, try `-slot 1`, more `-wakeups`, or `-group -dst <talkgroup>`.
9. Keep `ipsc.pcap`, the `rx` outputs and the exact `tx` command lines; they are what the IPSC layout gets derived from.

Notes:
- Inbound TDMA timing: an idle repeater should accept the first wakeup and data bursts without alignment, but this is not verified. If it ignores them, the `rx` timing of its outbound bursts is the starting point for aligning.
- Operate within your license, on frequencies you are authorized to use, and identify your station as your rules require; the DMR bursts carry only your radio ID.
