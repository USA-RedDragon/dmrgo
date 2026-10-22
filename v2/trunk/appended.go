package trunk

import (
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
)

// MaxAppendedOctets is the most octets of appended data in a UDT: four
// blocks of 12 octets, less the CRC of the last block.
const MaxAppendedOctets = 4*12 - 2

const maxBinaryBits = 4*96 - 16 - 1

// Appended is the appended data of a UDT with the fields of its header
// that describe it: the UDT Format, the pad nibbles and the number of
// blocks. The Set methods fill it for one format, and the methods named
// after a format read it. UDT.Appended returns it for a received UDT, and
// MS.SendShortData sends it. (ETSI TS 102 361-4 clause 6.5.1 and Annex B.3)
type Appended struct {
	// Format is the UDT Format of the data.
	Format enums.UDTFormat
	// PadNibble is the number of pad nibbles after the user data.
	PadNibble uint8
	// Blocks is the number of appended blocks, 1 to 4.
	Blocks int
	// Data holds the 12*Blocks-2 octets of the blocks before the CRC of the
	// last block.
	Data [MaxAppendedOctets]byte
}

// AddressList is the appended data of a UDT in the address format: up to
// 15 MS or talkgroup addresses (ETSI TS 102 361-4 clause B.3.2).
type AddressList struct {
	// OneKey reports that Addresses[0] is the One-key_talkgroup of DGNA.
	OneKey bool
	// Addresses holds the first Len addresses. constants.AddressNull marks
	// an unused entry.
	Addresses [15]uint32
	// Len is the number of entries of Addresses in use: 3, 7, 11 or 15 for
	// 1 to 4 blocks.
	Len int
}

func appendedBits(blocks int) int {
	return 96*blocks - 16
}

func address(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func padNibbles(free int) uint8 {
	if free < 0 || free >= 4*32 {
		return 0
	}
	return uint8(free / 4)
}

func blocksFor(n int) int {
	for b := 1; b <= 4; b++ {
		if n <= appendedBits(b) {
			return b
		}
	}
	return 0
}

func (a *Appended) reset(f enums.UDTFormat, blocks int) {
	*a = Appended{Format: f, Blocks: blocks}
}

func (a *Appended) size() (int, error) {
	if a == nil || a.Blocks < 1 || a.Blocks > 4 {
		return 0, ErrUDTLength
	}
	return appendedBits(a.Blocks), nil
}

func (a *Appended) check(f enums.UDTFormat) (int, error) {
	if a.Format != f {
		return 0, ErrUDTFormat
	}
	return a.size()
}

func (a *Appended) pad(from int) {
	for i := from; i < appendedBits(a.Blocks); i++ {
		bits.SetBit(a.Data[:], i, 1)
	}
}

// SetBinary sets a to the first n bits of b, packed most significant bit
// first, in the binary format with the end marker for variable length
// data: a 0 bit after the data and 1 bits to the end of the last block. n
// is 1 to 367. It returns ErrUDTLength for n out of range or a b too short
// for n. (ETSI TS 102 361-4 clause B.3.1)
func (a *Appended) SetBinary(b []byte, n int) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	if n < 1 || n > maxBinaryBits || len(b)*8 < n {
		return ErrUDTLength
	}
	a.reset(enums.UDTBinary, blocksFor(n+1))
	bits.Copy(a.Data[:], 0, b, 0, n)
	a.pad(n + 1)
	return nil
}

