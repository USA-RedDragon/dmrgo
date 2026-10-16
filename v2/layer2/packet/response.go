package packet

import (
	"encoding/binary"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Response is the Class and Type of a response packet, as Class<<3 | Type
// (ETSI TS 102 361-1 Table 8.3).
type Response uint8

const (
	// ResponseACK acknowledges all blocks of all packets up to NI.
	ResponseACK Response = 0b00_001
	// ResponseNACKIllegalFormat rejects a packet with an illegal format.
	ResponseNACKIllegalFormat Response = 0b01_000
	// ResponseNACKPacketCRC rejects a packet whose message CRC failed.
	ResponseNACKPacketCRC Response = 0b01_001
	// ResponseNACKMemoryFull rejects a packet because the recipient memory is full.
	ResponseNACKMemoryFull Response = 0b01_010
	// ResponseNACKFSNOutOfSequence rejects a packet whose FSN is out of sequence.
	ResponseNACKFSNOutOfSequence Response = 0b01_011
	// ResponseNACKUndeliverable rejects an undeliverable packet.
	ResponseNACKUndeliverable Response = 0b01_100
	// ResponseNACKOutOfSequence rejects a packet whose N(S) is not VI or VI + 1.
	ResponseNACKOutOfSequence Response = 0b01_101
	// ResponseNACKInvalidUser rejects a user the system disallows.
	ResponseNACKInvalidUser Response = 0b01_110
	// ResponseSACK asks for the selective retry of the blocks flagged in the
	// response data blocks.
	ResponseSACK Response = 0b10_000
)

// NewResponseCode returns the Response of a Class and Type.
func NewResponseCode(class, typ uint8) Response {
	return Response((class&0b11)<<3 | typ&0b111)
}

// ResponseOf returns the Response of a C_RHEAD.
func ResponseOf(h *pdu.ResponsePacketHeader) Response {
	return NewResponseCode(h.ResponseClass, h.ResponseType)
}

// Class returns the 2-bit Class.
func (r Response) Class() uint8 { return uint8(r) >> 3 & 0b11 }

// Type returns the 3-bit Type.
func (r Response) Type() uint8 { return uint8(r) & 0b111 }

// String returns the Table 8.3 name of r.
func (r Response) String() string {
	switch r {
	case ResponseACK:
		return "ACK"
	case ResponseNACKIllegalFormat:
		return "NACK Illegal Format"
	case ResponseNACKPacketCRC:
		return "NACK Packet CRC"
	case ResponseNACKMemoryFull:
		return "NACK Memory Full"
	case ResponseNACKFSNOutOfSequence:
		return "NACK FSN Out Of Sequence"
	case ResponseNACKUndeliverable:
		return "NACK Undeliverable"
	case ResponseNACKOutOfSequence:
		return "NACK Out Of Sequence"
	case ResponseNACKInvalidUser:
		return "NACK Invalid User"
	case ResponseSACK:
		return "SACK"
	}
	return fmt.Sprintf("Unknown Response (class %d, type %d)", r.Class(), r.Type())
}

// ResponseData is the selective retry bitmap of the C_RDATA PDU (ETSI TS
// 102 361-1 clause 8.2.2.3, Table 9.14). The zero value retries nothing. On
// air flag i is bit i%8 of octet i/8, where bit 0 is the least significant,
// and is 1 for a received block (ETSI TS 102 361-1 Figure 8.17).
type ResponseData struct {
	Retry [127]bool // Retry[i] is set to ask for the block with DBSN i again
}

// RetryBlocks returns the DBSNs flagged for retry in ascending order. It
// allocates the slice; AppendRetryBlocks does not.
func (r *ResponseData) RetryBlocks() []int {
	n := 0
	for _, retry := range r.Retry {
		if retry {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return r.AppendRetryBlocks(make([]int, 0, n))
}

// AppendRetryBlocks appends the DBSNs flagged for retry to dst in ascending
// order and returns the extended slice. It does not allocate when dst has
// room.
func (r *ResponseData) AppendRetryBlocks(dst []int) []int {
	for i, retry := range r.Retry {
		if retry {
			dst = append(dst, i)
		}
	}
	return dst
}

// Encode returns the rate 1/2 C_RDATA blocks for a packet of the given
// number of blocks: one block with 64 flags, or two blocks with 127 flags
// when blocks is over 64. Flags for blocks past the packet are 1. It
// allocates the blocks; AppendEncode does not.
func (r *ResponseData) Encode(blocks int) ([]*pdu.Rate12Data, error) {
	data, err := r.AppendEncode(make([]pdu.Rate12Data, 0, 2), blocks)
	if err != nil {
		return nil, err
	}
	out := make([]*pdu.Rate12Data, len(data))
	for i := range data {
		out[i] = &data[i]
	}
	return out, nil
}

// AppendEncode appends the C_RDATA blocks that Encode returns to dst and
// returns the extended slice. It does not allocate when dst has room for two
// blocks. On error it returns dst unchanged.
func (r *ResponseData) AppendEncode(dst []pdu.Rate12Data, blocks int) ([]pdu.Rate12Data, error) {
	if blocks < 0 || blocks > maxBlocks {
		return dst, newCountError(ErrTooLong, blocks)
	}
	n, flags := 1, 64
	if blocks > 64 {
		n, flags = 2, maxBlocks
	}
	var buf [2 * 12]byte
	size := n * 12
	for i := range size - crcOctets {
		buf[i] = 0xFF
	}
	for i := 0; i < flags; i++ {
		if i < blocks && r.Retry[i] {
			buf[i/8] &^= 1 << (i % 8)
		}
	}
	binary.LittleEndian.PutUint32(buf[size-crcOctets:], crc.CRC32(buf[:size-crcOctets]))
	for i := range n {
		dst = append(dst, pdu.Rate12Data{Data: [12]byte(buf[i*12:])})
	}
	return dst, nil
}

// DecodeResponseData decodes one or two C_RDATA blocks and checks their CRC.
// On a CRC failure it returns the decoded flags with an error that wraps
// ErrResponseData and fec.ErrUncorrectable.
func DecodeResponseData(blocks ...*pdu.Rate12Data) (ResponseData, error) {
	var r ResponseData
	if len(blocks) != 1 && len(blocks) != 2 {
		return r, newCountError(ErrResponseData, len(blocks))
	}
	var buf [2 * 12]byte
	for i, b := range blocks {
		copy(buf[i*12:], b.Data[:])
	}
	err := decodeResponseData(&r, buf[:len(blocks)*12])
	return r, err
}

// decodeResponseData decodes the C_RDATA octets buf into r.
func decodeResponseData(r *ResponseData, buf []byte) error {
	clear(r.Retry[:])
	if len(buf) != 12 && len(buf) != 24 {
		return errResponseLength
	}
	flags := 64
	if len(buf) == 24 {
		flags = maxBlocks
	}
	for i := 0; i < flags; i++ {
		r.Retry[i] = buf[i/8]&(1<<(i%8)) == 0
	}
	if !checkMessageCRC(buf) {
		return errResponseCRC
	}
	return nil
}

// NewResponse builds the response packet a recipient sends for the
// confirmed packet with header to. The response goes from the packet's
// destination to its source with the same SAP. Status is the packet's
// N(S), or the 3 LSBs of its FSN for ResponseNACKFSNOutOfSequence. A
// ResponseSACK carries data as C_RDATA blocks; data is ignored otherwise.
// The Packet has its own storage; Builder.NewResponse builds into reused
// storage.
//
//go:noinline
func NewResponse(to *pdu.ConfirmedDataHeader, r Response, data *ResponseData) (Packet, error) {
	var b Builder
	return b.NewResponse(to, r, data)
}

// NewResponse does what the package function NewResponse does, into the
// storage of b. It does not allocate once the storage has room for two
// blocks.
func (b *Builder) NewResponse(to *pdu.ConfirmedDataHeader, r Response, data *ResponseData) (Packet, error) {
	status := to.SendSequenceNumber
	if r == ResponseNACKFSNOutOfSequence {
		status = to.FragmentSequenceNumber & 0b111
	}
	b.response = pdu.ResponsePacketHeader{
		SAP:             to.SAP,
		LLIDDestination: to.LLIDSource,
		LLIDSource:      to.LLIDDestination,
		FullMessageFlag: true,
		ResponseClass:   r.Class(),
		ResponseType:    r.Type(),
		ResponseStatus:  status,
	}
	var blocks []layer2.Data
	if r == ResponseSACK {
		var none ResponseData
		if data == nil {
			data = &none
		}
		var err error
		b.rate12, err = data.AppendEncode(appendLen(b.rate12, 2)[:0], int(to.BlocksToFollow))
		if err != nil {
			return Packet{}, err
		}
		b.blocks = appendLen(b.blocks, len(b.rate12))
		for i := range b.rate12 {
			b.blocks[i] = &b.rate12[i]
		}
		blocks = b.blocks
		b.response.BlocksToFollow = uint8(len(blocks) & 0xFF)
	}
	return Packet{
		Header: pdu.DataHeader{Format: pdu.FormatResponsePacket, ResponsePacketHeader: &b.response},
		Blocks: blocks,
	}, nil
}
