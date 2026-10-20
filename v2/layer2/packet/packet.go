package packet

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// maxBlocks is the largest number of blocks that follow a header: the
// Blocks To Follow element has 7 bits.
const maxBlocks = 127

// maxBlocksPerPacket is maxBlocks under the name that NewBuilder and
// NewAssembler, whose parameter is maxBlocks, use.
const maxBlocksPerPacket = maxBlocks

// maxBlockOctets is the most octets that a block holds: 24, for rate 1.
const maxBlockOctets = 24

// crcOctets is the length of the message CRC at the end of the last block.
const crcOctets = 4

// Packet is a header and the data blocks that follow it. Each block is a
// *pdu.Rate12Data, *pdu.Rate34Data or *pdu.Rate1Data with DataType set, ready
// for the matching pdu Encode function.
type Packet struct {
	Header pdu.DataHeader // the data header that starts the packet
	Blocks []layer2.Data  // the data blocks that follow the header
}

// blockOctets returns the user data octets of one block: 12, 18 or 24 for
// rate 1/2, 3/4 and 1, and 2 less for a confirmed block, which starts with
// the DBSN and CRC-9 (ETSI TS 102 361-1 clause 8.2.2.1, clause 8.2.2.2).
func blockOctets(rate enums.DataType, confirmed bool) (int, error) {
	var n int
	switch rate {
	case enums.DataTypeRate12:
		n = 12
	case enums.DataTypeRate34:
		n = 18
	case enums.DataTypeRate1:
		n = 24
	case enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC,
		enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader, enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock,
		enums.DataTypeReserved:
		return 0, ErrRate
	default:
		return 0, ErrRate
	}
	if confirmed {
		n -= 2
	}
	return n, nil
}

// appendMessageCRC appends the 32-bit message CRC over data, the user data
// and pad octets, least significant octet first (ETSI TS 102 361-1 clause
// B.3.9, Figure B.8C).
func appendMessageCRC(data []byte) []byte {
	return binary.LittleEndian.AppendUint32(data, crc.CRC32(data))
}

// checkMessageCRC reports whether the last 4 octets of data are the message
// CRC of the octets before them.
func checkMessageCRC(data []byte) bool {
	if len(data) < crcOctets {
		return false
	}
	n := len(data) - crcOctets
	return crc.CRC32(data[:n]) == binary.LittleEndian.Uint32(data[n:])
}

// layout returns the number of blocks and pad octets for a payload of n
// octets in blocks of per octets: the payload, the pad and the 4-octet
// message CRC exactly fill the blocks (ETSI TS 102 361-1 clause 8.2.2.1).
func layout(n, per int) (blocks, pad int) {
	blocks = (n + crcOctets + per - 1) / per
	return blocks, blocks*per - crcOctets - n
}

// NewPacket splits payload into the data blocks of one packet at the given
// rate and returns them with a copy of header. It sets the block count and
// pad fields of the header, gives confirmed blocks DBSN 0, 1, 2 and so on,
// and keeps the other header fields. It returns an error that wraps
// ErrRate, ErrFormat or ErrTooLong. It allocates the storage of the Packet;
// Builder.Segment builds into the storage of a Builder.
func NewPacket(header pdu.DataHeader, payload []byte, rate enums.DataType) (Packet, error) {
	confirmed, ok := isConfirmed(&header)
	if !ok {
		return Packet{}, ErrFormat
	}
	per, err := blockOctets(rate, confirmed)
	if err != nil {
		return Packet{}, err
	}
	blocks, _ := layout(len(payload), per)
	return NewBuilder(min(blocks, maxBlocks)).Segment(header, payload, rate)
}