// Binary appends the data of a in the binary format to dst, packed most
// significant bit first, and returns dst and the number of bits. It finds
// the end of the data from the end marker of SetBinary; read Data for
// binary data of a fixed length. It returns ErrUDTFormat for another
// format and ErrUDTLength when a has no end marker or no data before it.
// It allocates only to grow dst. (ETSI TS 102 361-4 clause B.3.1)
func (a *Appended) Binary(dst []byte) ([]byte, int, error) {
	if a == nil {
		return dst, 0, dmrgo.ErrNilReceiver
	}

	total, err := a.check(enums.UDTBinary)
	if err != nil {
		return dst, 0, err
	}
	n := total - 1
	for n >= 0 && bits.Bit(a.Data[:], n) == 1 {
		n--
	}
	if n < 1 {
		return dst, 0, ErrUDTLength
	}
	k := len(dst)
	for range (n + 7) / 8 {
		dst = append(dst, 0)
	}
	bits.Copy(dst[k:], 0, a.Data[:], 0, n)
	return dst, n, nil
}

// SetAddresses sets a to the 1 to 15 addresses of list in the address
// format, in the fewest blocks that hold them. The other entries of the
// last block are constants.AddressNull. oneKey marks the first address as
// the One-key_talkgroup of DGNA. It returns ErrUDTLength for a list out of
// range, or for an address over 24 bits. (ETSI TS 102 361-4 clause B.3.2)
func (a *Appended) SetAddresses(oneKey bool, list []uint32) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	if len(list) < 1 || len(list) > 15 {
		return ErrUDTLength
	}
	a.reset(enums.UDTAddress, (len(list)+1+3)/4)
	if oneKey {
		bits.SetBit(a.Data[:], 7, 1)
	}
	for i, v := range list {
		if v > 0xFFFFFF {
			return ErrUDTLength
		}
		bits.SetField(a.Data[:], 8+24*i, 24, uint64(v))
	}
	return nil
}

// Addresses returns the addresses of a in the address format. It returns
// ErrUDTFormat for another format and ErrUDTLength for a Blocks out of
// range. (ETSI TS 102 361-4 clause B.3.2)
func (a *Appended) Addresses() (AddressList, error) {
	if a == nil {
		return AddressList{}, dmrgo.ErrNilReceiver
	}

	var l AddressList
	total, err := a.check(enums.UDTAddress)
	if err != nil {
		return l, err
	}
	l.OneKey = bits.Bit(a.Data[:], 7) == 1
	l.Len = (total - 8) / 24
	for i := range l.Len {
		l.Addresses[i] = address(a.Data[1+3*i:])
	}
	return l, nil
}

func digitCode(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c - '0'), true
	case c == '*':
		return 0b1010, true
	case c == '#':
		return 0b1011, true
	}
	return 0, false
}

// SetDigits sets a to the 1 to 92 dialled digits of digits in the BCD
// format. A digit is '0' to '9', '*' or '#'. It returns ErrUDTLength for a
// count out of range and ErrUDTCharacter for another character. (ETSI TS
// 102 361-4 clause B.3.3 and clause 7.2.9)
func (a *Appended) SetDigits(digits []byte) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	n := len(digits)
	if n < 1 || n > 92 {
		return ErrUDTLength
	}
	a.reset(enums.UDTBCD, blocksFor(4*n))
	for i, c := range digits {
		v, ok := digitCode(c)
		if !ok {
			return ErrUDTCharacter
		}
		bits.SetField(a.Data[:], 4*i, 4, v)
	}
	a.PadNibble = padNibbles(appendedBits(a.Blocks) - 4*n)
	a.pad(4 * n)
	return nil
}

// Digits appends the dialled digits of a in the BCD format to dst as the
// characters '0' to '9', '*' and '#', and returns dst. The digits end
// before the pad nibbles or at the first DigitNULL. It returns
// ErrUDTFormat for another format, ErrUDTLength for a PadNibble over the
// size of the blocks, and ErrUDTCharacter for a reserved digit code. It
// allocates only to grow dst. (ETSI TS 102 361-4 clause B.3.3 and clause
// 7.2.9)
func (a *Appended) Digits(dst []byte) ([]byte, error) {
	if a == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	total, err := a.check(enums.UDTBCD)
	if err != nil {
		return dst, err
	}
	n := total/4 - int(a.PadNibble)
	if n < 0 {
		return dst, ErrUDTLength
	}
	for i := range n {
		v := bits.Field(a.Data[:], 4*i, 4)
		switch {
		case v <= 9:
			dst = append(dst, '0'+byte(v))
		case v == 0b1010:
			dst = append(dst, '*')
		case v == 0b1011:
			dst = append(dst, '#')
		case v == 0b1111:
			return dst, nil
		default:
			return dst, ErrUDTCharacter
		}
	}
	return dst, nil
}

