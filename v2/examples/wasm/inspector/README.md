# Packet inspector

A packet inspector that runs in the browser. It is the dmrgo library built to WebAssembly, with a static page and a small script. It needs no framework, no npm and no network access after the page loads.

Paste hex packets (one packet on each line), or drop a pcap file, a pcapng file or a binary dump. The page finds the protocol of each packet and shows a field tree. Click a field to highlight its octets. A field of a derived buffer, such as the BPTC payload of a burst, also highlights the packet octets that it came from when the transport carries them as they are.

## Protocols

- Motorola IP Site Connect (`transport/ipsc`): peer control, peer list, voice and data packets, and the packets that the library keeps raw.
- The DMRD family (`transport/dmrd`): the MMDVM network protocol, the HomeBrew Repeater Protocol, the MMDVMHost to DMRGateway local link, OpenBridge version 1 and the FreeDMR enhanced OpenBridge packets. The DMRD packet is the same in each dialect. The login, keepalive and configuration messages identify the dialect.
- Hytera IP Multi-Site Connect (`transport/hytera`): DMR frames, heartbeats, keepalives and P2P commands.
- Hytera HDAP (`hytera`): RRS, LP and TMP messages, on UDP ports 3002 to 3009 of a capture or inside a DMR data packet.
- MMDVM modem serial frames (`transport/mmdvm`): the DMR frames and the typed messages. A binary dump is cut into frames.
- Bare 33-octet DMR bursts, one on each line or in a binary dump of a multiple of 33 octets.

Inside each transport the page decodes the DMR burst: the SYNC, the slot type, the EMB and embedded data, the vocoder slots, the PDU (Full LC, CSBK, data headers and blocks, PI header), the FEC results, the embedded LC of a superframe, the Capacity Plus and Motorola CSBKs and LCs, and the data packets that the bursts of a channel make. A data packet is decoded further as a text message, a position, a UDP datagram, MOTOTRBO TMS, ARS and LRRP, and HDAP.

Decode errors show on the packet and on the field, with the reason that the library returns.

## Build and serve

From this directory:

```sh
GOOS=js GOARCH=wasm go build -trimpath -ldflags=-s -o inspector.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
python3 -m http.server 8080
```

Then open http://localhost:8080/. Any static web server works. The `wasm_exec.js` file must come from the Go release that built `inspector.wasm`.

The page also takes a capture from the server: `http://localhost:8080/?src=capture.pcap`. The optional parameters `format` (`auto`, `hex`, `pcap` or `binary`) and `protocol` (`auto`, `ipsc`, `dmrd`, `hytera`, `mmdvm`, `hdap` or `burst`) override the detection.

Built for the host, the same program prints the decode of a file as JSON:

```sh
go run . ../../../transport/ipsc/testdata/xpr8400.txt
```

## Test

```sh
go test .
```

The test feeds the captures and test vectors of the repository through `Inspect` and checks the decoded fields and their octets.