// Builder builds packets into storage that NewBuilder allocates once, so
// that it does not allocate. A Packet that a Builder returns points into this
// storage, through its sub-header and its Blocks, and is valid until the next
// call of the same method on the Builder, except that Retry leaves a Packet
// from Segment as it is. The zero value has no storage for blocks: Segment
// and Retry return ErrTooLong for a packet with blocks, and NewResponse
// works. A Packet points into the Builder, so a Builder must not be copied
// after first use; go vet reports a copy. A Builder is not safe for
// concurrent use.
type Builder struct {
	unconfirmed pdu.UnconfirmedDataHeader
	confirmed   pdu.ConfirmedDataHeader
	defined     pdu.DefinedDataHeader
	raw         pdu.RawDataHeader
	buf         []byte
	rate12      []pdu.Rate12Data
	rate34      []pdu.Rate34Data
	rate1       []pdu.Rate1Data
	blocks      []layer2.Data
	retry       retryStore
	resp        responseStore
	noCopy      noCopy
}

// NewBuilder returns a Builder for packets of up to maxBlocks data blocks of
// any rate. It allocates 94 bytes for each block on a 32-bit target and 110
// on a 64-bit one, on top of the Builder itself, 276 and 368 bytes, so 16
// blocks take about 1.8 KB on a microcontroller and 127, the most that a
// header counts, about 12 KB. A maxBlocks over 127 means 127, and one under
// 0 means 0.
func NewBuilder(maxBlocks int) *Builder {
	n := max(0, min(maxBlocks, maxBlocksPerPacket))
	return &Builder{
		buf:    make([]byte, n*maxBlockOctets),
		rate12: make([]pdu.Rate12Data, n),
		rate34: make([]pdu.Rate34Data, n),
		rate1:  make([]pdu.Rate1Data, n),
		blocks: make([]layer2.Data, n),
		retry:  retryStore{blocks: make([]layer2.Data, n)},
	}
}

// retryStore holds the header and block list of the Packet that
// Builder.Retry returns, apart from the storage of Segment, so that a
// retry does not change the packet it resends.
type retryStore struct {
	confirmed pdu.ConfirmedDataHeader
	defined   pdu.DefinedDataHeader
	raw       pdu.RawDataHeader
	blocks    []layer2.Data
}

// Segment does what NewPacket does, into the storage of b.
func (b *Builder) Segment(header pdu.DataHeader, payload []byte, rate enums.DataType) (Packet, error) {
	confirmed, err := b.setLayout(&header, len(payload), rate)
	if err != nil {
		return Packet{}, err
	}
	per, _ := blockOctets(rate, confirmed)
	n := blocksOf(&header)
	if n > len(b.blocks) {
		return Packet{}, ErrTooLong
	}
	size := n*per - crcOctets
	clear(b.buf[copy(b.buf[:size], payload):size])
	binary.LittleEndian.PutUint32(b.buf[size:], crc.CRC32(b.buf[:size]))
	for i := range n {
		b.blocks[i] = b.block(i, b.buf[i*per:(i+1)*per], confirmed)
	}
	return Packet{Header: header, Blocks: b.blocks[:n]}, nil
}

// Retry builds the later try of the confirmed packet p that a SACK with
// response data rd asks for: a copy of the header of p with the Full Message
// Flag clear and the block count set to the number of blocks it resends, then
// block i of p for each DBSN i that rd flags, in DBSN order. A nil rd resends
// every block, as after a NACK or a missing response. The header keeps N(S)
// and the other fields of the first try. p is a C_HEAD, or a DD_HEAD or
// R_HEAD with SARQ set, with the blocks Segment makes. Retry returns an error
// that wraps ErrFormat for another header, and ErrNoRetryBlocks when rd flags
// no block of p. The Packet points to the blocks of p and to storage of b,
// apart from that of Segment and NewResponse, and is valid until the next
// Retry. (ETSI TS 102 361-1 clause 9.3.20, ETSI TS 102 361-3 clause 5.4)
func (b *Builder) Retry(p *Packet, rd *ResponseData) (Packet, error) {
	h := p.Header
	n := 0
	for i := range p.Blocks {
		if rd == nil || i < len(rd.Retry) && rd.Retry[i] {
			n++
		}
	}
	count := uint8(n & 0x7F)
	switch confirmed, _ := isConfirmed(&h); {
	case !confirmed:
		return Packet{}, errRetryFormat
	case n == 0:
		return Packet{}, ErrNoRetryBlocks
	case n > len(b.retry.blocks):
		return Packet{}, ErrTooLong
	case h.Format == pdu.FormatConfirmed:
		b.retry.confirmed = *h.ConfirmedDataHeader
		b.retry.confirmed.FullMessageFlag, b.retry.confirmed.BlocksToFollow = false, count
		h.ConfirmedDataHeader = &b.retry.confirmed
	case h.Format == pdu.FormatShortDataDefined:
		b.retry.defined = *h.DefinedDataHeader
		b.retry.defined.FullMessageFlag, b.retry.defined.AppendedBlocks = false, count
		h.DefinedDataHeader, h.AppendedBlocks = &b.retry.defined, count
	default:
		b.retry.raw = *h.RawDataHeader
		b.retry.raw.FullMessageFlag, b.retry.raw.AppendedBlocks = false, count
		h.RawDataHeader, h.AppendedBlocks = &b.retry.raw, count
	}
	j := 0
	for i, blk := range p.Blocks {
		if rd == nil || i < len(rd.Retry) && rd.Retry[i] {
			b.retry.blocks[j] = blk
			j++
		}
	}
	return Packet{Header: h, Blocks: b.retry.blocks[:n]}, nil
}

