package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 Table B.21
const voiceLCHeaderMask = 0x96

// EncodeEmbeddedLC returns the four 32-bit embedded signalling fields, in
// burst order B to E, that carry the 9-octet LC lc, in the form
// RawBurst.SetEmbeddedData takes.
func EncodeEmbeddedLC(lc [9]byte) [4][4]byte {
	return fragmentOctets(encodeEmbeddedLC(lc))
}

// DecodeEmbeddedLC decodes the 9 LC octets from the embedded signalling
// fields f of bursts B to E, as RawBurst.EmbeddedData returns them. The FEC
// result sets Uncorrectable when no nearest BPTC codeword, or more than one,
// has a 5-bit checksum that matches its LC.
func DecodeEmbeddedLC(f [4][4]byte) ([9]byte, fec.Result) {
	return decodeEmbeddedLC(fragmentWords(f))
}

func decodeEmbeddedLC(fragments [4]uint32) ([9]byte, fec.Result) {
	return embeddedLCChecksum(bptc.DecodeEmbeddedWithCheck(fragments, embeddedLCChecksumValid))
}

// EmbeddedLCAssembler collects the four embedded LC fragments of voice
// bursts B to E in the order of their LCSS, First, Continuation,
// Continuation and Last, and decodes the LC as DecodeEmbeddedLC does. The
// zero EmbeddedLCAssembler is ready to use. It is not safe for concurrent
// use.
type EmbeddedLCAssembler struct {
	fragments [4]uint32
	count     int
}

// Reset discards any partial LC.
func (a *EmbeddedLCAssembler) Reset() {
	if a == nil {
		return
	}
	*a = EmbeddedLCAssembler{}
}

// Add adds the embedded signalling field frag of a burst whose EMB carries
// lcss. When a Last fragment follows a First and two Continuation
// fragments, Add returns the LC with done set, and
// ErrEmbeddedLCUncorrectable when its FEC result is uncorrectable. Add
// returns ErrEmbeddedLCSequence when a fragment out of that order discards a
// partial LC.
func (a *EmbeddedLCAssembler) Add(frag [4]byte, lcss enums.LCSS) (lc [9]byte, res fec.Result, done bool, err error) {
	if a == nil {
		return [9]byte{}, fec.Result{}, false, dmrgo.ErrNilReceiver
	}
	return a.add(fragmentWord(frag), lcss)
}

func (a *EmbeddedLCAssembler) add(frag uint32, lcss enums.LCSS) ([9]byte, fec.Result, bool, error) {
	var want int
	switch lcss {
	case enums.LCSSFirstFragment:
		err := a.discard()
		a.fragments[0], a.count = frag, 1
		return [9]byte{}, fec.Result{}, false, err
	case enums.LCSSContinuationFragment:
		want = a.count
		if want != 1 && want != 2 {
			return [9]byte{}, fec.Result{}, false, a.discard()
		}
	case enums.LCSSLastFragment:
		want = 3
	case enums.LCSSSingleFragment:
		return [9]byte{}, fec.Result{}, false, a.discard()
	default:
		return [9]byte{}, fec.Result{}, false, a.discard()
	}
	if a.count != want {
		return [9]byte{}, fec.Result{}, false, a.discard()
	}
	a.fragments[a.count] = frag
	a.count++
	if a.count < 4 {
		return [9]byte{}, fec.Result{}, false, nil
	}
	a.count = 0
	lc, res := decodeEmbeddedLC(a.fragments)
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

// ETSI TS 102 361-1 clause B.2.1
func encodeEmbeddedLC(lc [9]byte) [4]uint32 {
	cs := crc.Checksum5(lc)
	var info [10]byte
	bits.SetField(info[:], 0, 22, bits.Field(lc[:], 0, 22))
	for row := 2; row < 7; row++ {
		bits.SetField(info[:], row*11, 10, bits.Field(lc[:], 22+10*(row-2), 10))
		bits.SetBit(info[:], row*11+10, cs>>(6-row))
	}
	return bptc.EncodeEmbedded(info)
}

func embeddedLCChecksumValid(info [10]byte) bool {
	_, result := embeddedLCChecksum(info, fec.Result{})
	return !result.Uncorrectable
}

func embeddedLCChecksum(info [10]byte, result fec.Result) ([9]byte, fec.Result) {
	var lc [9]byte
	var cs uint8
	bits.SetField(lc[:], 0, 22, bits.Field(info[:], 0, 22))
	for row := 2; row < 7; row++ {
		bits.SetField(lc[:], 22+10*(row-2), 10, bits.Field(info[:], row*11, 10))
		cs = cs<<1 | bits.Bit(info[:], row*11+10)
	}
	if !crc.CheckChecksum5(lc, cs) {
		result.Uncorrectable = true
	}
	return lc, result
}

// EncodeEmbeddedFullLinkControl returns the four embedded signalling
// fields, in burst order B to E, that carry lc. lc.BurstDataType is ignored.
// It returns dmrgo.ErrNilArgument for a nil lc.
func EncodeEmbeddedFullLinkControl(lc *pdu.FullLinkControl) ([4][4]byte, error) {
	if lc == nil {
		return [4][4]byte{}, dmrgo.ErrNilArgument
	}
	frags, err := encodeEmbeddedFullLinkControl(lc)
	return fragmentOctets(frags), err
}

func encodeEmbeddedFullLinkControl(lc *pdu.FullLinkControl) ([4]uint32, error) {
	octets, err := lc.Bytes()
	if err != nil {
		return [4]uint32{}, err
	}
	return encodeEmbeddedLC(octets), nil
}

// DecodeEmbeddedFullLinkControl decodes a Full LC from the four embedded
// signalling fields f of bursts B to E, as DecodeEmbeddedLC(f) does.
// The LC has BurstDataType DataTypeVoiceLCHeader, and its FEC field combines
// the DecodeEmbeddedLC result and the field decode.
func DecodeEmbeddedFullLinkControl(f [4][4]byte) pdu.FullLinkControl {
	lc, result := decodeEmbeddedLC(fragmentWords(f))
	flc := pdu.DecodeFullLinkControl(embeddedFullLCCodeword(lc), enums.DataTypeVoiceLCHeader)
	flc.FEC = combineEmbeddedFEC(result, flc.FEC)
	return flc
}

func embeddedFullLCCodeword(lc [9]byte) [12]byte {
	codeword := reedsolomon.Encode(lc)
	for i := 9; i < 12; i++ {
		codeword[i] ^= voiceLCHeaderMask
	}
	return codeword
}

func combineEmbeddedFEC(result, field fec.Result) fec.Result {
	result.ErrorsCorrected += field.ErrorsCorrected
	result.Uncorrectable = result.Uncorrectable || field.Uncorrectable
	return result
}
