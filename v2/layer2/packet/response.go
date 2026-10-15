package packet

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Response is the Class and Type of a response packet (ETSI TS 102 361-1
// Table 8.3), as Class<<3 | Type.
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

// ErrResponseData is returned for C_RDATA blocks of the wrong count or with
// a bad CRC.
var ErrResponseData = errors.New("invalid response data blocks")

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

// ResponseToName returns the Table 8.3 name of r.
func ResponseToName(r Response) string {
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

// ResponseData is the selective retry bitmap of the C_RDATA PDU (Table
// 9.14, §8.2.2.3). Retry[i] asks for the block with DBSN i again. On air a
// flag is 1 for a received block, so the zero value retries nothing. Flag
// i is octet i/8, bit 7-i%8; the spec does not state the bit order, this
// follows the MSB-first order of every other DMR PDU.
type ResponseData struct {
	Retry [MaxBlocks]bool
}

// RetryBlocks returns the DBSNs flagged for retry in ascending order.
func (r *ResponseData) RetryBlocks() []int {
	var out []int
	for i, retry := range r.Retry {
		if retry {
			out = append(out, i)
		}
	}
	return out
}

// Encode returns the rate 1/2 C_RDATA blocks for a packet of the given
// number of blocks: one block with 64 flags, or two blocks with 127 flags
// when blocks is over 64. Flags for blocks past the packet are 1.
func (r *ResponseData) Encode(blocks int) ([]*pdu.Rate12Data, error) {
	if blocks < 0 || blocks > MaxBlocks {
		return nil, fmt.Errorf("%w: %d blocks", ErrTooLong, blocks)
	}
	n, flags := 1, 64
	if blocks > 64 {
		n, flags = 2, MaxBlocks
	}
	buf := make([]byte, n*12-CRCOctets)
	for i := range buf {
		buf[i] = 0xFF
	}
	for i := 0; i < flags; i++ {
		if i < blocks && r.Retry[i] {
			buf[i/8] &^= 0x80 >> (i % 8)
		}
	}
	buf = AppendMessageCRC(buf)
	out := make([]*pdu.Rate12Data, n)
	for i := range out {
		out[i] = &pdu.Rate12Data{DataType: elements.DataTypeRate12, Data: [12]byte(buf[i*12:])}
	}
	return out, nil
}

// DecodeResponseData decodes one or two C_RDATA blocks and checks their CRC.
func DecodeResponseData(blocks ...*pdu.Rate12Data) (ResponseData, error) {
	if len(blocks) != 1 && len(blocks) != 2 {
		return ResponseData{}, fmt.Errorf("%w: %d blocks", ErrResponseData, len(blocks))
	}
	buf := make([]byte, 0, len(blocks)*12)
	for _, b := range blocks {
		buf = append(buf, b.Data[:]...)
	}
	return decodeResponseData(buf)
}

func decodeResponseData(buf []byte) (ResponseData, error) {
	var r ResponseData
	if (len(buf) != 12 && len(buf) != 24) || !CheckMessageCRC(buf) {
		return r, fmt.Errorf("%w: bad length or CRC", ErrResponseData)
	}
	flags := 64
	if len(buf) == 24 {
		flags = MaxBlocks
	}
	for i := 0; i < flags; i++ {
		r.Retry[i] = buf[i/8]&(0x80>>(i%8)) == 0
	}
	return r, nil
}

// NewResponse builds the response packet a recipient sends for the
// confirmed packet with header to. The response goes from the packet's
// destination to its source with the same SAP. Status is the packet's
// N(S), or the 3 LSBs of its FSN for ResponseNACKFSNOutOfSequence. A
// ResponseSACK carries data as C_RDATA blocks; data is ignored otherwise.
func NewResponse(to *pdu.ConfirmedDataHeader, r Response, data *ResponseData) (Packet, error) {
	status := to.SendSequenceNumber
	if r == ResponseNACKFSNOutOfSequence {
		status = to.FragmentSequenceNumber & 0b111
	}
	h := &pdu.ResponsePacketHeader{
		SAP:             to.SAP,
		LLIDDestination: to.LLIDSource,
		LLIDSource:      to.LLIDDestination,
		FullMessageFlag: true,
		ResponseClass:   r.Class(),
		ResponseType:    r.Type(),
		ResponseStatus:  status,
	}
	var blocks []elements.Data
	if r == ResponseSACK {
		if data == nil {
			data = &ResponseData{}
		}
		rd, err := data.Encode(int(to.BlocksToFollow))
		if err != nil {
			return Packet{}, err
		}
		for _, b := range rd {
			blocks = append(blocks, b)
		}
		h.BlocksToFollow = uint8(len(blocks)) //nolint:gosec // 1 or 2
	}
	return Packet{
		Header: pdu.DataHeader{DataType: elements.DataTypeDataHeader, Format: pdu.FormatResponsePacket, ResponsePacketHeader: h},
		Blocks: blocks,
	}, nil
}