// block fills block i of the storage for its chunk length and returns it.
// A confirmed block gets DBSN i.
func (b *Builder) block(i int, chunk []byte, confirmed bool) layer2.Data {
	dbsn := uint8(i & 0x7F)
	switch len(chunk) {
	case 16, 18:
		blk := &b.rate34[i]
		if confirmed {
			_ = blk.SetConfirmed(dbsn, [16]byte(chunk))
		} else {
			blk.Data = [18]byte(chunk)
		}
		return blk
	case 22, 24:
		blk := &b.rate1[i]
		if confirmed {
			_ = blk.SetConfirmed(dbsn, [22]byte(chunk))
		} else {
			blk.Data = [24]byte(chunk)
		}
		return blk
	default:
		blk := &b.rate12[i]
		if confirmed {
			_ = blk.SetConfirmed(dbsn, [10]byte(chunk))
		} else {
			blk.Data = [12]byte(chunk)
		}
		return blk
	}
}

// setLayout sets the block count and pad fields of a copy of the sub-header
// of h in the storage of b, and points h at the copy.
func (b *Builder) setLayout(h *pdu.DataHeader, n int, rate enums.DataType) (bool, error) {
	confirmed, ok := isConfirmed(h)
	if !ok {
		return false, ErrFormat
	}
	per, err := blockOctets(rate, confirmed)
	if err != nil {
		return false, err
	}
	blocks, pad := layout(n, per)
	switch h.Format {
	case pdu.FormatUnconfirmed:
		b.unconfirmed = *h.UnconfirmedDataHeader
		b.unconfirmed.BlocksToFollow, b.unconfirmed.PadOctetCount = uint8(blocks&0xFF), uint8(pad&0xFF)
		h.UnconfirmedDataHeader = &b.unconfirmed
	case pdu.FormatConfirmed:
		b.confirmed = *h.ConfirmedDataHeader
		b.confirmed.BlocksToFollow, b.confirmed.PadOctetCount = uint8(blocks&0xFF), uint8(pad&0xFF)
		h.ConfirmedDataHeader = &b.confirmed
	case pdu.FormatShortDataDefined:
		b.defined = *h.DefinedDataHeader
		b.defined.AppendedBlocks = uint8(blocks & 0xFF)
		h.DefinedDataHeader = &b.defined
		h.AppendedBlocks = b.defined.AppendedBlocks
	case pdu.FormatShortDataRawOrStatusPrecoded:
		b.raw = *h.RawDataHeader
		b.raw.AppendedBlocks = uint8(blocks & 0xFF)
		h.RawDataHeader = &b.raw
		h.AppendedBlocks = b.raw.AppendedBlocks
	case pdu.FormatUnifiedDataTransport, pdu.FormatResponsePacket, pdu.FormatProprietary:
	}
	limit := maxBlocks
	if h.Format == pdu.FormatShortDataDefined || h.Format == pdu.FormatShortDataRawOrStatusPrecoded {
		limit = 63
	}
	if blocks > limit {
		return false, ErrTooLong
	}
	return confirmed, nil
}

