package pdu

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ETSI TS 102 361-2 §5.4.3, §7.1.1.4, §7.1.1.5, §7.2.18 and §7.2.19:
// Inband talker alias.
//
// The alias is a bit stream of Code Units: the header carries 49 bits for
// the 7-bit format and 48 bits for the other formats (the MSB is reserved),
// and each of blocks 1 to 3 carries 56 bits.

const (
	talkerAliasBlockBits = 56
	talkerAliasMaxBlocks = 3
)

var (
	// ErrTalkerAliasIncomplete is returned when the blocks that the header
	// length calls for have not all been received.
	ErrTalkerAliasIncomplete = errors.New("talker alias blocks missing")
	// ErrTalkerAliasLength is returned when an alias does not fit in a
	// header and three blocks.
	ErrTalkerAliasLength = errors.New("talker alias too long")
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
// (ETSI TS 102 361-2 §5.4.3).
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
// when fewer blocks are given, and an error when the Code Units are not
// valid for the format.
func DecodeTalkerAlias(header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) (string, error) {
	format := header.TalkerAliasDataFormat
	n := TalkerAliasBlocks(format, header.TalkerAliasDataLength)
	if len(blocks) < n {
		return "", ErrTalkerAliasIncomplete
	}
	var headerData [49]bit.Bit
	bit.PutUint(headerData[:], header.TalkerAliasData)
	stream := make([]bit.Bit, 0, 49+n*talkerAliasBlockBits)
	if format == layer3.TalkerAliasSevenBit {
		stream = append(stream, headerData[:]...)
	} else {
		stream = append(stream, headerData[1:]...)
	}
	for _, b := range blocks[:n] {
		if b == nil {
			return "", ErrTalkerAliasIncomplete
		}
		stream = append(stream, bit.Unpack(b.TalkerAliasData[:])...)
	}
	unit := talkerAliasUnitBits(format)
	length := min(header.TalkerAliasDataLength, len(stream)/unit)
	units := make([]uint16, length)
	for i := range units {
		units[i] = uint16(bit.Uint(stream[i*unit : (i+1)*unit])) //nolint:gosec // unit is at most 16 bits
	}
	return talkerAliasString(format, units)
}

func talkerAliasString(format layer3.TalkerAliasDataFormat, units []uint16) (string, error) {
	switch format {
	case layer3.TalkerAliasUTF8:
		b := make([]byte, len(units))
		for i, u := range units {
			b[i] = byte(u & 0xFF)
		}
		if !utf8.Valid(b) {
			return string(b), errors.New("talker alias is not valid UTF-8")
		}
		return string(b), nil
	case layer3.TalkerAliasUTF16:
		return string(utf16.Decode(units)), nil
	case layer3.TalkerAliasSevenBit, layer3.TalkerAliasISOEightBit:
	}
	var sb strings.Builder
	for _, u := range units {
		sb.WriteRune(rune(u))
	}
	return sb.String(), nil
}

func talkerAliasUnits(alias string, format layer3.TalkerAliasDataFormat) ([]uint16, error) {
	switch format {
	case layer3.TalkerAliasUTF8:
		units := make([]uint16, len(alias))
		for i := 0; i < len(alias); i++ {
			units[i] = uint16(alias[i])
		}
		return units, nil
	case layer3.TalkerAliasUTF16:
		return utf16.Encode([]rune(alias)), nil
	case layer3.TalkerAliasSevenBit, layer3.TalkerAliasISOEightBit:
		limit := rune(0xFF)
		if format == layer3.TalkerAliasSevenBit {
			limit = 0x7F
		}
		units := make([]uint16, 0, len(alias))
		for _, r := range alias {
			if r > limit {
				return nil, fmt.Errorf("character %q is not in the %s format", r, format.String())
			}
			units = append(units, uint16(r)) //nolint:gosec // r <= limit <= 0xFF
		}
		return units, nil
	}
	return nil, fmt.Errorf("unknown talker alias format %d", format)
}

// EncodeTalkerAlias returns the Talker Alias header LC and the block LCs
// that carry alias in the given format, with the SFID. DataType is not set.
// It returns ErrTalkerAliasLength when the alias needs more than 31 Code
// Units or more than three blocks, and an error when a character is not in
// the format.
func EncodeTalkerAlias(alias string, format layer3.TalkerAliasDataFormat) ([]FullLinkControl, error) {
	units, err := talkerAliasUnits(alias, format)
	if err != nil {
		return nil, err
	}
	unit := talkerAliasUnitBits(format)
	headerBits := talkerAliasHeaderBits(format)
	if len(units) > 31 || len(units)*unit > headerBits+talkerAliasMaxBlocks*talkerAliasBlockBits {
		return nil, ErrTalkerAliasLength
	}
	n := TalkerAliasBlocks(format, len(units))
	stream := make([]bit.Bit, headerBits+n*talkerAliasBlockBits)
	for i, u := range units {
		bit.PutUint(stream[i*unit:(i+1)*unit], uint64(u))
	}
	header := &FLCTalkerAliasHeader{TalkerAliasDataFormat: format, TalkerAliasDataLength: len(units)}
	if format == layer3.TalkerAliasSevenBit {
		header.TalkerAliasData = bit.Uint(stream[:49])
		stream = stream[1:]
	} else {
		header.TalkerAliasData = bit.Uint(stream[:48])
	}
	out := []FullLinkControl{{FLCO: enums.FLCOTalkerAliasHeader, FID: enums.FIDStandardized, TalkerAliasHeader: header}}
	opcodes := [talkerAliasMaxBlocks]enums.FLCO{enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3}
	for i := range n {
		block := &FLCTalkerAliasBlock{}
		start := 48 + i*talkerAliasBlockBits
		copy(block.TalkerAliasData[:], bit.Pack(stream[start:start+talkerAliasBlockBits]))
		out = append(out, FullLinkControl{FLCO: opcodes[i], FID: enums.FIDStandardized, TalkerAliasBlock: block})
	}
	return out, nil
}

// TalkerAliasAssembler collects Talker Alias header and block LCs, as
// received in any order in the embedded LC of a voice call, and decodes the
// alias once the header and all the blocks it calls for are present. Blocks
// received before the first header are kept; a later header that differs
// from the stored one discards the stored blocks. The zero value is ready
// to use.
type TalkerAliasAssembler struct {
	header    *FLCTalkerAliasHeader
	blocks    [talkerAliasMaxBlocks]*FLCTalkerAliasBlock
	completed bool
}

// Reset discards the stored header and blocks.
func (a *TalkerAliasAssembler) Reset() {
	*a = TalkerAliasAssembler{}
}

// Add adds a Full LC. LCs that are not SFID Talker Alias header or block
// LCs are ignored. ok is true when the alias is complete; it is reported
// once for each new alias. err is set when a complete alias does not
// decode.
func (a *TalkerAliasAssembler) Add(lc *FullLinkControl) (alias string, ok bool, err error) {
	switch {
	case lc.FLCO == enums.FLCOTalkerAliasHeader && lc.TalkerAliasHeader != nil:
		h := *lc.TalkerAliasHeader
		if a.header == nil || *a.header != h {
			if a.header != nil {
				a.blocks = [talkerAliasMaxBlocks]*FLCTalkerAliasBlock{}
			}
			a.header = &h
			a.completed = false
		}
	case lc.TalkerAliasBlock != nil && lc.FLCO >= enums.FLCOTalkerAliasBlock1 && lc.FLCO <= enums.FLCOTalkerAliasBlock3:
		b := *lc.TalkerAliasBlock
		i := int(lc.FLCO - enums.FLCOTalkerAliasBlock1)
		if a.blocks[i] == nil || *a.blocks[i] != b {
			a.blocks[i] = &b
			a.completed = false
		}
	default:
		return "", false, nil
	}
	if a.header == nil || a.completed {
		return "", false, nil
	}
	alias, err = DecodeTalkerAlias(a.header, a.blocks[:TalkerAliasBlocks(a.header.TalkerAliasDataFormat, a.header.TalkerAliasDataLength)]...)
	if errors.Is(err, ErrTalkerAliasIncomplete) {
		return "", false, nil
	}
	a.completed = true
	return alias, err == nil, err
}