func symbolBits(f enums.UDTFormat) int {
	switch f {
	case enums.UDTISO7:
		return 7
	case enums.UDTISO8, enums.UDTLIP:
		return 8
	case enums.UDTUTF16BE, enums.UDTMixed:
		return 16
	case enums.UDTBinary, enums.UDTAddress, enums.UDTBCD, enums.UDTNMEA, enums.UDTIP, enums.UDTManufacturer1,
		enums.UDTManufacturer2:
	}
	return 0
}

func symbolCount(text []byte, w int) int {
	n := 0
	for i := 0; i < len(text); {
		r, k := utf8.DecodeRune(text[i:])
		i += k
		n++
		if w == 16 && r >= 0x10000 {
			n++
		}
	}
	return n
}

// SetText sets a to the UTF-8 text in the format f: enums.UDTISO7 (up to
// 52 characters of ISO/IEC 646), enums.UDTISO8 (up to 46 characters of
// ISO/IEC 8859-1) or enums.UDTUTF16BE (up to 23 UTF-16 code units). It
// returns ErrUDTFormat for another format, ErrUDTLength for an empty or a
// too long text, and ErrUDTCharacter for a character that the format
// cannot hold. Invalid UTF-8 gives U+FFFD. (ETSI TS 102 361-4 clauses
// B.3.4, B.3.5 and B.3.8)
func (a *Appended) SetText(f enums.UDTFormat, text []byte) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	if f != enums.UDTISO7 && f != enums.UDTISO8 && f != enums.UDTUTF16BE {
		return ErrUDTFormat
	}
	return a.setSymbols(f, 0, text)
}

// SetMixed sets a to the address addr and the UTF-8 text in the mixed
// format: up to 21 UTF-16 code units. An empty text gives one block of NUL
// characters, as the DGNA_Alias delete. oneKey marks addr as the
// One-key_talkgroup of DGNA. It returns ErrUDTLength for a too long text or
// an address over 24 bits. (ETSI TS 102 361-4 clause B.3.9)
func (a *Appended) SetMixed(oneKey bool, addr uint32, text []byte) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	if addr > 0xFFFFFF {
		return ErrUDTLength
	}
	if len(text) == 0 {
		a.reset(enums.UDTMixed, 1)
	} else if err := a.setSymbols(enums.UDTMixed, 32, text); err != nil {
		return err
	}
	if oneKey {
		bits.SetBit(a.Data[:], 7, 1)
	}
	bits.SetField(a.Data[:], 8, 24, uint64(addr))
	return nil
}

func (a *Appended) setSymbols(f enums.UDTFormat, start int, text []byte) error {
	w := symbolBits(f)
	n := symbolCount(text, w)
	b := blocksFor(start + w*n)
	if n == 0 || b == 0 {
		return ErrUDTLength
	}
	a.reset(f, b)
	o := start
	for i := 0; i < len(text); {
		r, k := utf8.DecodeRune(text[i:])
		i += k
		switch {
		case r < 0, w == 7 && r >= 0x80, w == 8 && r > 0xFF:
			return ErrUDTCharacter
		case w == 16 && r >= 0x10000:
			r -= 0x10000
			bits.SetField(a.Data[:], o, 16, uint64(0xD800+r>>10))
			bits.SetField(a.Data[:], o+16, 16, uint64(0xDC00+r&0x3FF))
			o += 32
			continue
		}
		bits.SetField(a.Data[:], o, w, uint64(r))
		o += w
	}
	a.PadNibble = padNibbles(appendedBits(b) - o)
	a.pad(o)
	return nil
}

