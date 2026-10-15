package packet

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var (
	// ErrNoHeader is returned for a data block that no header announced.
	ErrNoHeader = errors.New("data block without a header")
	// ErrRateChange is returned when the data type changes within a packet
	// (§8.2.2.1). The partial packet is discarded.
	ErrRateChange = errors.New("data type changed within a packet")
	// ErrMessageCRC is returned with the Message whose message CRC failed.
	ErrMessageCRC = errors.New("message CRC-32 does not match")
	// ErrBlocksMissing is returned when the blocks of a confirmed packet
	// have all arrived but some failed their CRC-9. Missing lists them and
	// Response builds the SACK. The Assembler keeps the good blocks for the
	// retry.
	ErrBlocksMissing = errors.New("confirmed blocks failed CRC-9")
)

// Message is a reassembled packet.
type Message struct {
	// Header is the first header block.
	Header pdu.DataHeader
	// Proprietary is the P_HEAD sent as the second header, or nil.
	Proprietary *pdu.ProprietaryHeader
	// DataType is the rate of the data blocks, or 0 without blocks.
	DataType elements.DataType
	// Data is the user data. The pad octets of a U_HEAD or C_HEAD packet are
	// removed. A short data packet keeps all octets before the CRC; its Bit
	// Padding is in the header.
	Data []byte
	// CRC is the received message CRC.
	CRC uint32
	// Response is the decoded C_RDATA of a SACK response packet, or nil.
	Response *ResponseData
}

// Assembler collects the header and data blocks of one packet at a time on
// one channel and returns each packet with its message CRC checked. For a
// confirmed packet it tracks blocks by DBSN, so a retry of the failed blocks
// completes it. An Assembler is not safe for concurrent use.
type Assembler struct {
	msg       Message
	active    bool
	confirmed bool
	total     int
	expect    int
	count     int
	per       int
	blocks    [][]byte
	valid     []bool
	ns        uint8
}

// Reset discards any partial packet.
func (a *Assembler) Reset() {
	*a = Assembler{}
}

// AddBurst adds a decoded burst. A burst that is not data, or whose data did
// not decode, is ignored. A data block with an uncorrectable FEC result is
// counted but not used, so the message CRC or CRC-9 catches it.
func (a *Assembler) AddBurst(b *layer2.Burst) (Message, bool, error) {
	if !b.IsData || b.Data == nil {
		return Message{}, false, nil
	}
	if _, ok := blockOf(b.Data); ok && b.FEC.PDU.Uncorrectable {
		return a.addBlock(block{}, false)
	}
	return a.Add(b.Data)
}

// Add adds a decoded PDU: a *pdu.DataHeader, *pdu.Rate12Data,
// *pdu.Rate34Data or *pdu.Rate1Data. Other PDUs are ignored. done is true
// when the packet is complete, with its Message; on ErrMessageCRC the Message
// is returned too. A header other than a P_HEAD second
// header discards any partial packet, except a C_HEAD with the same SAP,
// LLIDs and N(S) as a confirmed packet that still has blocks missing, which
// starts the retry of those blocks. A header with no blocks to follow, such
// as an ACK, returns its Message at once. UDT and status precoded headers
// are ignored.
func (a *Assembler) Add(d elements.Data) (Message, bool, error) {
	switch v := d.(type) {
	case *pdu.DataHeader:
		return a.addHeader(v)
	default:
		b, ok := blockOf(d)
		if !ok {
			return Message{}, false, nil
		}
		return a.addBlock(b, true)
	}
}

func (a *Assembler) addHeader(h *pdu.DataHeader) (Message, bool, error) {
	if h.Format == pdu.FormatProprietary && h.ProprietaryHeader != nil {
		if !a.active || a.count != 0 || a.msg.Proprietary != nil || a.expect == 0 {
			return Message{}, false, ErrNoHeader
		}
		p := *h.ProprietaryHeader
		a.msg.Proprietary = &p
		a.expect--
		a.total--
		return a.finishIfEmpty()
	}
	if a.isRetry(h) {
		a.expect, a.count = int(h.ConfirmedDataHeader.BlocksToFollow), 0
		return Message{}, false, nil
	}
	a.Reset()
	confirmed, err := isConfirmed(h)
	if err != nil && (h.Format != pdu.FormatResponsePacket || h.ResponsePacketHeader == nil) {
		return Message{}, false, nil
	}
	a.active, a.confirmed = true, confirmed
	a.msg.Header = *h
	a.total = blocksOf(h)
	a.expect = a.total
	if h.ConfirmedDataHeader != nil && h.Format == pdu.FormatConfirmed {
		a.ns = h.ConfirmedDataHeader.SendSequenceNumber
	}
	return a.finishIfEmpty()
}

