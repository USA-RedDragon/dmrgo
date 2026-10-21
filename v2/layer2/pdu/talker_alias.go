package pdu

import (
	"errors"
	"github.com/USA-RedDragon/dmrgo/v2"
	"slices"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	talkerAliasBlockBits  = 56
	talkerAliasMaxBlocks  = 3
	talkerAliasMaxUnits   = 31
	talkerAliasStreamBits = 49 + talkerAliasMaxBlocks*talkerAliasBlockBits
)

// TalkerAliasMaxText is the length of the longest UTF-8 text of an alias:
// 31 UTF-16 Code Units of 3 bytes each.
const TalkerAliasMaxText = 3 * talkerAliasMaxUnits

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
	length = max(0, min(length, talkerAliasMaxUnits))
	need := length*talkerAliasUnitBits(format) - talkerAliasHeaderBits(format)
	if need <= 0 {
		return 0
	}
	return min((need+talkerAliasBlockBits-1)/talkerAliasBlockBits, talkerAliasMaxBlocks)
}

// TalkerAlias returns the alias carried by a Talker Alias header and its
// blocks, given in order block 1, 2, 3. Only the blocks that the header
// length calls for are used. It returns ErrTalkerAliasIncomplete when fewer
// blocks are given or header or a block is nil, and
// ErrTalkerAliasInvalidUTF8, with the alias, when the Code Units of the
// UTF-8 format are not valid UTF-8. It allocates the string;
// AppendTalkerAlias does not.
func TalkerAlias(header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) (string, error) {
	var buf [TalkerAliasMaxText]byte
	n, err := talkerAliasText(&buf, header, blocks...)
	return string(buf[:n]), err
}

// AppendTalkerAlias appends the alias that TalkerAlias returns to dst as
// UTF-8 and returns the extended slice, with the errors of TalkerAlias. With
// ErrTalkerAliasIncomplete it returns dst unchanged. It allocates only when
// dst has no room.
func AppendTalkerAlias(dst []byte, header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) ([]byte, error) {
	var buf [TalkerAliasMaxText]byte
	n, err := talkerAliasText(&buf, header, blocks...)
	return append(dst, buf[:n]...), err
}

func talkerAliasText(dst *[TalkerAliasMaxText]byte, header *FLCTalkerAliasHeader, blocks ...*FLCTalkerAliasBlock) (int, error) {
	if header == nil {
		return 0, ErrTalkerAliasIncomplete
	}
	format := header.TalkerAliasDataFormat
	n := TalkerAliasBlocks(format, header.TalkerAliasDataLength)
	if len(blocks) < n {
		return 0, ErrTalkerAliasIncomplete
	}
	var stream [(talkerAliasStreamBits + 7) / 8]byte
	used := talkerAliasHeaderBits(format)
	bits.SetField64(stream[:], 0, used, header.TalkerAliasData)
	for _, b := range blocks[:n] {
		if b == nil {
			return 0, ErrTalkerAliasIncomplete
		}
		bits.Copy(stream[:], used, b.TalkerAliasData[:], 0, talkerAliasBlockBits)
		used += talkerAliasBlockBits
	}
	unit := talkerAliasUnitBits(format)
	var units [talkerAliasMaxUnits]uint16
	length := max(0, min(header.TalkerAliasDataLength, used/unit, talkerAliasMaxUnits))
	for i := range length {
		units[i] = uint16(bits.Field(stream[:], i*unit, unit) & 0xFFFF)
	}
	return writeTalkerAliasText(dst, format, units[:length])
}

func writeTalkerAliasText(dst *[TalkerAliasMaxText]byte, format layer3.TalkerAliasDataFormat, units []uint16) (int, error) {
	n := 0
	switch format {
	case layer3.TalkerAliasUTF8:
		for _, u := range units {
			dst[n] = byte(u & 0xFF)
			n++
		}
		if !utf8.Valid(dst[:n]) {
			return n, ErrTalkerAliasInvalidUTF8
		}
		return n, nil
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
			n += utf8.EncodeRune(dst[n:], r)
		}
		return n, nil
	case layer3.TalkerAliasSevenBit, layer3.TalkerAliasISOEightBit:
	}
	for _, u := range units {
		n += utf8.EncodeRune(dst[n:], rune(u))
	}
	return n, nil
}

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