// Text appends the text of a to dst as UTF-8 and returns dst, for the
// formats enums.UDTISO7, enums.UDTISO8 (read as ISO/IEC 8859-1),
// enums.UDTUTF16BE and enums.UDTMixed (the text after the address). The
// text ends before the pad nibbles or at the first NUL character, and
// drops its trailing DEL characters, the 1 bits of a block that a sender
// fills without pad nibbles. It returns ErrUDTFormat for another format and
// ErrUDTLength for a PadNibble over the size of the blocks. It allocates
// only to grow dst. (ETSI TS 102 361-4 clauses B.3.4, B.3.5, B.3.8 and
// B.3.9)
func (a *Appended) Text(dst []byte) ([]byte, error) {
	if a == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	w := symbolBits(a.Format)
	if w == 0 || a.Format == enums.UDTLIP {
		return dst, ErrUDTFormat
	}
	total, err := a.size()
	if err != nil {
		return dst, err
	}
	start := 0
	if a.Format == enums.UDTMixed {
		start = 32
	}
	room := total - start - 4*int(a.PadNibble)
	if room < 0 {
		return dst, ErrUDTLength
	}
	n := room / w
	from := len(dst)
	if w == 16 {
		return trimDEL(wire.AppendUTF8(dst, a.Data[start/8:start/8+2*n], true), from), nil
	}
	for i := range n {
		c := bits.Field(a.Data[:], start+w*i, w) & 0xFF
		if c == 0 {
			break
		}
		dst = wire.AppendRune(dst, rune(c))
	}
	return trimDEL(dst, from), nil
}

func trimDEL(dst []byte, from int) []byte {
	for len(dst) > from && dst[len(dst)-1] == 0x7F {
		dst = dst[:len(dst)-1]
	}
	return dst
}

// Mixed returns the address of a in the mixed format, and whether it is
// the One-key_talkgroup of DGNA. Text returns the text. It returns
// ErrUDTFormat for another format. (ETSI TS 102 361-4 clause B.3.9)
func (a *Appended) Mixed() (addr uint32, oneKey bool, err error) {
	if a == nil {
		return 0, false, dmrgo.ErrNilReceiver
	}

	if _, err = a.check(enums.UDTMixed); err != nil {
		return 0, false, err
	}
	return address(a.Data[1:]), bits.Bit(a.Data[:], 7) == 1, nil
}

// SetNMEAShort sets a to the Short NMEA format of n in one block. It
// returns the errors of NMEAShort.Encode. (ETSI TS 102 361-4 clause
// B.3.6.1)
func (a *Appended) SetNMEAShort(n *NMEAShort) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	p, err := n.Encode()
	if err != nil {
		return err
	}
	a.reset(enums.UDTNMEA, 1)
	copy(a.Data[:], p[:])
	return nil
}

// NMEAShort returns the Short NMEA format of a. It returns ErrUDTFormat for
// another format, or an NMEA format with other than 1 block. (ETSI TS 102
// 361-4 clause B.3.6.1)
func (a *Appended) NMEAShort() (NMEAShort, error) {
	if a == nil {
		return NMEAShort{}, dmrgo.ErrNilReceiver
	}

	if a.Format != enums.UDTNMEA || a.Blocks != 1 {
		return NMEAShort{}, ErrUDTFormat
	}
	return DecodeNMEAShort([10]byte(a.Data[:10])), nil
}

// SetNMEALong sets a to the Long NMEA Format Specified of n in two blocks.
// It returns the errors of NMEALong.Encode. (ETSI TS 102 361-4 clause
// B.3.6.2)
func (a *Appended) SetNMEALong(n *NMEALong) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	p, err := n.Encode()
	if err != nil {
		return err
	}
	a.reset(enums.UDTNMEA, 2)
	copy(a.Data[:], p[:])
	return nil
}

