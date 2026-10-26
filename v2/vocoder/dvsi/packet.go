package dvsi

import "bytes"

// StartByte is the first byte of every packet (AMBE-3000R manual section
// 6.5.1).
const StartByte = 0x61

// HeaderLen is the length of the packet header: StartByte, the 2-byte
// LENGTH, most significant byte first, and the packet type. LENGTH counts
// the bytes after the header, the parity field included (AMBE-3000R manual
// section 6.5.2).
const HeaderLen = 4

// MaxLength is the largest LENGTH that Parse accepts. The longest packet of
// the manuals, a speech packet of MaxSamples samples with channel, CMODE,
// TONE and parity fields, has LENGTH 339.
const MaxLength = 384

// MaxPacketLen is the length of the longest packet that Parse accepts.
const MaxPacketLen = HeaderLen + MaxLength

// MaxChannel is the highest channel number: the AMBE-3003 has channels 0 to
// 2, the AMBE-3000R only channel 0 (AMBE-3003 manual Table 27).
const MaxChannel = 2

// FrameSamples is the number of 8 kHz samples in one 20 ms frame.
const FrameSamples = 160

// MinSamples and MaxSamples bound the samples of a SPEECHD field
// (AMBE-3000R manual Table 98).
const (
	MinSamples = 156
	MaxSamples = 164
)

// FrameBits is the number of channel bits in one DMR AMBE+2 frame with FEC.
const FrameBits = 72

// MinBits and MaxBits bound the bits of a CHAND field (AMBE-3000R manual
// Table 106).
const (
	MinBits = 40
	MaxBits = 192
)

// PacketType is the fourth byte of a packet (AMBE-3000R manual Table 29).
type PacketType uint8

// The packet types.
const (
	TypeControl PacketType = 0x00 // configuration requests and their responses
	TypeChannel PacketType = 0x01 // channel bits to decode, or encoded by the chip
	TypeSpeech  PacketType = 0x02 // speech samples to encode, or decoded by the chip
)

// Field is a field identifier, the first byte of a field (AMBE-3000R manual
// Table 31).
type Field uint8

// The field identifiers that the package builds or reads.
const (
	FieldRateP      Field = 0x0A // six rate control words for the current channel
	FieldInit       Field = 0x0B // initialize the encoder (bit 0) and decoder (bit 1)
	FieldProdID     Field = 0x30 // product identification query
	FieldVerString  Field = 0x31 // version string query
	FieldReset      Field = 0x33 // reset with the configuration pins
	FieldReady      Field = 0x39 // sent by the chip when it is ready after a reset
	FieldParityMode Field = 0x3F // turn the parity field on (1) or off (0)
	FieldChannel0   Field = 0x40 // the next fields are for channel 0
	FieldChannel1   Field = 0x41 // the next fields are for channel 1 (AMBE-3003)
	FieldChannel2   Field = 0x42 // the next fields are for channel 2 (AMBE-3003)
)

const (
	fieldSpeechData   Field = 0x00
	fieldChannelData  Field = 0x01
	fieldCMode        Field = 0x02
	fieldSamples      Field = 0x03
	fieldTone         Field = 0x08
	fieldSpeechFormat Field = 0x16
	fieldChannelData4 Field = 0x17
	fieldGetCfg       Field = 0x36
	fieldReadCfg      Field = 0x37
	fieldParity       Field = 0x2F
)

func begin(dst []byte, t PacketType) ([]byte, int) {
	return append(dst, StartByte, 0, 0, byte(t)), len(dst)
}

func finish(dst []byte, start int, parity bool) []byte {
	n := len(dst) - start - HeaderLen
	if parity {
		n += 2
	}
	dst[start+1], dst[start+2] = byte(n>>8&0xFF), byte(n&0xFF)
	if !parity {
		return dst
	}
	dst = append(dst, byte(fieldParity))
	var x byte
	for _, b := range dst[start+1:] {
		x ^= b
	}
	return append(dst, x)
}

func appendChannelField(dst []byte, channel int) ([]byte, error) {
	if channel < 0 || channel > MaxChannel {
		return dst, ErrChannel
	}
	if channel > 0 {
		dst = append(dst, byte(FieldChannel0)+byte(channel))
	}
	return dst, nil
}

// AppendReset appends a control packet with a PKT_RESET field, which resets
// the chip to its power-up state. The chip replies with a PKT_READY packet
// (AMBE-3000R manual Table 72). With parity the packet ends in a parity
// field. It allocates only to grow dst.
func AppendReset(dst []byte, parity bool) []byte {
	dst, start := begin(dst, TypeControl)
	return finish(append(dst, byte(FieldReset)), start, parity)
}

