package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §7.1.3 — Embedded signalling
//
// Voice bursts B to E of a superframe carry one 72-bit Full LC in four
// 32-bit embedded signalling fragments with LCSS First, Continuation,
// Continuation and Last. The LC and its 5-bit checksum (§B.3.11) fill the
// 77 information bits of the variable length BPTC (§B.2.1): per Figure B.3
// rows 0 and 1 hold 11 LC bits and rows 2 to 6 hold 10 LC bits followed by
// one checksum bit, most significant checksum bit first. The embedded LC has
// no RS(12,9) parity.

// voiceLCHeaderMask is the Voice LC Header Data Type CRC Mask octet (Table
// B.21).
const voiceLCHeaderMask = 0x96

// EncodeEmbeddedLC returns the four 32-bit embedded signalling fragments, in
// burst order B to E, that carry the 72 LC bits of a Full LC.
func EncodeEmbeddedLC(lc [72]bit.Bit) [4][32]bit.Bit {
	cs := crc.CalculateChecksum5(lcOctets(lc))
	var info [77]bit.Bit
	n := 0
	for row := 0; row < 7; row++ {
		if row < 2 {
			copy(info[row*11:row*11+11], lc[n:n+11])
			n += 11
			continue
		}
		copy(info[row*11:row*11+10], lc[n:n+10])
		n += 10
		info[row*11+10] = bit.Bit((cs >> (6 - row)) & 1)
	}
	return bptc.EncodeEmbedded(info)
}

// DecodeEmbeddedLC decodes the 72 LC bits from the four embedded signalling
// fragments of bursts B to E. It decodes the BPTC to the nearest codewords
// with bptc.DecodeEmbeddedWithCheck and keeps the one whose 5-bit checksum
// matches its LC. The FEC result covers the BPTC; Uncorrectable is set when
// no nearest codeword or more than one has a matching checksum.
func DecodeEmbeddedLC(fragments [4][32]bit.Bit) ([72]bit.Bit, fec.Result) {
	return embeddedLCChecksum(bptc.DecodeEmbeddedWithCheck(fragments, embeddedLCChecksumValid))
}

func embeddedLCChecksumValid(info [77]bit.Bit) bool {
	_, result := embeddedLCChecksum(info, fec.Result{})
	return !result.Uncorrectable
}

// DecodeEmbeddedLCStrict is DecodeEmbeddedLC with the BPTC decoded by
// bptc.DecodeEmbeddedStrict, so it accepts and returns the same LCs as
// MMDVMHost (CDMREmbeddedData::decodeEmbeddedData).
func DecodeEmbeddedLCStrict(fragments [4][32]bit.Bit) ([72]bit.Bit, fec.Result) {
	return embeddedLCChecksum(bptc.DecodeEmbeddedStrict(fragments))
}

func embeddedLCChecksum(info [77]bit.Bit, result fec.Result) ([72]bit.Bit, fec.Result) {
	var lc [72]bit.Bit
	var cs uint8
	n := 0
	for row := 0; row < 7; row++ {
		if row < 2 {
			copy(lc[n:n+11], info[row*11:row*11+11])
			n += 11
			continue
		}
		copy(lc[n:n+10], info[row*11:row*11+10])
		n += 10
		cs = cs<<1 | uint8(info[row*11+10])
	}
	if !crc.CheckChecksum5(lcOctets(lc), cs) {
		result.Uncorrectable = true
	}
	return lc, result
}

// EncodeEmbeddedFullLinkControl returns the four embedded signalling
// fragments, in burst order B to E, that carry lc. lc.BurstDataType is ignored.
func EncodeEmbeddedFullLinkControl(lc *pdu.FullLinkControl) ([4][32]bit.Bit, error) {
	bits, err := fullLCBits(lc)
	if err != nil {
		return [4][32]bit.Bit{}, err
	}
	return EncodeEmbeddedLC(bits), nil
}

// DecodeEmbeddedFullLinkControl decodes a Full LC from the four embedded
// signalling fragments of bursts B to E. The returned LC has DataType
// DataTypeVoiceLCHeader, so it encodes as a Voice LC Header as is, and its
// FEC is the DecodeEmbeddedLC result, which is also returned.
func DecodeEmbeddedFullLinkControl(fragments [4][32]bit.Bit) (pdu.FullLinkControl, fec.Result) {
	return embeddedFullLinkControl(DecodeEmbeddedLC(fragments))
}

// DecodeEmbeddedFullLinkControlStrict is DecodeEmbeddedFullLinkControl with
// the LC decoded by DecodeEmbeddedLCStrict.
func DecodeEmbeddedFullLinkControlStrict(fragments [4][32]bit.Bit) (pdu.FullLinkControl, fec.Result) {
	return embeddedFullLinkControl(DecodeEmbeddedLCStrict(fragments))
}

func embeddedFullLinkControl(lc [72]bit.Bit, result fec.Result) (pdu.FullLinkControl, fec.Result) {
	octets := lcOctets(lc)
	codeword := reedsolomon.Encode(octets)
	for i := 9; i < 12; i++ {
		codeword[i] ^= voiceLCHeaderMask
	}
	var data [96]bit.Bit
	copy(data[:], bit.Unpack(codeword[:]))
	flc, rs := pdu.DecodeFullLinkControl(data, enums.DataTypeVoiceLCHeader)
	result.ErrorsCorrected += rs.ErrorsCorrected
	result.Uncorrectable = result.Uncorrectable || rs.Uncorrectable
	flc.FEC = result
	return flc, result
}

// fullLCBits returns the 72 LC bits of lc, without RS(12,9) parity.
func fullLCBits(lc *pdu.FullLinkControl) ([72]bit.Bit, error) {
	header := *lc
	header.BurstDataType = enums.DataTypeVoiceLCHeader
	data, err := header.Encode()
	if err != nil {
		return [72]bit.Bit{}, fmt.Errorf("cannot encode full link control: %w", err)
	}
	var bits [72]bit.Bit
	copy(bits[:], data[:72])
	return bits, nil
}

func lcOctets(lc [72]bit.Bit) [9]byte {
	var octets [9]byte
	copy(octets[:], bit.Pack(lc[:]))
	return octets
}