// NMEALong returns the Long NMEA Format Specified of a. It returns
// ErrUDTFormat for another format, or an NMEA format with other than 2
// blocks. The Long NMEA Format Unspecified, with 3 blocks, is for the
// manufacturer that its MFID names, in Data octet 11. (ETSI TS 102 361-4
// clauses B.3.6.2 and B.3.6.3)
func (a *Appended) NMEALong() (NMEALong, error) {
	if a == nil {
		return NMEALong{}, dmrgo.ErrNilReceiver
	}

	if a.Format != enums.UDTNMEA || a.Blocks != 2 {
		return NMEALong{}, ErrUDTFormat
	}
	return DecodeNMEALong([22]byte(a.Data[:22])), nil
}

// SetIP sets a to the IP address ip in the IP format: the first 4 octets
// of ip in one block for IPv4, or all 16 in two blocks for IPv6. (ETSI TS
// 102 361-4 clause B.3.7)
func (a *Appended) SetIP(ip [16]byte, v6 bool) {
	if a == nil {
		return
	}

	if v6 {
		a.reset(enums.UDTIP, 2)
		copy(a.Data[:], ip[:])
		return
	}
	a.reset(enums.UDTIP, 1)
	copy(a.Data[:], ip[:4])
}

// IP returns the IP address of a in the IP format, and whether it is IPv6.
// An IPv4 address is in the first 4 octets of ip. It returns ErrUDTFormat
// for another format and ErrUDTLength for other than 1 or 2 blocks. (ETSI
// TS 102 361-4 clause B.3.7)
func (a *Appended) IP() (ip [16]byte, v6 bool, err error) {
	if a == nil {
		return [16]byte{}, false, dmrgo.ErrNilReceiver
	}

	if a.Format != enums.UDTIP {
		return ip, false, ErrUDTFormat
	}
	switch a.Blocks {
	case 1:
		copy(ip[:], a.Data[:4])
		return ip, false, nil
	case 2:
		copy(ip[:], a.Data[:16])
		return ip, true, nil
	}
	return ip, false, ErrUDTLength
}

// SetLIP sets a to the 1 to 46 octets of a LIP PDU in the LIP format, such
// as an encoded LIPShortReport. It returns ErrUDTLength for a count out of
// range. (ETSI TS 102 361-4 clause B.3.10)
func (a *Appended) SetLIP(pdu []byte) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}

	n := len(pdu)
	if n < 1 || n > MaxAppendedOctets {
		return ErrUDTLength
	}
	a.reset(enums.UDTLIP, blocksFor(8*n))
	copy(a.Data[:], pdu)
	a.PadNibble = padNibbles(appendedBits(a.Blocks) - 8*n)
	a.pad(8 * n)
	return nil
}

// LIP appends the octets of the LIP PDU of a to dst and returns dst. The
// PDU ends before the pad nibbles. DecodeLIPShortReport reads a short
// location report from its first 10 octets. It returns ErrUDTFormat for
// another format and ErrUDTLength for a PadNibble over the size of the
// blocks. It allocates only to grow dst. (ETSI TS 102 361-4 clause B.3.10)
func (a *Appended) LIP(dst []byte) ([]byte, error) {
	if a == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	total, err := a.check(enums.UDTLIP)
	if err != nil {
		return dst, err
	}
	if 4*int(a.PadNibble) > total {
		return dst, ErrUDTLength
	}
	return append(dst, a.Data[:(total-4*int(a.PadNibble))/8]...), nil
}

// Appended returns the appended data of u, with the UDT Format and the pad
// nibbles of its header. (ETSI TS 102 361-4 clause 6.5.1)
func (u *UDT) Appended() Appended {
	if u == nil {
		return Appended{}
	}

	a := Appended{Format: u.Header.UDTFormat, PadNibble: u.Header.PadNibble, Blocks: int(u.Header.AppendedBlocks&3) + 1}
	copy(a.Data[:], u.Data[:12*a.Blocks-2])
	return a
}
