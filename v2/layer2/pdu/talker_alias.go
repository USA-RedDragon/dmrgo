package pdu

import (
	"errors"
	"slices"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	talkerAliasBlockBits = 56
	talkerAliasMaxBlocks = 3
	talkerAliasMaxUnits  = 31
	// talkerAliasStreamBits is the 49-bit header data and three blocks.
	talkerAliasStreamBits = 49 + talkerAliasMaxBlocks*talkerAliasBlockBits
	// talkerAliasMaxText is the longest UTF-8 text of an alias: 31 UTF-16
	// Code Units of 3 bytes each.
	talkerAliasMaxText = 3 * talkerAliasMaxUnits
)

func talkerAliasUnitBits(format layer3.TalkerAliasDataFormat) int {
	switch format {
	case layer3.TalkerAliasSevenBit:
		return 7
	case layer3.TalkerAliasUTF16:
		return 16
	case layer3.TalkerAliasISOEightBit, layer3.TalkerAliasUTF8:
		return 8
	}
	return 8
}

func talkerAliasHeaderBits(format layer3.TalkerAliasDataFormat) int {
	if format == layer3.TalkerAliasSevenBit {
		return 49
	}
	return 48
}

// TalkerAliasBlocks returns the number of Talker Alias block LCs (0 to 3)
// that follow a header with the given format and length in Code Units
// (ETSI TS 102 361-2 clause 5.4.3).
func TalkerAliasBlocks(format layer3.TalkerAliasDataFormat, length int) int {
	need := length*talkerAliasUnitBits(format) - talkerAliasHeaderBits(format)
	if need <= 0 {
		return 0
	}
	return min((need+talkerAliasBlockBits-1)/talkerAliasBlockBits, talkerAliasMaxBlocks)
}

// DecodeTalkerAlias returns the alias carried by a Talker Alias header and
// its blocks, given in order block 1, 2, 3. Only the blocks that the
// header length calls for are used. It returns ErrTalkerAliasIncomplete
// when fewer blocks are given, and ErrTalkerAliasInvalidUTF8, with the
// alias, when the Code Units of the UTF-8 format are not valid UTF-8. It
// allocates the string; AppendTalkerAlias does not.
func DecodeTalkerAlias(header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) (string, error) {
	var buf [talkerAliasMaxText]byte
	b, err := AppendTalkerAlias(buf[:0], header, blocks...)
	return string(b), err
}

// AppendTalkerAlias appends the alias that DecodeTalkerAlias returns to dst
// as UTF-8 and returns the extended slice, with the errors of
// DecodeTalkerAlias. With ErrTalkerAliasIncomplete it returns dst unchanged.
func AppendTalkerAlias(dst []byte, header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) ([]byte, error) {
	format := header.TalkerAliasDataFormat
	n := TalkerAliasBlocks(format, header.TalkerAliasDataLength)
	if len(blocks) < n {
		return dst, ErrTalkerAliasIncomplete
	}
	var stream [talkerAliasStreamBits]bit.Bit
	used := talkerAliasHeaderBits(layer3.TalkerAliasSevenBit)
	bit.PutUint(stream[:used], header.TalkerAliasData)
	if format != layer3.TalkerAliasSevenBit {
		used = copy(stream[:], stream[1:used])
	}
	for _, b := range blocks[:n] {
		if b == nil {
			return dst, ErrTalkerAliasIncomplete
		}
		unpackBits(stream[used:used+talkerAliasBlockBits], b.TalkerAliasData[:])
		used += talkerAliasBlockBits
	}
	unit := talkerAliasUnitBits(format)
	var units [talkerAliasMaxUnits]uint16
	length := max(0, min(header.TalkerAliasDataLength, used/unit, talkerAliasMaxUnits))
	for i := range length {
		units[i] = uint16(bit.Uint(stream[i*unit:(i+1)*unit]) & 0xFFFF)
	}
	return appendTalkerAliasText(dst, format, units[:length])
}