// AppendQuery appends a control packet with PKT_PRODID and PKT_VERSTRING
// fields. The chip replies with its product name, such as "AMBE3000R", and
// its version string (AMBE-3000R manual Tables 66 to 69). It allocates only
// to grow dst.
func AppendQuery(dst []byte, parity bool) []byte {
	dst, start := begin(dst, TypeControl)
	return finish(append(dst, byte(FieldProdID), byte(FieldVerString)), start, parity)
}

// AppendParityMode appends a control packet with a PKT_PARITYMODE field
// that turns the parity field of later packets on or off. The response to
// it already uses the new mode (AMBE-3000R manual Table 79). parity sets
// whether this packet itself ends in a parity field; a packet with a valid
// parity field is accepted in both modes. It allocates only to grow dst.
func AppendParityMode(dst []byte, on, parity bool) []byte {
	dst, start := begin(dst, TypeControl)
	mode := byte(0)
	if on {
		mode = 1
	}
	return finish(append(dst, byte(FieldParityMode), mode), start, parity)
}

// AppendRateDMR appends a control packet that sets the channel to the DMR
// AMBE+2 rate, 3600 bit/s with 2450 bit/s of speech and 1150 bit/s of FEC,
// with the rate control words 0x0431 0x0754 0x2400 0x0000 0x0000 0x6F48 in
// a PKT_RATEP field, and then initializes its encoder and decoder with a
// PKT_INIT field (AMBE-3000R manual Tables 43, 46 and 115, rate index 33).
// The packet starts with a channel field when channel is not 0. It returns
// ErrChannel for a channel outside 0 to MaxChannel. It allocates only to
// grow dst.
func AppendRateDMR(dst []byte, channel int, parity bool) ([]byte, error) {
	if channel < 0 || channel > MaxChannel {
		return dst, ErrChannel
	}
	dst, start := begin(dst, TypeControl)
	dst, _ = appendChannelField(dst, channel)
	dst = append(dst, byte(FieldRateP), 0x04, 0x31, 0x07, 0x54, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x6F, 0x48, byte(FieldInit), 0x03)
	return finish(dst, start, parity), nil
}

const (
	dcmodeLostFrame   = 0x0004
	dcmodeVoiceActive = 0x0002
	dcmodeDataInvalid = 0x0020
	dcmodeToneFrame   = 0x8000
)

func appendSpeechFormat(dst []byte, parity bool) []byte {
	dst, start := begin(dst, TypeControl)
	return finish(append(dst, byte(fieldSpeechFormat), 0x00, 0x01), start, parity)
}

func appendCMode(dst []byte, channel int, slot *[FrameBits / 8]byte, mode uint16, parity bool) []byte {
	dst, start := begin(dst, TypeChannel)
	dst, _ = appendChannelField(dst, channel)
	dst = append(dst, byte(fieldCMode), byte(mode>>8), byte(mode&0xFF))
	if slot != nil {
		dst = append(dst, byte(fieldChannelData), FrameBits)
		dst = append(dst, slot[:]...)
	}
	return finish(dst, start, parity)
}

// AppendChannel appends a channel packet that asks the chip to decode bits
// channel bits, packed MSB first in data, which must hold (bits+7)/8 bytes
// (AMBE-3000R manual Table 106). The chip replies with a speech packet. It
// returns ErrChannel for a channel outside 0 to MaxChannel and ErrBits for
// bits outside MinBits to MaxBits or data of another length, with dst
// unchanged. It allocates only to grow dst.
func AppendChannel(dst []byte, channel int, data []byte, bits int, parity bool) ([]byte, error) {
	if channel < 0 || channel > MaxChannel {
		return dst, ErrChannel
	}
	if bits < MinBits || bits > MaxBits || len(data) != (bits+7)/8 {
		return dst, ErrBits
	}
	dst, start := begin(dst, TypeChannel)
	dst, _ = appendChannelField(dst, channel)
	dst = append(dst, byte(fieldChannelData), byte(bits))
	dst = append(dst, data...)
	return finish(dst, start, parity), nil
}

