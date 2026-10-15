package packet

import (
	"encoding/binary"
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Message is a reassembled packet. Without Assembler.Reuse it has its own
// storage. With Reuse, Data, the sub-headers of Header, Proprietary and
// Response point into the storage of the Assembler and are valid until its
// next Add, AddBurst or Reset.
type Message struct {
	// Header is the first header block.
	Header pdu.DataHeader
	// Proprietary is the P_HEAD sent as the second header, or nil.
	Proprietary *pdu.ProprietaryHeader
	// DataType is the rate of the data blocks, or 0 without blocks.
	DataType enums.DataType
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
// completes it. The Assembler copies the headers and blocks it gets, so the
// caller can reuse a layer2.Burst or PDU after Add or AddBurst returns. The
// zero value is ready to use. An Assembler is not safe for concurrent use.
type Assembler struct {
	// Reuse makes the Assembler keep its storage from packet to packet, so
	// that Add and AddBurst do not allocate once the storage has grown to
	// the largest packet, and Response does not allocate. A Message then
	// points into this storage, and a Packet from Response is valid until
	// the next Response. False, the default, gives each Message and Packet
	// its own storage. Reset keeps Reuse.
	Reuse bool

	msg       Message
	active    bool
	confirmed bool
	hasBlocks bool
	total     int
	expect    int
	count     int
	per       int
	ns        uint8
	valid     [maxBlocks]bool
	blocks    []byte
	store     store
	builder   Builder
}

// store holds copies of the headers and the response data that a Message
// points to.
type store struct {
	unconfirmed    pdu.UnconfirmedDataHeader
	confirmed      pdu.ConfirmedDataHeader
	response       pdu.ResponsePacketHeader
	defined        pdu.DefinedDataHeader
	statusPrecoded pdu.StatusPrecodedHeader
	raw            pdu.RawDataHeader
	udt            pdu.UDTHeader
	proprietary    pdu.ProprietaryHeader
	second         pdu.ProprietaryHeader
	responseData   ResponseData
}

// copyHeader sets *dst to *src with each sub-header copied into s.
func (s *store) copyHeader(dst, src *pdu.DataHeader) {
	*dst = *src
	if src.UnconfirmedDataHeader != nil {
		s.unconfirmed = *src.UnconfirmedDataHeader
		dst.UnconfirmedDataHeader = &s.unconfirmed
	}
	if src.ConfirmedDataHeader != nil {
		s.confirmed = *src.ConfirmedDataHeader
		dst.ConfirmedDataHeader = &s.confirmed
	}
	if src.ResponsePacketHeader != nil {
		s.response = *src.ResponsePacketHeader
		dst.ResponsePacketHeader = &s.response
	}
	if src.DefinedDataHeader != nil {
		s.defined = *src.DefinedDataHeader
		dst.DefinedDataHeader = &s.defined
	}
	if src.StatusPrecodedHeader != nil {
		s.statusPrecoded = *src.StatusPrecodedHeader
		dst.StatusPrecodedHeader = &s.statusPrecoded
	}
	if src.RawDataHeader != nil {
		s.raw = *src.RawDataHeader
		dst.RawDataHeader = &s.raw
	}
	if src.UDTHeader != nil {
		s.udt = *src.UDTHeader
		dst.UDTHeader = &s.udt
	}
	if src.ProprietaryHeader != nil {
		s.proprietary = *src.ProprietaryHeader
		dst.ProprietaryHeader = &s.proprietary
	}
}

// Reset discards any partial packet. It keeps Reuse and, with Reuse, the
// storage.
func (a *Assembler) Reset() {
	a.msg = Message{}
	a.active, a.confirmed, a.hasBlocks = false, false, false
	a.total, a.expect, a.count, a.per, a.ns = 0, 0, 0, 0, 0
	clear(a.valid[:])
	if !a.Reuse {
		a.blocks = nil
	}
}

// AddBurst adds a decoded burst. A burst that is not data, or whose data did
// not decode, is ignored. A data block with an uncorrectable FEC result is
// counted but not used, so the message CRC or CRC-9 catches it.
func (a *Assembler) AddBurst(b *layer2.Burst) (Message, bool, error) {
	if b.Kind != layer2.KindData || b.Data == nil {
		return Message{}, false, nil
	}
	if _, ok := blockOf(b.Data); ok && b.FEC.PDU.Uncorrectable {
		return a.addBlock(block{}, false)
	}
	return a.Add(b.Data)
}

// Add adds a decoded PDU: a *pdu.DataHeader, *pdu.Rate12Data,
// *pdu.Rate34Data or *pdu.Rate1Data, and ignores other PDUs. done is true
// when the packet is complete, with its Message; on ErrMessageCRC the
// Message is returned too. The package doc gives the rules for headers.
func (a *Assembler) Add(d layer2.Data) (Message, bool, error) {
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
		a.store.second = *h.ProprietaryHeader
		a.msg.Proprietary = &a.store.second
		a.expect--
		a.total--
		return a.finishIfEmpty()
	}
	if a.isRetry(h) {
		a.expect, a.count = int(h.ConfirmedDataHeader.BlocksToFollow), 0
		return Message{}, false, nil
	}
	a.Reset()
	confirmed, ok := isConfirmed(h)
	if !ok && (h.Format != pdu.FormatResponsePacket || h.ResponsePacketHeader == nil) {
		return Message{}, false, nil
	}
	a.active, a.confirmed = true, confirmed
	a.store.copyHeader(&a.msg.Header, h)
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
	a.own(&m)
	return m, true, nil
}

// own gives m its own copies of the storage it points to, unless the
// Assembler reuses its storage.
func (a *Assembler) own(m *Message) {
	if a.Reuse {
		return
	}
	s := new(store)
	s.copyHeader(&m.Header, &m.Header)
	if m.Proprietary != nil {
		s.second = *m.Proprietary
		m.Proprietary = &s.second
	}
	if m.Response != nil {
		s.responseData = *m.Response
		m.Response = &s.responseData
	}
}

func (a *Assembler) addBlock(b block, usable bool) (Message, bool, error) {
	if !a.active || a.count >= a.expect {
		return Message{}, false, ErrNoHeader
	}
	if usable {
		if !a.hasBlocks {
			per, err := blockOctets(b.dataType, a.confirmed)
			if err != nil {
				return Message{}, false, err
			}
			a.per, a.msg.DataType, a.hasBlocks = per, b.dataType, true
			a.blocks = resize(a.blocks, a.total*per)
		} else if b.dataType != a.msg.DataType {
			a.Reset()
			return Message{}, false, ErrRateChange
		}
	}
	idx := a.count
	a.count++
	if usable {
		a.storeBlock(idx, b)
	}
	if a.confirmed && a.hasBlocks && a.allValid() {
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

// storeBlock copies the octets of b to their place in the packet: arrival
// index idx for an unconfirmed block, the DBSN for a confirmed block with a
// good CRC-9.
func (a *Assembler) storeBlock(idx int, b block) {
	if !a.confirmed {
		if idx < a.total {
			copy(a.blocks[idx*a.per:], b.raw)
			a.valid[idx] = true
		}
		return
	}
	dbsn := int(b.raw[0] >> 1)
	if dbsn >= a.total || !b.confirmedOK() {
		return
	}
	copy(a.blocks[dbsn*a.per:], b.raw[2:])
	a.valid[dbsn] = true
}

func (a *Assembler) allValid() bool {
	for _, ok := range a.valid[:a.total] {
		if !ok {
			return false
		}
	}
	return true
}

// Missing returns the DBSNs of the current confirmed packet that have not
// arrived with a good CRC-9.
func (a *Assembler) Missing() []int {
	n := a.total
	if a.hasBlocks {
		n = 0
		for _, ok := range a.valid[:a.total] {
			if !ok {
				n++
			}
		}
	}
	if n == 0 {
		return nil
	}
	return a.AppendMissing(make([]int, 0, n))
}

// AppendMissing appends the DBSNs that Missing returns to dst and returns
// the extended slice. It does not allocate when dst has room.
func (a *Assembler) AppendMissing(dst []int) []int {
	for i, ok := range a.valid[:a.total] {
		if !ok || !a.hasBlocks {
			dst = append(dst, i)
		}
	}
	return dst
}

// Response builds the response packet for the current confirmed packet: a
// SACK for the blocks Missing returns. It returns ErrFormat when there is no
// confirmed packet in progress. After Add returns a confirmed Message, use
// NewResponse with ResponseACK, or ResponseNACKPacketCRC on ErrMessageCRC.
func (a *Assembler) Response() (Packet, error) {
	h := a.msg.Header.ConfirmedDataHeader
	if !a.active || h == nil {
		return Packet{}, errNoConfirmed
	}
	var rd ResponseData
	for i, ok := range a.valid[:a.total] {
		rd.Retry[i] = !ok || !a.hasBlocks
	}
	if a.Reuse {
		return a.builder.NewResponse(h, ResponseSACK, &rd)
	}
	return NewResponse(h, ResponseSACK, &rd)
}

func (a *Assembler) finish() (Message, bool, error) {
	m := a.msg
	buf := a.blocks[:0]
	complete := true
	if a.hasBlocks {
		buf = a.blocks
		for i, ok := range a.valid[:a.total] {
			if !ok {
				complete = false
				clear(buf[i*a.per : (i+1)*a.per])
			}
		}
	}
	a.Reset()
	err := a.fill(&m, buf, complete)
	a.own(&m)
	return m, true, err
}

// fill sets the Data, CRC and Response of m from the block octets buf and
// checks the message CRC, or the CRC of the response data.
func (a *Assembler) fill(m *Message, buf []byte, complete bool) error {
	if m.Header.Format == pdu.FormatResponsePacket {
		err := decodeResponseData(&a.store.responseData, buf)
		if err != nil && !errors.Is(err, fec.ErrUncorrectable) {
			return err
		}
		m.Response = &a.store.responseData
		m.Data = buf[:len(buf)-crcOctets]
		return err
	}
	if len(buf) < crcOctets {
		return ErrMessageCRC
	}
	n := len(buf) - crcOctets
	m.CRC = binary.LittleEndian.Uint32(buf[n:])
	m.Data = buf[:max(n-padOf(&m.Header), 0)]
	if !complete || !checkMessageCRC(buf) {
		return ErrMessageCRC
	}
	return nil
}