// appendTalkerAliasText appends the Code Units of format to dst as UTF-8.
// An unpaired UTF-16 surrogate becomes U+FFFD, as in utf16.Decode.
func appendTalkerAliasText(dst []byte, format layer3.TalkerAliasDataFormat, units []uint16) ([]byte, error) {
	switch format {
	case layer3.TalkerAliasUTF8:
		start := len(dst)
		for _, u := range units {
			dst = append(dst, byte(u&0xFF))
		}
		if !utf8.Valid(dst[start:]) {
			return dst, ErrTalkerAliasInvalidUTF8
		}
		return dst, nil
	case layer3.TalkerAliasUTF16:
		for i := 0; i < len(units); i++ {
			r := rune(units[i])
			if utf16.IsSurrogate(r) {
				r = utf8.RuneError
				if i+1 < len(units) {
					if pair := utf16.DecodeRune(rune(units[i]), rune(units[i+1])); pair != utf8.RuneError {
						r = pair
						i++
					}
				}
			}
			dst = utf8.AppendRune(dst, r)
		}
		return dst, nil
	case layer3.TalkerAliasSevenBit, layer3.TalkerAliasISOEightBit:
	}
	for _, u := range units {
		dst = utf8.AppendRune(dst, rune(u))
	}
	return dst, nil
}

// talkerAliasUnits writes the first Code Units of alias in format into
// units and returns the number of Code Units of the whole alias.
func talkerAliasUnits(units *[talkerAliasMaxUnits]uint16, alias string, format layer3.TalkerAliasDataFormat) (int, error) {
	n := 0
	switch format {
	case layer3.TalkerAliasUTF8:
		for i := 0; i < len(alias); i++ {
			n = putUnit(units, n, uint16(alias[i]))
		}
		return n, nil
	case layer3.TalkerAliasUTF16:
		for _, r := range alias {
			if r < 0x10000 {
				n = putUnit(units, n, uint16(r&0xFFFF))
				continue
			}
			r1, r2 := utf16.EncodeRune(r)
			n = putUnit(units, n, uint16(r1&0xFFFF))
			n = putUnit(units, n, uint16(r2&0xFFFF))
		}
		return n, nil
	case layer3.TalkerAliasSevenBit, layer3.TalkerAliasISOEightBit:
		limit := rune(0xFF)
		if format == layer3.TalkerAliasSevenBit {
			limit = 0x7F
		}
		for _, r := range alias {
			if r > limit {
				return 0, errCharacterFormat
			}
			n = putUnit(units, n, uint16(r&0xFFFF))
		}
		return n, nil
	}
	return 0, errUnknownFormat
}

// putUnit sets units[n] to u when it fits, and returns n+1.
func putUnit(units *[talkerAliasMaxUnits]uint16, n int, u uint16) int {
	if n < len(units) {
		units[n] = u
	}
	return n + 1
}

// streamBits writes bits first to first+len(dst)-1 of the Code Units of
// units, each unit bits long, most significant bit first, into dst. Bits
// past the units are 0.
func streamBits(dst []bit.Bit, units *[talkerAliasMaxUnits]uint16, count, unit, first int) {
	for i := range dst {
		k := first + i
		dst[i] = 0
		if j := k / unit; j < count {
			dst[i] = bit.Bit(units[j] >> (unit - 1 - k%unit) & 1)
		}
	}
}

// EncodeTalkerAlias returns the Talker Alias header LC and the block LCs
// that carry alias in the given format, with the SFID. DataType is not set.
// It returns ErrTalkerAliasLength when the alias needs more than 31 Code
// Units or more than three blocks, and an error when a character is not in
// the format. It allocates the LCs and their payloads;
// EncodeTalkerAliasInto does not.
func EncodeTalkerAlias(alias string, format layer3.TalkerAliasDataFormat) ([]FullLinkControl, error) {
	var lcs [talkerAliasMaxBlocks + 1]FullLinkControl
	p := new([talkerAliasMaxBlocks + 1]FullLinkControlPayloads)
	n, err := EncodeTalkerAliasInto(&lcs, p, alias, format)
	if err != nil {
		return nil, err
	}
	return slices.Clone(lcs[:n]), nil
}