// AppendSpeech appends a speech packet that asks the chip to encode pcm,
// 8 kHz 16-bit samples, each sent most significant byte first (AMBE-3000R
// manual Table 98). The chip replies with a channel packet. It returns
// ErrChannel for a channel outside 0 to MaxChannel and ErrSamples for other
// than MinSamples to MaxSamples samples, with dst unchanged. It allocates
// only to grow dst.
func AppendSpeech(dst []byte, channel int, pcm []int16, parity bool) ([]byte, error) {
	if channel < 0 || channel > MaxChannel {
		return dst, ErrChannel
	}
	if len(pcm) < MinSamples || len(pcm) > MaxSamples {
		return dst, ErrSamples
	}
	k := len(dst)
	for range speechLen(channel, len(pcm), parity) {
		dst = append(dst, 0)
	}
	putSpeech(dst[k:], channel, pcm, parity)
	return dst, nil
}

func speechLen(channel, samples int, parity bool) int {
	n := HeaderLen + 2 + 2*samples
	if channel > 0 {
		n++
	}
	if parity {
		n += 2
	}
	return n
}

func putSpeech(b []byte, channel int, pcm []int16, parity bool) int {
	n := speechLen(channel, len(pcm), parity)
	b = b[:n]
	l := n - HeaderLen
	b[0], b[1], b[2], b[3] = StartByte, byte(l>>8&0xFF), byte(l&0xFF), byte(TypeSpeech)
	i := HeaderLen
	if channel > 0 {
		b[i] = byte(FieldChannel0) + byte(channel&3)
		i++
	}
	b[i], b[i+1] = byte(fieldSpeechData), byte(len(pcm)&0xFF)
	i += 2
	for _, s := range pcm {
		v := int(s) & 0xFFFF
		b[i], b[i+1] = byte(v>>8), byte(v&0xFF)
		i += 2
	}
	if parity {
		b[i] = byte(fieldParity)
		var x byte
		for _, c := range b[1 : n-1] {
			x ^= c
		}
		b[n-1] = x
	}
	return n
}

// Packet is one parsed packet.
type Packet struct {
	// Type is the packet type.
	Type PacketType
	// Fields holds the bytes after the header, without the parity field. It
	// points into the bytes given to Parse.
	Fields []byte
}

// Parse parses the packet at the start of b and returns it with its length
// in bytes. With parity, the packet must end in a parity field with a
// correct parity byte, which Parse removes from Fields; without, Parse keeps
// every byte in Fields (AMBE-3000R manual section 6.5.5). It returns
// ErrIncomplete when b holds only the start of a packet, and ErrStart,
// ErrType, ErrLength or ErrParity when b does not start with a packet; a
// reader then drops b[0] and looks for the next StartByte. It does not
// allocate.
func Parse(b []byte, parity bool) (Packet, int, error) {
	p, n, st := parse(b, parity)
	switch st {
	case parseIncomplete:
		return p, n, ErrIncomplete
	case parseStart:
		return p, n, ErrStart
	case parseType:
		return p, n, ErrType
	case parseLength:
		return p, n, ErrLength
	case parseParity:
		return p, n, ErrParity
	default:
		return p, n, nil
	}
}

type parseStatus uint8

const (
	parseOK parseStatus = iota
	parseIncomplete
	parseStart
	parseType
	parseLength
	parseParity
)

func parse(b []byte, parity bool) (Packet, int, parseStatus) {
	if len(b) == 0 {
		return Packet{}, 0, parseIncomplete
	}
	if b[0] != StartByte {
		return Packet{}, 0, parseStart
	}
	if len(b) < HeaderLen {
		return Packet{}, 0, parseIncomplete
	}
	t := PacketType(b[3])
	if t > TypeSpeech {
		return Packet{}, 0, parseType
	}
	n := int(b[1])<<8 | int(b[2])
	if n > MaxLength || parity && n < 2 {
		return Packet{}, 0, parseLength
	}
	if len(b) < HeaderLen+n {
		return Packet{}, 0, parseIncomplete
	}
	fields := b[HeaderLen : HeaderLen+n]
	if parity {
		var x byte
		for _, c := range b[1 : HeaderLen+n-1] {
			x ^= c
		}
		if fields[n-2] != byte(fieldParity) || fields[n-1] != x {
			return Packet{}, 0, parseParity
		}
		fields = fields[:n-2]
	}
	return Packet{Type: t, Fields: fields}, HeaderLen + n, parseOK
}

func sample(hi, lo byte) int16 {
	return int16(hi)<<8 | int16(lo)
}

func responseLen(f Field, data []byte) int {
	switch f {
	case FieldReady:
		return 0
	case FieldProdID, FieldVerString:
		if i := bytes.IndexByte(data, 0); i >= 0 {
			return i + 1
		}
		return -1
	case fieldGetCfg, fieldReadCfg:
		return 3
	default:
		return 1
	}
}