// isConfirmed reports whether h is a confirmed or SARQ header. ok is false
// for a header that does not carry segmented data.
func isConfirmed(h *pdu.DataHeader) (confirmed, ok bool) {
	switch {
	case h.Format == pdu.FormatUnconfirmed && h.UnconfirmedDataHeader != nil:
		return false, true
	case h.Format == pdu.FormatConfirmed && h.ConfirmedDataHeader != nil:
		return true, true
	case h.Format == pdu.FormatShortDataDefined && h.DefinedDataHeader != nil:
		return h.DefinedDataHeader.SARQ, true
	case h.Format == pdu.FormatShortDataRawOrStatusPrecoded && h.RawDataHeader != nil:
		return h.RawDataHeader.SARQ, true
	}
	return false, false
}

func blocksOf(h *pdu.DataHeader) int {
	switch {
	case h.UnconfirmedDataHeader != nil && h.Format == pdu.FormatUnconfirmed:
		return int(h.UnconfirmedDataHeader.BlocksToFollow)
	case h.ConfirmedDataHeader != nil && h.Format == pdu.FormatConfirmed:
		return int(h.ConfirmedDataHeader.BlocksToFollow)
	case h.ResponsePacketHeader != nil && h.Format == pdu.FormatResponsePacket:
		return int(h.ResponsePacketHeader.BlocksToFollow)
	case h.DefinedDataHeader != nil && h.Format == pdu.FormatShortDataDefined:
		return int(h.DefinedDataHeader.AppendedBlocks)
	case h.RawDataHeader != nil && h.Format == pdu.FormatShortDataRawOrStatusPrecoded:
		return int(h.RawDataHeader.AppendedBlocks)
	}
	return 0
}

// fullMessage reports whether h has its Full Message Flag set, or has no
// such flag: true for the first try of a packet, false for a later try
// (ETSI TS 102 361-1 clause 9.3.20).
func fullMessage(h *pdu.DataHeader) bool {
	switch {
	case h.UnconfirmedDataHeader != nil && h.Format == pdu.FormatUnconfirmed:
		return h.UnconfirmedDataHeader.FullMessageFlag
	case h.ConfirmedDataHeader != nil && h.Format == pdu.FormatConfirmed:
		return h.ConfirmedDataHeader.FullMessageFlag
	case h.DefinedDataHeader != nil && h.Format == pdu.FormatShortDataDefined:
		return h.DefinedDataHeader.FullMessageFlag
	case h.RawDataHeader != nil && h.Format == pdu.FormatShortDataRawOrStatusPrecoded:
		return h.RawDataHeader.FullMessageFlag
	}
	return true
}

func padOf(h *pdu.DataHeader) int {
	switch {
	case h.UnconfirmedDataHeader != nil && h.Format == pdu.FormatUnconfirmed:
		return int(h.UnconfirmedDataHeader.PadOctetCount)
	case h.ConfirmedDataHeader != nil && h.Format == pdu.FormatConfirmed:
		return int(h.ConfirmedDataHeader.PadOctetCount)
	}
	return 0
}

type block struct {
	dataType enums.DataType
	raw      []byte
}

func blockOf(d layer2.Data) (block, bool) {
	switch b := d.(type) {
	case *pdu.Rate12Data:
		return block{enums.DataTypeRate12, b.Data[:]}, true
	case *pdu.Rate34Data:
		return block{enums.DataTypeRate34, b.Data[:]}, true
	case *pdu.Rate1Data:
		return block{enums.DataTypeRate1, b.Data[:]}, true
	}
	return block{}, false
}

func (b block) confirmedOK() bool {
	switch len(b.raw) {
	case 18:
		r := pdu.Rate34Data{Data: [18]byte(b.raw)}
		return r.ConfirmedCRC9OK()
	case 24:
		r := pdu.Rate1Data{Data: [24]byte(b.raw)}
		return r.ConfirmedCRC9OK()
	default:
		r := pdu.Rate12Data{Data: [12]byte(b.raw)}
		return r.ConfirmedCRC9OK()
	}
}
