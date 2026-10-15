package packet

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// maxBlocks is the largest number of blocks that follow a header: the
// Blocks To Follow element has 7 bits.
const maxBlocks = 127

// crcOctets is the length of the message CRC at the end of the last block.
const crcOctets = 4

var (
	// ErrRate is returned for a data type other than rate 1/2, 3/4 or 1.
	ErrRate = errors.New("data type is not rate 1/2, 3/4 or 1 coded")
	// ErrFormat is returned by Segment for a header that is not U_HEAD,
	// C_HEAD, DD_HEAD or R_HEAD, or whose matching sub-header is nil.
	ErrFormat = errors.New("header format does not carry segmented data")
	// ErrTooLong is returned when the payload needs more than 127 blocks,
	// or more than the header can count.
	ErrTooLong = errors.New("payload needs too many blocks")
)

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
		return 0, fmt.Errorf("%w: %s", ErrRate, rate.String())
	default:
		return 0, fmt.Errorf("%w: %s", ErrRate, rate.String())
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

// Segment splits payload into the data blocks of one packet at the given
// rate and returns them with a copy of header. It sets the block count and
// pad fields of the header, gives confirmed blocks DBSN 0, 1, 2 and so on,
// and keeps the other header fields. It returns an error that wraps
// ErrRate, ErrFormat or ErrTooLong.
func Segment(header pdu.DataHeader, payload []byte, rate enums.DataType) (Packet, error) {
	confirmed, err := setLayout(&header, len(payload), rate)
	if err != nil {
		return Packet{}, err
	}
	per, _ := blockOctets(rate, confirmed)
	n := blocksOf(&header)
	buf := make([]byte, n*per-crcOctets)
	copy(buf, payload)
	buf = appendMessageCRC(buf)

	p := Packet{Header: header, Blocks: make([]layer2.Data, n)}
	for i := range p.Blocks {
		chunk := buf[i*per : (i+1)*per]
		if confirmed {
			p.Blocks[i] = confirmedBlock(uint8(i), chunk)
		} else {
			p.Blocks[i] = unconfirmedBlock(chunk)
		}
	}
	return p, nil
}

func setLayout(h *pdu.DataHeader, n int, rate enums.DataType) (bool, error) {
	confirmed, err := isConfirmed(h)
	if err != nil {
		return false, err
	}
	per, err := blockOctets(rate, confirmed)
	if err != nil {
		return false, err
	}
	blocks, pad := layout(n, per)
	switch h.Format {
	case pdu.FormatUnconfirmed:
		u := *h.UnconfirmedDataHeader
		u.BlocksToFollow, u.PadOctetCount = uint8(blocks&0xFF), uint8(pad&0xFF)
		h.UnconfirmedDataHeader = &u
	case pdu.FormatConfirmed:
		c := *h.ConfirmedDataHeader
		c.BlocksToFollow, c.PadOctetCount = uint8(blocks&0xFF), uint8(pad&0xFF)
		h.ConfirmedDataHeader = &c
	case pdu.FormatShortDataDefined:
		d := *h.DefinedDataHeader
		d.AppendedBlocks = uint8(blocks & 0xFF)
		h.DefinedDataHeader = &d
		h.AppendedBlocks = d.AppendedBlocks
	case pdu.FormatShortDataRawOrStatusPrecoded:
		r := *h.RawDataHeader
		r.AppendedBlocks = uint8(blocks & 0xFF)
		h.RawDataHeader = &r
		h.AppendedBlocks = r.AppendedBlocks
	case pdu.FormatUnifiedDataTransport, pdu.FormatResponsePacket, pdu.FormatProprietary:
	}
	limit := maxBlocks
	if h.Format == pdu.FormatShortDataDefined || h.Format == pdu.FormatShortDataRawOrStatusPrecoded {
		limit = 63
	}
	if blocks > limit {
		return false, fmt.Errorf("%w: %d blocks, at most %d", ErrTooLong, blocks, limit)
	}
	return confirmed, nil
}

func isConfirmed(h *pdu.DataHeader) (bool, error) {
	switch {
	case h.Format == pdu.FormatUnconfirmed && h.UnconfirmedDataHeader != nil:
		return false, nil
	case h.Format == pdu.FormatConfirmed && h.ConfirmedDataHeader != nil:
		return true, nil
	case h.Format == pdu.FormatShortDataDefined && h.DefinedDataHeader != nil:
		return h.DefinedDataHeader.SARQ, nil
	case h.Format == pdu.FormatShortDataRawOrStatusPrecoded && h.RawDataHeader != nil:
		return h.RawDataHeader.SARQ, nil
	}
	return false, fmt.Errorf("%w: %s", ErrFormat, h.Format.String())
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

func padOf(h *pdu.DataHeader) int {
	switch {
	case h.UnconfirmedDataHeader != nil && h.Format == pdu.FormatUnconfirmed:
		return int(h.UnconfirmedDataHeader.PadOctetCount)
	case h.ConfirmedDataHeader != nil && h.Format == pdu.FormatConfirmed:
		return int(h.ConfirmedDataHeader.PadOctetCount)
	}
	return 0
}

func unconfirmedBlock(chunk []byte) layer2.Data {
	switch len(chunk) {
	case 18:
		b := &pdu.Rate34Data{}
		copy(b.Data[:], chunk)
		return b
	case 24:
		b := &pdu.Rate1Data{}
		copy(b.Data[:], chunk)
		return b
	default:
		b := &pdu.Rate12Data{}
		copy(b.Data[:], chunk)
		return b
	}
}

func confirmedBlock(dbsn uint8, chunk []byte) layer2.Data {
	switch len(chunk) {
	case 16:
		b := &pdu.Rate34Data{}
		b.SetConfirmed(dbsn, [16]byte(chunk))
		return b
	case 22:
		b := &pdu.Rate1Data{}
		b.SetConfirmed(dbsn, [22]byte(chunk))
		return b
	default:
		b := &pdu.Rate12Data{}
		b.SetConfirmed(dbsn, [10]byte(chunk))
		return b
	}
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