// Response returns the data of response field f of a control packet: the
// status byte, 0 for success, for most fields, the text without its
// terminating zero byte for FieldProdID and FieldVerString, and no bytes for
// FieldReady. It returns false when the packet is not a control packet,
// has no field f or cannot be read up to it. The data points into
// p.Fields. It does not allocate.
func (p Packet) Response(f Field) ([]byte, bool) {
	if p.Type != TypeControl {
		return nil, false
	}
	b := p.Fields
	for len(b) > 0 {
		id := Field(b[0])
		n := responseLen(id, b[1:])
		if n < 0 || n > len(b)-1 {
			return nil, false
		}
		if id == f {
			if f == FieldProdID || f == FieldVerString {
				return b[1:n], true
			}
			return b[1 : 1+n], true
		}
		b = b[1+n:]
	}
	return nil, false
}

func (p Packet) failed() bool {
	b := p.Fields
	for len(b) > 0 {
		id := Field(b[0])
		n := responseLen(id, b[1:])
		if n < 0 || n > len(b)-1 {
			return true
		}
		if n == 1 && b[1] != 0 {
			return true
		}
		b = b[1+n:]
	}
	return false
}

type cmode struct {
	flags uint16
	ok    bool
}

func (p Packet) walk(want PacketType, data Field) (channel int, body []byte, mode cmode, err error) {
	if p.Type != want {
		return 0, nil, mode, ErrField
	}
	b := p.Fields
	found := false
	for len(b) > 0 {
		id := Field(b[0])
		switch {
		case id >= FieldChannel0 && id <= FieldChannel2:
			if found {
				return 0, nil, mode, ErrField
			}
			channel = int(id - FieldChannel0)
			b = b[1:]
			continue
		case id == data && !found:
			if len(b) < 2 {
				return 0, nil, mode, ErrField
			}
			n := int(b[1])
			if data == fieldSpeechData {
				n *= 2
			} else {
				n = (n + 7) / 8
			}
			if len(b) < 2+n {
				return 0, nil, mode, ErrField
			}
			body, found = b[1:2+n], true
			b = b[2+n:]
			continue
		case id == fieldCMode && len(b) >= 3:
			mode = cmode{flags: uint16(b[1])<<8 | uint16(b[2]), ok: true}
			b = b[3:]
			continue
		}
		var n int
		switch id {
		case fieldCMode, fieldTone:
			n = 2
		case fieldSamples:
			n = 1
		case fieldChannelData4:
			if len(b) < 2 {
				return 0, nil, mode, ErrField
			}
			n = 1 + (int(b[1])+1)/2
		default:
			return 0, nil, mode, ErrField
		}
		if len(b) < 1+n {
			return 0, nil, mode, ErrField
		}
		b = b[1+n:]
	}
	if !found {
		return 0, nil, mode, ErrField
	}
	return channel, body, mode, nil
}

// AppendSamples appends the samples of the SPEECHD field of a speech packet
// to dst and returns the extended slice and the channel of the field. A
// packet without a channel field is for channel 0. It returns ErrField when
// p is not a speech packet with one SPEECHD field and known fields only,
// and ErrSamples for other than MinSamples to MaxSamples samples, with dst
// unchanged. It allocates only to grow dst.
func (p Packet) AppendSamples(dst []int16) ([]int16, int, error) {
	channel, body, _, err := p.walk(TypeSpeech, fieldSpeechData)
	if err != nil {
		return dst, 0, err
	}
	n := int(body[0])
	if n < MinSamples || n > MaxSamples {
		return dst, 0, ErrSamples
	}
	for i := range n {
		dst = append(dst, sample(body[1+2*i], body[2+2*i]))
	}
	return dst, channel, nil
}

// AppendChannelData appends the bits of the CHAND field of a channel packet,
// packed MSB first, to dst and returns the extended slice, the number of
// bits and the channel of the field. A packet without a channel field is
// for channel 0. It returns ErrField when p is not a channel packet with one
// CHAND field and known fields only, and ErrBits for bits outside MinBits to
// MaxBits, with dst unchanged. It allocates only to grow dst.
func (p Packet) AppendChannelData(dst []byte) ([]byte, int, int, error) {
	channel, body, _, err := p.walk(TypeChannel, fieldChannelData)
	if err != nil {
		return dst, 0, 0, err
	}
	bits := int(body[0])
	if bits < MinBits || bits > MaxBits {
		return dst, 0, 0, ErrBits
	}
	return append(dst, body[1:]...), bits, channel, nil
}