func (a *Assembler) isRetry(h *pdu.DataHeader) bool {
	if !a.active || !a.confirmed || h.Format != pdu.FormatConfirmed || h.ConfirmedDataHeader == nil {
		return false
	}
	prev := a.msg.Header.ConfirmedDataHeader
	if prev == nil || a.count < a.expect {
		return false
	}
	c := h.ConfirmedDataHeader
	return c.SAP == prev.SAP && c.LLIDSource == prev.LLIDSource &&
		c.LLIDDestination == prev.LLIDDestination && c.SendSequenceNumber == a.ns
}

func (a *Assembler) finishIfEmpty() (Message, bool, error) {
	if a.total > 0 {
		return Message{}, false, nil
	}
	m := a.msg
	a.Reset()
	return m, true, nil
}

func (a *Assembler) addBlock(b block, usable bool) (Message, bool, error) {
	if !a.active || a.count >= a.expect {
		return Message{}, false, ErrNoHeader
	}
	if usable {
		if a.blocks == nil {
			per, err := BlockOctets(b.dataType, a.confirmed)
			if err != nil {
				return Message{}, false, err
			}
			a.per, a.msg.DataType = per, b.dataType
			a.blocks = make([][]byte, a.total)
			a.valid = make([]bool, a.total)
		} else if b.dataType != a.msg.DataType {
			a.Reset()
			return Message{}, false, ErrRateChange
		}
	}
	idx := a.count
	a.count++
	if usable {
		a.store(idx, b)
	}
	if a.confirmed && a.blocks != nil && a.allValid() {
		return a.finish()
	}
	if a.count < a.expect {
		return Message{}, false, nil
	}
	if !a.confirmed {
		return a.finish()
	}
	return Message{}, false, ErrBlocksMissing
}

func (a *Assembler) store(idx int, b block) {
	if !a.confirmed {
		if idx < len(a.blocks) {
			a.blocks[idx], a.valid[idx] = append([]byte(nil), b.raw...), true
		}
		return
	}
	dbsn := int(b.raw[0] >> 1)
	if dbsn >= len(a.blocks) || !b.confirmedOK() {
		return
	}
	a.blocks[dbsn], a.valid[dbsn] = append([]byte(nil), b.raw[2:]...), true
}

func (a *Assembler) allValid() bool {
	for _, ok := range a.valid {
		if !ok {
			return false
		}
	}
	return true
}

// Missing returns the DBSNs of the current confirmed packet that have not
// arrived with a good CRC-9.
func (a *Assembler) Missing() []int {
	var out []int
	for i, ok := range a.valid {
		if !ok {
			out = append(out, i)
		}
	}
	if a.blocks == nil {
		for i := 0; i < a.total; i++ {
			out = append(out, i)
		}
	}
	return out
}

// Response builds the response packet for the current confirmed packet: a
// SACK for the blocks Missing returns. It returns ErrFormat when there is no
// confirmed packet in progress. After Add returns a confirmed Message, use
// NewResponse with ResponseACK, or ResponseNACKPacketCRC on ErrMessageCRC.
func (a *Assembler) Response() (Packet, error) {
	h := a.msg.Header.ConfirmedDataHeader
	if !a.active || h == nil {
		return Packet{}, fmt.Errorf("%w: no confirmed packet in progress", ErrFormat)
	}
	var rd ResponseData
	for _, i := range a.Missing() {
		rd.Retry[i] = true
	}
	return NewResponse(h, ResponseSACK, &rd)
}

func (a *Assembler) finish() (Message, bool, error) {
	m := a.msg
	var buf []byte
	complete := true
	for i, blk := range a.blocks {
		if !a.valid[i] {
			complete = false
			blk = make([]byte, a.per)
		}
		buf = append(buf, blk...)
	}
	a.Reset()
	if m.Header.Format == pdu.FormatResponsePacket {
		rd, err := decodeResponseData(buf)
		if err != nil {
			return m, true, err
		}
		m.Response = &rd
		m.Data = buf[:len(buf)-CRCOctets]
		return m, true, nil
	}
	if len(buf) < CRCOctets {
		return m, true, ErrMessageCRC
	}
	n := len(buf) - CRCOctets
	m.CRC = binary.LittleEndian.Uint32(buf[n:])
	end := n - padOf(&m.Header)
	if end < 0 {
		end = 0
	}
	m.Data = buf[:end]
	if !complete || !CheckMessageCRC(buf) {
		return m, true, ErrMessageCRC
	}
	return m, true, nil
}