// EncodeTalkerAliasInto writes the LCs that EncodeTalkerAlias returns into
// dst and returns their number, with the errors of EncodeTalkerAlias. The
// payload of dst[i] is in p[i] and stays valid until the next use of p.
// dst and p must not be nil.
func EncodeTalkerAliasInto(dst *[talkerAliasMaxBlocks + 1]FullLinkControl, p *[talkerAliasMaxBlocks + 1]FullLinkControlPayloads, alias string, format layer3.TalkerAliasDataFormat) (int, error) {
	var units [talkerAliasMaxUnits]uint16
	count, err := talkerAliasUnits(&units, alias, format)
	if err != nil {
		return 0, err
	}
	unit := talkerAliasUnitBits(format)
	headerBits := talkerAliasHeaderBits(format)
	if count > talkerAliasMaxUnits || count*unit > headerBits+talkerAliasMaxBlocks*talkerAliasBlockBits {
		return 0, ErrTalkerAliasLength
	}
	n := TalkerAliasBlocks(format, count)
	header := &p[0].flcTalkerAliasHeader
	*header = FLCTalkerAliasHeader{TalkerAliasDataFormat: format, TalkerAliasDataLength: count}
	var head [49]bit.Bit
	skip := 0
	if format == layer3.TalkerAliasSevenBit {
		streamBits(head[:], &units, count, unit, 0)
		header.TalkerAliasData = bit.Uint(head[:])
		skip = 1
	} else {
		streamBits(head[:48], &units, count, unit, 0)
		header.TalkerAliasData = bit.Uint(head[:48])
	}
	dst[0] = FullLinkControl{FLCO: enums.FLCOTalkerAliasHeader, FID: enums.FIDStandardized, TalkerAliasHeader: header}
	opcodes := [talkerAliasMaxBlocks]enums.FLCO{enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3}
	for i := range n {
		block := &p[i+1].flcTalkerAliasBlock
		*block = FLCTalkerAliasBlock{}
		var bits [talkerAliasBlockBits]bit.Bit
		streamBits(bits[:], &units, count, unit, skip+48+i*talkerAliasBlockBits)
		packBits(block.TalkerAliasData[:], bits[:])
		dst[i+1] = FullLinkControl{FLCO: opcodes[i], FID: enums.FIDStandardized, TalkerAliasBlock: block}
	}
	return n + 1, nil
}

// TalkerAliasAssembler collects Talker Alias header and block LCs in any
// order and decodes the alias once the header and all its blocks are present.
// Blocks that arrive before the first header are kept; a header that differs
// from the stored one discards the stored blocks. The zero value is ready to
// use.
type TalkerAliasAssembler struct {
	header    FLCTalkerAliasHeader
	hasHeader bool
	blocks    [talkerAliasMaxBlocks]FLCTalkerAliasBlock
	hasBlock  [talkerAliasMaxBlocks]bool
	completed bool
}

// Reset discards the stored header and blocks.
func (a *TalkerAliasAssembler) Reset() {
	*a = TalkerAliasAssembler{}
}

// Add adds a Full LC. LCs that are not SFID Talker Alias header or block
// LCs are ignored. ok is true when the alias is complete; it is reported
// once for each new alias. err is set when a complete alias does not
// decode. It allocates the string; AppendAdd does not.
func (a *TalkerAliasAssembler) Add(lc *FullLinkControl) (alias string, ok bool, err error) {
	var buf [talkerAliasMaxText]byte
	b, ok, err := a.AppendAdd(buf[:0], lc)
	return string(b), ok, err
}

// AppendAdd adds lc as Add does, and appends the alias that Add returns to
// dst as UTF-8 instead of returning it as a string. It returns dst
// unchanged when Add returns an empty alias.
func (a *TalkerAliasAssembler) AppendAdd(dst []byte, lc *FullLinkControl) (alias []byte, ok bool, err error) {
	switch {
	case lc.FLCO == enums.FLCOTalkerAliasHeader && lc.TalkerAliasHeader != nil:
		h := *lc.TalkerAliasHeader
		if !a.hasHeader || a.header != h {
			if a.hasHeader {
				a.hasBlock = [talkerAliasMaxBlocks]bool{}
			}
			a.header = h
			a.hasHeader = true
			a.completed = false
		}
	case lc.TalkerAliasBlock != nil && lc.FLCO >= enums.FLCOTalkerAliasBlock1 && lc.FLCO <= enums.FLCOTalkerAliasBlock3:
		b := *lc.TalkerAliasBlock
		i := int(lc.FLCO - enums.FLCOTalkerAliasBlock1)
		if !a.hasBlock[i] || a.blocks[i] != b {
			a.blocks[i] = b
			a.hasBlock[i] = true
			a.completed = false
		}
	default:
		return dst, false, nil
	}
	if !a.hasHeader || a.completed {
		return dst, false, nil
	}
	n := TalkerAliasBlocks(a.header.TalkerAliasDataFormat, a.header.TalkerAliasDataLength)
	var blocks [talkerAliasMaxBlocks]*FLCTalkerAliasBlock
	for i := range n {
		if !a.hasBlock[i] {
			return dst, false, nil
		}
		blocks[i] = &a.blocks[i]
	}
	out, err := AppendTalkerAlias(dst, &a.header, blocks[:n]...)
	a.completed = true
	return out, err == nil, err
}

// The errors of EncodeTalkerAlias for an alias that its format cannot carry.
var (
	errCharacterFormat = errors.New("a character of the talker alias is not in its format")
	errUnknownFormat   = errors.New("unknown talker alias format")
)