func putUnit(units *[talkerAliasMaxUnits]uint16, n int, u uint16) int {
	if n < len(units) {
		units[n] = u
	}
	return n + 1
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
// It returns an error for a nil dst or p.
func EncodeTalkerAliasInto(dst *[talkerAliasMaxBlocks + 1]FullLinkControl, p *[talkerAliasMaxBlocks + 1]FullLinkControlPayloads, alias string, format layer3.TalkerAliasDataFormat) (int, error) {
	if dst == nil || p == nil {
		return 0, errNil
	}
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
	var stream [(talkerAliasStreamBits + 7) / 8]byte
	for j := range count {
		bits.SetField(stream[:], j*unit, unit, uint64(units[j]))
	}
	skip := talkerAliasHeaderBits(format) - 48
	header.TalkerAliasData = bits.Field64(stream[:], 0, 48+skip)
	dst[0] = FullLinkControl{FLCO: enums.FLCOTalkerAliasHeader, FID: enums.FIDStandardized, TalkerAliasHeader: header}
	opcodes := [talkerAliasMaxBlocks]enums.FLCO{enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3}
	for i := range n {
		block := &p[i+1].flcTalkerAliasBlock
		*block = FLCTalkerAliasBlock{}
		bits.Copy(block.TalkerAliasData[:], 0, stream[:], skip+48+i*talkerAliasBlockBits, talkerAliasBlockBits)
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
	if a == nil {
		return
	}
	*a = TalkerAliasAssembler{}
}

// Add adds a Full LC. LCs that are not SFID Talker Alias header or block
// LCs are ignored. ok is true when the alias is complete; it is reported
// once for each new alias. err is set when a complete alias does not
// decode. It returns at most TalkerAliasMaxText bytes of alias. It
// allocates the string; AppendAdd does not.
func (a *TalkerAliasAssembler) Add(lc *FullLinkControl) (alias string, ok bool, err error) {
	if a == nil {
		return "", false, dmrgo.ErrNilReceiver
	}
	var buf [TalkerAliasMaxText]byte
	n, ok, err := a.AddInto(&buf, lc)
	return string(buf[:n]), ok, err
}

// AppendAdd adds lc as Add does, and appends the alias that Add returns to
// dst as UTF-8 instead of returning it as a string. It returns dst unchanged
// when Add returns an empty alias. It allocates only when dst has no room
// for TalkerAliasMaxText bytes.
func (a *TalkerAliasAssembler) AppendAdd(dst []byte, lc *FullLinkControl) (alias []byte, ok bool, err error) {
	if a == nil {
		return dst, false, dmrgo.ErrNilReceiver
	}
	var buf [TalkerAliasMaxText]byte
	n, ok, err := a.AddInto(&buf, lc)
	return append(dst, buf[:n]...), ok, err
}

// AddInto adds lc as Add does, and writes the alias that Add returns to dst
// as UTF-8 instead of returning it as a string. It returns the length of
// the alias, 0 when Add returns an empty alias. A nil dst returns an error
// and changes nothing.
func (a *TalkerAliasAssembler) AddInto(dst *[TalkerAliasMaxText]byte, lc *FullLinkControl) (n int, ok bool, err error) {
	if a == nil {
		return 0, false, dmrgo.ErrNilReceiver
	}
	if dst == nil {
		return 0, false, errNil
	}
	if lc == nil {
		return 0, false, nil
	}
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
		return 0, false, nil
	}
	if !a.hasHeader || a.completed {
		return 0, false, nil
	}
	nb := TalkerAliasBlocks(a.header.TalkerAliasDataFormat, a.header.TalkerAliasDataLength)
	var blocks [talkerAliasMaxBlocks]*FLCTalkerAliasBlock
	for i := range nb {
		if !a.hasBlock[i] {
			return 0, false, nil
		}
		blocks[i] = &a.blocks[i]
	}
	n, err = talkerAliasText(dst, &a.header, blocks[:nb]...)
	a.completed = true
	return n, err == nil, err
}

var (
	errCharacterFormat = errors.New("a character of the talker alias is not in its format")
	errUnknownFormat   = errors.New("unknown talker alias format")
)
