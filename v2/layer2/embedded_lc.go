package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// LCSSForPosition returns pos.LCSS().
//
// Deprecated: use enums.VoiceBurstType.LCSS.
func LCSSForPosition(pos enums.VoiceBurstType) (lcss enums.LCSS, ok bool) {
	return pos.LCSS()
}

// voiceLCHeaderMask is the Voice LC Header Data Type CRC Mask octet (ETSI
// TS 102 361-1 Table B.21).
const voiceLCHeaderMask = 0x96

// EncodeEmbeddedLC returns the four 32-bit embedded signalling fields, in
// burst order B to E, that carry the 9-octet LC lc, in the form
// RawBurst.SetEmbeddedData takes.
func EncodeEmbeddedLC(lc [9]byte) [4][4]byte {
	var bits [72]bit.Bit
	copy(bits[:], bit.Unpack(lc[:]))
	return fragmentOctets(encodeEmbeddedLC(bits))
}

// DecodeEmbeddedLC decodes the 9 LC octets from the embedded signalling
// fields f of bursts B to E, as RawBurst.EmbeddedData returns them. The FEC
// result sets Uncorrectable when no nearest BPTC codeword, or more than one,
// has a 5-bit checksum that matches its LC. With strict set, it corrects the
// BPTC as MMDVMHost does (CDMREmbeddedData::decodeEmbeddedData).
func DecodeEmbeddedLC(f [4][4]byte, strict bool) ([9]byte, fec.Result) {
	lc, res := decodeEmbeddedLCBits(fragmentBits(f), strict)
	return lcOctets(lc), res
}

func decodeEmbeddedLCBits(fragments [4][32]bit.Bit, strict bool) ([72]bit.Bit, fec.Result) {
	if strict {
		return decodeEmbeddedLCStrict(fragments)
	}
	return decodeEmbeddedLC(fragments)
}

// EmbeddedLCAssembler collects the four embedded LC fragments of voice
// bursts B to E in the order of their LCSS, First, Continuation,
// Continuation and Last, and decodes the LC as DecodeEmbeddedLC does. The
// zero EmbeddedLCAssembler is ready to use. It is not safe for concurrent
// use.
type EmbeddedLCAssembler struct {
	// Strict selects the strict decode of DecodeEmbeddedLC. Reset keeps it.
	Strict bool

	fragments [4][32]bit.Bit
	count     int
}

// Reset discards any partial LC.
func (a *EmbeddedLCAssembler) Reset() {
	*a = EmbeddedLCAssembler{Strict: a.Strict}
}

// Add adds the embedded signalling field frag of a burst whose EMB carries
// lcss. When a Last fragment follows a First and two Continuation
// fragments, Add returns the LC with done set, and ErrEmbeddedLCUncorrectable when its
// FEC result is uncorrectable. Add returns ErrEmbeddedLCSequence when a
// fragment out of that order discards a partial LC.
func (a *EmbeddedLCAssembler) Add(frag [4]byte, lcss enums.LCSS) (lc [9]byte, res fec.Result, done bool, err error) {
	bits, res, done, err := a.add(bit.Unpack32(frag), lcss)
	if !done {
		return lc, res, false, err
	}
	return lcOctets(bits), res, true, err
}

func (a *EmbeddedLCAssembler) add(frag [32]bit.Bit, lcss enums.LCSS) ([72]bit.Bit, fec.Result, bool, error) {
	var want int
	switch lcss {
	case enums.LCSSFirstFragment:
		err := a.discard()
		a.fragments[0], a.count = frag, 1
		return [72]bit.Bit{}, fec.Result{}, false, err
	case enums.LCSSContinuationFragment:
		want = a.count
		if want != 1 && want != 2 {
			return [72]bit.Bit{}, fec.Result{}, false, a.discard()
		}
	case enums.LCSSLastFragment:
		want = 3
	case enums.LCSSSingleFragment:
		return [72]bit.Bit{}, fec.Result{}, false, a.discard()
	default:
		return [72]bit.Bit{}, fec.Result{}, false, a.discard()
	}
	if a.count != want {
		return [72]bit.Bit{}, fec.Result{}, false, a.discard()
	}
	a.fragments[a.count] = frag
	a.count++
	if a.count < 4 {
		return [72]bit.Bit{}, fec.Result{}, false, nil
	}
	a.count = 0
	lc, res := decodeEmbeddedLCBits(a.fragments, a.Strict)
	if res.Uncorrectable {
		return lc, res, true, ErrEmbeddedLCUncorrectable
	}
	return lc, res, true, nil
}

func (a *EmbeddedLCAssembler) discard() error {
	if a.count == 0 {
		return nil
	}
	a.count = 0
	return ErrEmbeddedLCSequence
}

func encodeEmbeddedLC(lc [72]bit.Bit) [4][32]bit.Bit {
	cs := crc.Checksum5(lcOctets(lc))
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

func decodeEmbeddedLC(fragments [4][32]bit.Bit) ([72]bit.Bit, fec.Result) {
	return embeddedLCChecksum(bptc.DecodeEmbeddedWithCheck(fragments, embeddedLCChecksumValid))
}

func embeddedLCChecksumValid(info [77]bit.Bit) bool {
	_, result := embeddedLCChecksum(info, fec.Result{})
	return !result.Uncorrectable
}

func decodeEmbeddedLCStrict(fragments [4][32]bit.Bit) ([72]bit.Bit, fec.Result) {
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
// fields, in burst order B to E, that carry lc. lc.BurstDataType is ignored.
func EncodeEmbeddedFullLinkControl(lc *pdu.FullLinkControl) ([4][4]byte, error) {
	frags, err := encodeEmbeddedFullLinkControl(lc)
	return fragmentOctets(frags), err
}

func encodeEmbeddedFullLinkControl(lc *pdu.FullLinkControl) ([4][32]bit.Bit, error) {
	bits, err := fullLCBits(lc)
	if err != nil {
		return [4][32]bit.Bit{}, err
	}
	return encodeEmbeddedLC(bits), nil
}

// DecodeEmbeddedFullLinkControl decodes a Full LC from the four embedded
// signalling fields f of bursts B to E, as DecodeEmbeddedLC(f, strict) does.
// The LC has BurstDataType DataTypeVoiceLCHeader, and its FEC field combines
// the DecodeEmbeddedLC result and the field decode.
func DecodeEmbeddedFullLinkControl(f [4][4]byte, strict bool) pdu.FullLinkControl {
	return embeddedFullLinkControl(decodeEmbeddedLCBits(fragmentBits(f), strict))
}

func embeddedFullLinkControl(lc [72]bit.Bit, result fec.Result) pdu.FullLinkControl {
	octets := lcOctets(lc)
	codeword := reedsolomon.Encode(octets)
	for i := 9; i < 12; i++ {
		codeword[i] ^= voiceLCHeaderMask
	}
	var data [96]bit.Bit
	copy(data[:], bit.Unpack(codeword[:]))
	flc := pdu.DecodeFullLinkControl(data, enums.DataTypeVoiceLCHeader)
	result.ErrorsCorrected += flc.FEC.ErrorsCorrected
	result.Uncorrectable = result.Uncorrectable || flc.FEC.Uncorrectable
	flc.FEC = result
	return flc
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
