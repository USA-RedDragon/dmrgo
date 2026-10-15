# dmrgo

[![License](https://badgen.net/github/license/USA-RedDragon/dmrgo)](https://github.com/USA-RedDragon/dmrgo/blob/main/LICENSE.md) [![GoReportCard](https://goreportcard.com/badge/github.com/USA-RedDragon/dmrgo)](https://goreportcard.com/report/github.com/USA-RedDragon/dmrgo) [![codecov](https://codecov.io/gh/USA-RedDragon/dmrgo/graph/badge.svg?token=ON6XI1I21H)](https://codecov.io/gh/USA-RedDragon/dmrgo)

This library decodes and encodes raw 33-byte DMR packets per the ETSI TS 102 361 specification into Golang-native structures.

## Usage

```go
package main

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func main() {
	// A single 33-byte DMR burst (voice LC header)
	raw := [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}

	// A FEC or CRC failure is not an error: it shows in burst.FEC, and
	// burst.Data is still set.
	burst, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Kind: %s, SYNC: %s\n", burst.Kind, burst.SyncPattern)
	fmt.Println(burst)
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok && !burst.FEC.PDU.Uncorrectable {
		if target, source, ok := lc.Addresses(); ok {
			fmt.Printf("Call from %d to %d\n", source, target)
		}
	}

	// Re-encode back to 33 bytes
	encoded, err := burst.Encode()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Encoded %d bytes, unchanged: %t\n", len(encoded), encoded == raw)

	// Byte-level access to one field, without a full decode
	r := layer2.RawBurst(raw)
	payload, res := r.BPTCPayload()
	fmt.Printf("BPTC payload %x, %d bits corrected\n", payload, res.ErrorsCorrected)

	// Zero-allocation path: reuse a Burst across calls
	var reusable layer2.Burst
	for _, raw := range [][33]byte{raw} {
		if err := reusable.DecodeFromBytes(raw); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Println(reusable.String())
	}
}
```

`layer2.Burst` is the decoded form of a burst. `Kind` says whether it is a voice, data or standalone Reverse Channel burst. A data burst can carry an EMB and embedded data in place of the SYNC (§6.4.2), as ASELSAN Tier III base stations send it; then `SyncPattern` is `enums.SyncNone` and `HasEmbeddedSignalling` reports true. `DecodeFromBytes` never fails on a FEC or CRC error: `burst.FEC` reports it and `burst.Data` still holds the decoded PDU. The error is `layer2.ErrReservedDataType` for a reserved data type, or `layer2.ErrColorCodeMismatch` when `DecodeFromBytesWith` checks the color code. For an error instead of the FEC report, call `Err` on a `fec.Result`, such as `burst.FEC.Aggregate().Err()`: it returns `fec.ErrUncorrectable` when the result is uncorrectable and nil otherwise. Functions that check a message over several bursts, such as `layer2.VoiceReceiver` and `packet.Assembler`, return an error for a failed check, such as `layer2.ErrEmbeddedLCUncorrectable` or `packet.ErrMessageCRC`, together with what they decoded; these errors wrap `fec.ErrUncorrectable`. `DecodeFromBytesWith` also takes the burst kind, the voice burst position or the data type when the caller knows them from outside the burst. `Encode` writes a decoded burst back unchanged until its fields change. `Burst.Data` is a `layer2.Data`: a PDU with `DataType` and `EncodeBits`, which returns its information bits. `Encode` codes `Data.EncodeBits()` for the slot type, so `Data.DataType()` must match `SlotType.DataType`; for a Full LC that is its `BurstDataType`, which selects the RS(12,9) mask.

`layer2.RawBurst` reads and writes single fields of a 33-byte burst in place: the SYNC pattern, slot type, BPTC, Rate 3/4 and Rate 1 payloads, BPTC reserved bits, EMB, embedded data and voice slots. Motorola Capacity Max sets the BPTC reserved bit R(2) as a RAS flag; `Burst.BPTCReserved` and `RawBurst.BPTCReserved` read it, and `Encode` keeps it. `layer2.BuildDataBurst` builds a data burst from a 12-byte payload, `pdu.LCBytes.Codeword` and `pdu.FullLinkControl.Codeword` give a Full LC codeword, `pdu.Seal` rewrites the parity or CRC of an edited payload, and `pdu.FullLCParityOK` and `pdu.BlockCRCOK` check them. `pdu.LCBytes` edits the PF and Reserved bits, FLCO, FID, Service Options and addresses of the 9 raw LC octets in place, and `pdu.DataHeaderBytes` and `pdu.CSBKBytes` edit the addresses and Group flag of a raw data header or CSBK, which `Reseal` then gives a new CRC. `pdu.FullLinkControlFromBytes` and `FullLinkControl.Bytes` convert between the 9 LC octets and the decoded form. `pdu.EMBInfo` and `pdu.SlotTypeInfo` hold the EMB and slot type information bits in one octet without the QR(16,7,6) or Golay(20,8) parity, as IPSC carries them, and convert to and from the codewords.

The PDUs in `layer2/pdu` decode with `pdu.DecodeX(bits)` and encode with `x.Encode()`. `DecodeX` returns only the PDU: a PDU with its own FEC or CRC, such as a CSBK, Full LC, data header or slot type, reports the result in its `FEC` field, and a PDU without one, such as a CSBK payload or a data block, has no `FEC` field. Functions that decode a bare value, such as `layer2.DecodeEmbeddedLC` or `vocoder.DecodeAMBE72`, return a `fec.Result` with it. Raw and fallback fields, such as `CSBK.RawPayload`, `FullLinkControl.RawPayload` or `UnifiedSingleBlockData.Payload`, are byte arrays that hold their bits most significant bit first; when the field is not a whole number of bytes, the low bits of the last byte are padding and must be zero. An Encode error for a field value that does not fit its bits, padding bits included, is a `*bit.FieldRangeError`. Every type prints with `String`.

## Vocoders

A DMR voice burst carries three 72-bit vocoder frames of 20 ms each. `Burst.VoiceSlots` holds them as received. The `vocoder.Codec` interface converts one frame of 8 kHz 16-bit PCM to and from those 72 on-air bits:

```go
type Codec interface {
	io.Closer
	FrameSamples() int
	Encode(pcm []int16) ([72]bit.Bit, error)
	Decode(slot [72]bit.Bit) ([]int16, error)
	Silence() [72]bit.Bit
	FID() enums.FeatureSetID
}
```

Codec packages register themselves with `vocoder.Register`, like `database/sql` drivers. Use `vocoder.New(name)` and `vocoder.Names()` to find them. The FID is advisory: it does not identify the vocoder, so select the codec by name. Codecs are not safe for concurrent use unless their documentation says so. The interface does not change in v2; later features come as optional interfaces.

AMBE+2 is not included. `vocoder.EncodeAMBE72` and `vocoder.DecodeAMBE72` apply the DMR Golay and PN FEC to the 49 information bits of a `vocoder.AMBEFrame`, `Burst.AMBEFrames` and `Burst.SetAMBEFrames` decode and code the three frames of a burst, and `vocoder.AMBESilence` is the AMBE+2 silence frame.

### Codec2

**Build requirement:** the `vocoder/codec2` package uses cgo to link the system libcodec2 (LGPL 2.1) dynamically. A program that imports it needs a C compiler, libcodec2 and pkg-config (for example `apt-get install libcodec2-dev pkg-config`); without them the build fails with `Package 'codec2' not found`. Only programs that import `vocoder/codec2` need libcodec2. To build without it, use `-tags nocodec2` or `CGO_ENABLED=0`. `codec2.New` then returns `codec2.ErrUnavailable`.

The package is a Codec2 codec that is compatible with QRadioLink:

```go
import "github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"

c, err := codec2.New(codec2.Config{Mode: codec2.Mode2400, FID: codec2.DefaultFID})
```

- Each 72-bit frame holds one 20 ms Codec2 frame (`Mode2400` or `Mode3200`), MSB first, with zero padding. There is no FEC.
- Calls use Feature set ID `0xC2` in the Full LC. This FID is self-assigned, not allocated by ETSI. It does not identify the mode, so both ends must agree on the mode out of band.
- `Silence()` is 20 ms of digital silence encoded in the selected mode, for padding the last burst of a call.

## Examples

Each program in [`v2/examples`](v2/examples) runs offline on the captures in `v2/layer2/testdata` or on generated input. Run one with `go run -C v2 ./examples/<name>`.

- [`hotspot`](v2/examples/hotspot/main.go): decodes captured bursts with bit errors, applies the MMDVMHost acceptance check, regenerates clean bursts and reseals a rewritten Voice LC Header.
- [`bridge`](v2/examples/bridge/main.go): receives a call with `VoiceReceiver`, rewrites its talkgroup and source ID, and sends it again with `VoiceTransmitter`.
- [`scanner`](v2/examples/scanner/main.go): finds bursts in a noisy bit stream with the framer and follows the calls, talker alias and GPS on both time slots.
- [`trunking`](v2/examples/trunking/main.go): builds and decodes Tier III CSBKs, an MBC grant and CACH Short LC system parameters.
- [`data`](v2/examples/data/main.go): reassembles the SMS captures with `packet.Assembler`, and segments confirmed and unconfirmed packets with `packet.Segment`, including a selective retry.
- [`transcoder`](v2/examples/transcoder/main.go): checks the AMBE+2 FEC of a captured call, rebuilds it, and sends generated audio through Codec2 (skipped without libcodec2).
- [`conformance`](v2/examples/conformance/main.go): checks that every capture re-encodes byte for byte and that its FEC and CRCs verify with the byte-level functions.

The packages also have short godoc examples, for example `layer2.ExampleNewBurstFromBytes` and `packet.ExampleSegment`.

## ETSI TS 102 361 Specification Coverage
<!-- BEGIN ETSI COVERAGE -->

*Auto-generated by `cmd/speccheck --update-readme`. Status derived from test execution.*

### Summary

| Metric | Count |
| --- | --- |
| Total sections | 126 |
| ✅ Verified (tests pass) | 126 |
| ❌ Failing (tests fail) | 0 |
| 🔶 Untested (has source, no tests) | 0 |
| ⬜ Unimplemented | 0 |

**Overall: 126/126 verified (100.0%)**

### ETSI TS 102 361-1 — Air Interface Protocol

**64** sections - ✅ 64  ❌ 0  🔶 0  ⬜ 0

`[██████████████████████████████] 100.0%`

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| §6.1 | Vocoder socket | ✅ verified | 19/19 passed | `v2/layer2/burst.go`, `v2/vocoder/frame.go`, `v2/vocoder/codec.go`, `v2/layer2/voice_superframe.go`, `v2/internal/fec/golay/golay_23_12_7.go`, `v2/internal/fec/golay/golay_24_12_8.go`, `v2/enums/voice_burst_types.go` |
| §6.2 | Data and control | ✅ verified | 6/6 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/pi_header.go`, `v2/layer2/voice_transmitter_options.go` |
| §6.3 | Common Announcement Channel burst | ✅ verified | 5/5 passed | `v2/layer2/cach.go`, `v2/layer2/pdu/tact.go` |
| §6.4 | Reverse Channel | ✅ verified | 17/17 passed | `v2/layer2/reverse_channel.go`, `v2/layer2/burst.go` |
| §7.1.1 | Voice LC header | ✅ verified | 10/10 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/voice_superframe.go`, `v2/layer2/voice_transmitter_options.go` |
| §7.1.2 | Terminator with LC | ✅ verified | 7/7 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/voice_superframe.go` |
| §7.1.3 | Embedded signalling | ✅ verified | 25/25 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/embedded_lc.go`, `v2/layer2/embedded_lc_bytes.go`, `v2/layer2/voice_superframe.go`, `v2/layer2/voice_receiver.go`, `v2/layer2/voice_transmitter_options.go` |
| §7.1.4 | Short Link Control in CACH | ✅ verified | 11/11 passed | `v2/layer2/short_lc_assembler.go`, `v2/layer2/short_lc_cach.go`, `v2/internal/fec/bptc/bptc_cach.go` |
| §7.2.1 | Control Signalling BlocK (CSBK) | ✅ verified | 4/4 passed | `v2/layer2/pdu/csbk.go` |
| §7.3 | Idle message | ✅ verified | 7/7 passed | `v2/layer2/idle.go`, `v2/layer2/pdu/pr_fill.go`, `v2/layer2/burst.go` |
| §7.4.1 | Multi Block Control (MBC) | ✅ verified | 17/17 passed | `v2/layer2/pdu/mbc.go`, `v2/layer2/burst.go` |
| §8.2.1 | Header block structure | ✅ verified | 7/7 passed | `v2/layer2/pdu/data_header.go`, `v2/layer2/pdu/data_header_bytes.go` |
| §8.2.2 | Data block structure | ✅ verified | 11/11 passed | `v2/layer2/packet/packet.go`, `v2/layer2/packet/assembler.go`, `v2/layer2/packet/bursts.go` |
| §8.2.2.3 | Response packet format | ✅ verified | 3/3 passed | `v2/layer2/packet/response.go` |
| §8.2.3.1 | Unified Single Block Data block structure | ✅ verified | 10/10 passed | `v2/layer2/pdu/usbd.go`, `v2/layer2/burst.go` |
| §9.1.1 | Synchronization (SYNC) PDU | ✅ verified | 19/19 passed | `v2/enums/sync_patterns.go`, `v2/layer2/burst.go`, `v2/layer2/decode_options.go`, `v2/layer2/framer/framer.go` |
| §9.1.2 | Embedded signalling (EMB) PDU | ✅ verified | 9/9 passed | `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/pdu/emb_info.go` |
| §9.1.3 | Slot Type (SLOT) PDU | ✅ verified | 9/9 passed | `v2/layer2/pdu/slot_type.go`, `v2/layer2/pdu/slot_type_info.go` |
| §9.1.4 | TACT PDU | ✅ verified | 6/6 passed | `v2/layer2/pdu/tact.go`, `v2/internal/fec/hamming/hamming_7_4_3.go` |
| §9.1.5 | Reverse Channel (RC) PDU | ✅ verified | 6/6 passed | `v2/layer2/pdu/reverse_channel.go` |
| §9.1.6 | Full Link Control (FULL LC) PDU | ✅ verified | 10/10 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/full_link_control_bytes.go` |
| §9.1.7 | Short Link Control (SHORT LC) PDU | ✅ verified | 14/14 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go`, `v2/internal/crc/crc8.go` |
| §9.1.8 | Control Signalling Block (CSBK) PDU | ✅ verified | 7/7 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/csbk_bytes.go` |
| §9.1.9 | Pseudo Random Fill Bit (PR FILL) PDU | ✅ verified | 9/9 passed | `v2/layer2/pdu/pr_fill.go` |
| §9.2.2 | Rate ¾ coded packet Data (R_3_4_DATA) PDU | ✅ verified | 10/10 passed | `v2/layer2/pdu/rate34_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| §9.2.6 | Unconfirmed data packet Header (U_HEAD) PDU | ✅ verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| §9.2.7 | Rate ½ coded packet Data (R_1_2_DATA) PDU | ✅ verified | 7/7 passed | `v2/layer2/pdu/rate12_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| §9.2.9 | Proprietary Header (P_HEAD) PDU | ✅ verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| §9.2.13 | Unified Data Transport Header (UDT_HEAD) PDU | ✅ verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| §9.2.15 | Rate 1 coded packet Data (R_1_DATA) PDU | ✅ verified | 9/9 passed | `v2/layer2/pdu/rate1_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| §9.3.1 | Colour Code (CC) | ✅ verified | 4/4 passed | `v2/enums/color_code.go`, `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/pdu/slot_type.go` |
| §9.3.2 | Pre-emption and power control Indicator (PI) | ✅ verified | 1/1 passed | `v2/layer2/pdu/embedded_signalling.go` |
| §9.3.3 | LC Start/Stop (LCSS) | ✅ verified | 3/3 passed | `v2/enums/lcss.go` |
| §9.3.4 | EMB parity | ✅ verified | 3/3 passed | `v2/internal/fec/quadraticresidue/quadratic_residue_16_7_6.go`, `v2/layer2/pdu/embedded_signalling.go` |
| §9.3.5 | Feature set ID (FID) | ✅ verified | 11/11 passed | `v2/enums/feature_set_id.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| §9.3.6 | Data Type | ✅ verified | 6/6 passed | `v2/enums/data_type.go`, `v2/layer2/data.go` |
| §9.3.10 | Protect Flag (PF) | ✅ verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| §9.3.11 | Full Link Control Opcode (FLCO) | ✅ verified | 3/3 passed | `v2/enums/flco.go` |
| §9.3.17 | Data Packet Format (DPF) | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §9.3.18 | SAP identifier (SAP) | ✅ verified | 2/2 passed | `v2/layer2/pdu/data_header.go`, `v2/enums/sap.go` |
| §9.3.19 | Logical Link ID (LLID) | ✅ verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| §9.3.32 | Control Signalling BlocK Opcode (CSBKO) | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §B.1.1 | BPTC (196,96) | ✅ verified | 11/11 passed | `v2/internal/fec/bptc/bptc_196_96.go` |
| §B.2.1 | Variable length BPTC for embedded signalling | ✅ verified | 16/16 passed | `v2/internal/fec/bptc/bptc_embedded_lc.go` |
| §B.2.2 | Single Burst Variable length BPTC | ✅ verified | 14/14 passed | `v2/internal/fec/bptc/bptc_single_burst.go` |
| §B.2.3 | Variable length BPTC for CACH signalling | ✅ verified | 13/13 passed | `v2/internal/fec/bptc/bptc_cach.go` |
| §B.2.4 | Rate ¾ Trellis code | ✅ verified | 6/6 passed | `v2/internal/fec/trellis/trellis_3_4.go` |
| §B.2.5 | Rate 1 coded data | ✅ verified | 1/1 passed | `v2/layer2/burst.go` |
| §B.3.1 | Golay (20,8) | ✅ verified | 2/2 passed | `v2/internal/fec/golay/golay_20_8_7.go` |
| §B.3.2 | Quadratic residue (16,7,6) | ✅ verified | 1/1 passed | `v2/internal/fec/quadraticresidue/quadratic_residue_16_7_6.go` |
| §B.3.3 | Hamming (17,12,3) | ✅ verified | 1/1 passed | `v2/internal/fec/bptc/bptc_196_96.go` |
| §B.3.4 | Hamming (13,9,3), Hamming (15,11,3), and Hamming (16,11,4) | ✅ verified | 4/4 passed | `v2/internal/fec/bptc/bptc_196_96.go`, `v2/internal/fec/bptc/bptc_embedded_lc.go` |
| §B.3.5 | Hamming (7,4,3) | ✅ verified | 8/8 passed | `v2/internal/fec/hamming/hamming_7_4_3.go` |
| §B.3.6 | Reed-Solomon (12,9) | ✅ verified | 26/26 passed | `v2/internal/fec/reedsolomon/reed_solomon_12_9_4.go` |
| §B.3.7 | 8-bit CRC calculation | ✅ verified | 7/7 passed | `v2/internal/crc/crc8.go` |
| §B.3.8 | CRC-CCITT calculation | ✅ verified | 5/5 passed | `v2/internal/crc/crc_ccitt.go` |
| §B.3.9 | 32-bit CRC calculation | ✅ verified | 9/9 passed | `v2/internal/crc/crc32.go` |
| §B.3.10 | CRC-9 calculation | ✅ verified | 9/9 passed | `v2/internal/crc/crc9.go` |
| §B.3.11 | 5-bit Checksum (CS) calculation | ✅ verified | 9/9 passed | `v2/internal/crc/checksum5.go`, `v2/layer2/embedded_lc.go` |
| §B.3.12 | Data Type CRC Mask | ✅ verified | 26/26 passed | `v2/internal/crc/crc_ccitt.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/mmdvmhost.go`, `v2/layer2/pdu/codeword_bytes.go`, `v2/layer2/mmdvmhost_seal.go` |
| §B.3.13 | 7-bit CRC calculation | ✅ verified | 7/7 passed | `v2/internal/crc/crc7.go` |
| §B.4 | Interleaving | ✅ verified | 8/8 passed | `v2/layer2/cach_interleave.go` |
| §D | Idle and Null message bit definition | ✅ verified | 10/10 passed | `v2/layer2/idle.go` |
| §E | Transmit bit order | ✅ verified | 14/14 passed | `v2/layer2/transmit_order.go`, `v2/layer2/raw_burst.go` |

### ETSI TS 102 361-2 — DMR Voice and Generic Services and Facilities

**36** sections - ✅ 36  ❌ 0  🔶 0  ⬜ 0

`[██████████████████████████████] 100.0%`

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| §5.4.2 | Inband positioning data service | ✅ verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| §5.4.3 | Inband talker alias data service | ✅ verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer3/talker_alias_data_format.go`, `v2/layer2/pdu/talker_alias.go` |
| §6.1 | Transmit timeout | ✅ verified | 2/2 passed | `v2/constants/constants.go` |
| §6.3 | Reverse Channel Transmitter Interrupt | ✅ verified | 10/10 passed | `v2/layer2/pdu/reverse_channel.go`, `v2/enums/rc_command.go`, `v2/constants/constants.go` |
| §7.1.1 | Full Link Control PDUs | ✅ verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.1.1.1 | Group Voice Channel User LC PDU | ✅ verified | 6/6 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/lc_bytes.go` |
| §7.1.1.2 | Unit to Unit Voice Channel User LC PDU | ✅ verified | 6/6 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/lc_bytes.go` |
| §7.1.1.3 | GPS Info LC PDU | ✅ verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.1.1.4 | Talker Alias header LC PDU | ✅ verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer3/talker_alias_data_format.go` |
| §7.1.1.5 | Talker Alias block LC PDU | ✅ verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.1.2 | Control Signalling BlocK (CSBK) PDUs | ✅ verified | 6/6 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/csbk_addresses.go`, `v2/layer2/pdu/csbk_bytes.go` |
| §7.1.2.1 | BS Outbound Activation CSBK PDU | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.2.2 | Unit to Unit Voice Service Request CSBK PDU | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.2.3 | Unit to Unit Voice Service Answer Response CSBK PDU | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.2.4 | Negative Acknowledge Response CSBK PDU | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.2.5 | Preamble CSBK PDU | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.2.6 | Channel Timing CSBK PDU | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.3 | Short Link Control PDUs | ✅ verified | 18/18 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go`, `v2/enums/activity_id.go`, `v2/layer2/pdu/address_hash.go` |
| §7.2.1 | Service Options | ✅ verified | 4/4 passed | `v2/layer3/service_options.go` |
| §7.2.2 | Answer Response | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §7.2.3 | Reason Code | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §7.2.4 | Service Type | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| §7.2.5 | Source Type | ✅ verified | 5/5 passed | `v2/layer3/source_type.go`, `v2/layer2/pdu/csbk.go` |
| §7.2.6 | Additional Information Field | ✅ verified | 5/5 passed | `v2/layer3/additional_information_field.go`, `v2/layer2/pdu/csbk.go` |
| §7.2.7 | CSBK Blocks to Follow (CBF) | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §7.2.8 | Identifier (ID) | ✅ verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| §7.2.15 | Position Error | ✅ verified | 3/3 passed | `v2/layer3/position_error.go` |
| §7.2.16 | Longitude | ✅ verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.2.17 | Latitude | ✅ verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.2.18 | Talker Alias Data Format | ✅ verified | 3/3 passed | `v2/layer3/talker_alias_data_format.go` |
| §7.2.19 | Talker Alias Data Length | ✅ verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.2.20 | Tier 2 Reverse Channel (RC) Command | ✅ verified | 9/9 passed | `v2/enums/rc_command.go`, `v2/layer2/pdu/reverse_channel.go` |
| §A | Timers and constants in DMR | ✅ verified | 4/4 passed | `v2/constants/constants.go` |
| §B.1 | Full Link Control Opcode List | ✅ verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go` |
| §B.2 | CSBK Opcode List | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §B.3 | Short Link Control Opcode List | ✅ verified | 8/8 passed | `v2/enums/slco.go`, `v2/enums/activity_id.go`, `v2/layer2/pdu/short_link_control.go` |

### ETSI TS 102 361-3 — DMR Data Protocol

**13** sections - ✅ 13  ❌ 0  🔶 0  ⬜ 0

`[██████████████████████████████] 100.0%`

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| §5.3 | Unconfirmed data DLL bearer service | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §5.4 | Confirmed data DLL bearer service | ✅ verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| §5.5 | UDP/IPv4 data | ✅ verified | 6/6 passed | `v2/layer2/pdu/udp_ipv4_compressed.go`, `v2/enums/said.go`, `v2/enums/daid.go`, `v2/enums/spid.go`, `v2/enums/dpid.go` |
| §5.6 | UDP/IPv4 header compression | ✅ verified | 1/1 passed | `v2/layer2/pdu/udp_ipv4_compressed.go`, `v2/enums/said.go`, `v2/enums/daid.go`, `v2/enums/spid.go`, `v2/enums/dpid.go` |
| §6.1 | Defined Data | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §6.2 | Raw data | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §6.3 | Status/precoded data | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §6.4 | Short data confirmed response | ✅ verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| §7.1.1 | Full Link Control (FULL LC) PDUs | ✅ verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| §7.1.1.1 | Terminator Data Link Control PDU | ✅ verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go`, `v2/enums/flco.go` |
| §7.2 | UDP/IPv4 Compressed Header | ✅ verified | 1/1 passed | `v2/layer2/pdu/udp_ipv4_compressed.go` |
| §A | PDP timers and constants in DMR | ✅ verified | 2/2 passed | `v2/constants/constants.go` |
| §B.1 | PDP Full Link Control Opcode list | ✅ verified | 1/1 passed | `v2/enums/flco.go`, `v2/layer2/pdu/full_link_control.go` |

### ETSI TS 102 361-4 — DMR Trunking Protocol

**13** sections - ✅ 13  ❌ 0  🔶 0  ⬜ 0

`[██████████████████████████████] 100.0%`

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| §5.1 | The Use of the CACH | ✅ verified | 7/7 passed | `v2/layer2/pdu/short_link_control.go`, `v2/layer2/cach.go`, `v2/layer2/pdu/syscode.go` |
| §5.2 | Tier III Signalling | ✅ verified | 4/4 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/burst.go` |
| §5.3 | Modes of Control Channel | ✅ verified | 2/2 passed | `v2/layer2/burst.go` |
| §5.4 | CSBK/MBC/UDT/USBD Block Structure | ✅ verified | 4/4 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer2/pdu/data_header.go` |
| §6.2 | Random Access Procedures | ✅ verified | 2/2 passed | `v2/layer2/pdu/csbk.go`, `v2/constants/constants.go` |
| §6.6 | Call Procedures | ✅ verified | 3/3 passed | `v2/layer2/pdu/csbk.go` |
| §7.1.1 | Control Signalling Block (CSBK/MBC/UDT) PDUs | ✅ verified | 17/17 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer2/pdu/data_header.go`, `v2/enums/announcement_type.go` |
| §7.1.1.1 | TSCC Outbound channel CSBK/MBC/UDT | ✅ verified | 10/10 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer3/cdef_parms.go` |
| §7.1.2 | Short Link Control PDUs | ✅ verified | 3/3 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go` |
| §7.2 | Layer 3 Information Element Coding | ✅ verified | 6/6 passed | `v2/layer3/cdef_parms.go`, `v2/enums/announcement_type.go` |
| §A | Timers, constants levels and addresses | ✅ verified | 5/5 passed | `v2/constants/constants.go` |
| §B.1 | CSBK/MBC/UDT Opcode List | ✅ verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| §B.2 | Short Link Control Opcode List | ✅ verified | 1/1 passed | `v2/enums/slco.go` |

<!-- END ETSI COVERAGE -->
