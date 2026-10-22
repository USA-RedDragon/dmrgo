package packet

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Message is a reassembled packet. Data, Proprietary and Response point into
// the storage of the Assembler and are valid until its next Add, AddBurst,
// Lost or Reset.
type Message struct {
	// Header is the first header block.
	Header pdu.DataHeader
	// Proprietary is the P_HEAD sent as the second header, or nil.
	Proprietary *pdu.ProprietaryHeader
	// DataType is the rate of the data blocks. It is set only when Blocks
	// is over 0.
	DataType enums.DataType
	// Blocks is the number of data blocks that Data was built from, 0 for a
	// packet without data blocks, such as a header-only response, or one
	// whose blocks all failed their FEC.
	Blocks int
	// Data is the user data. The pad octets of a U_HEAD or C_HEAD packet are
	// removed when the header is a first try (Full Message Flag set). A
	// C_HEAD packet that a later try started, because the first header was
	// lost, has them removed too when every DBSN from 0 to one less than the
	// Blocks to Follow of the header that started it arrived, in that try or
	// a later one, and the message CRC holds: such a header, as a sender
	// sends it after no response or a NACK, keeps the Pad Octet Count of the
	// first try, though ETSI TS 102 361-1 clause 9.3.20 has a receiver read
	// it only on a first try. Otherwise a packet that a later try started
	// keeps them, and so does a U_HEAD packet with the Full Message Flag
	// clear, which no sender retries. A DD_HEAD
	// or R_HEAD packet has Bit Padding/8 octets removed on every try, as its
	// retries keep the Bit Padding of the first try.
	// It holds at most 3048 bytes: 127 blocks of at most 24 octets.
	// (ETSI TS 102 361-1 clause 9.3.20, clause 9.3.44)
	Data []byte
	// CRC is the received message CRC.
	CRC uint32
	// Response is the decoded C_RDATA of a SACK response packet, or nil.
	Response *ResponseData
	// BadCRC reports that the message CRC failed, that the Pad Octet Count
	// of a first try or the Bit Padding/8 is more than the octets before the
	// CRC, or, with
	// Assembler.AcceptBadCRC, that Data holds a confirmed block whose CRC-9
	// failed. (ETSI TS 102 361-1 clause 9.3.22)
	BadCRC bool
}

// UDPIPv4 decodes the compressed UDP/IPv4 header at the start of Data and
// returns it with the application data that follows it, which points into
// Data. ok is false when the header is not a U_HEAD or C_HEAD with the SAP
// UDP/IP header compression, or when Data is shorter than the compressed
// header. (ETSI TS 102 361-3 clause 5.6)
func (m *Message) UDPIPv4() (header pdu.UDPIPv4CompressedHeader, payload []byte, ok bool) {
	if m == nil {
		return pdu.UDPIPv4CompressedHeader{}, nil, false
	}
	var sap enums.SAP
	if u, ok := m.Header.AsUnconfirmed(); ok {
		sap = u.SAP
	} else if c, ok := m.Header.AsConfirmed(); ok {
		sap = c.SAP
	} else {
		return header, nil, false
	}
	if sap != enums.SAPUDPIPHeaderCompression || len(m.Data) < 5 {
		return header, nil, false
	}
	var raw [9]byte
	copy(raw[:], m.Data)
	header = pdu.DecodeUDPIPv4CompressedHeader(raw)
	n := header.Octets()
	if len(m.Data) < n {
		return pdu.UDPIPv4CompressedHeader{}, nil, false
	}
	return header, m.Data[n:], true
}

// Assembler collects the header and data blocks of one packet at a time on
// one channel and returns each packet with its message CRC checked. For a
// confirmed packet it tracks blocks by DBSN, so a retry of the failed blocks
// completes it. The Assembler copies the headers and blocks it gets, so the
// caller can reuse a layer2.Burst or PDU after Add or AddBurst returns. It
// keeps the storage that NewAssembler allocates from packet to packet, so
// that it does not allocate. The zero value has no storage for blocks, so
// it takes only packets without data blocks: a header of a packet with
// blocks returns ErrNoRoom. Use NewAssembler for others. A Message points into
// the Assembler, so an Assembler must not be copied after first use; go vet
// reports a copy. An Assembler is not safe for concurrent use.
type Assembler struct {
	// AcceptBadCRC makes the Assembler deliver a packet whose message CRC
	// failed, or whose confirmed blocks failed their CRC-9, as the "skip
	// CRC" setting of some radios does. The Message then has BadCRC set and
	// comes with no error. A confirmed block with a bad CRC-9 fills its DBSN
	// until a good copy arrives, and an attempt in which every DBSN arrived
	// completes the packet. A block that was lost, or whose FEC failed in
	// the PDU or in the payload of the burst (see AddBurst), is still
	// missing, and a header must still pass its CRC, because the
	// addresses must be right. AcceptBadCRC does not apply to a packet that
	// a later try started, whose size only a good message CRC confirms.
	// False, the default, returns ErrMessageCRC and ErrBlocksMissing as the
	// CRCs require. Reset keeps AcceptBadCRC.
	AcceptBadCRC bool

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
	bad       [maxBlocks]bool
	blocks    []byte
	store     store
	resp      responseStore
	noCopy    noCopy
}

// NewAssembler returns an Assembler for packets of up to maxBlocks data
// blocks of any rate. It allocates 24 bytes for each block on top of the
// Assembler itself, 532 bytes on a 32-bit target and 640 on a 64-bit one,
// so 16 blocks take about 0.9 KB on a microcontroller and 127, the most
// that a header counts, about 3.6 KB. A maxBlocks over 127 means 127, and
// one under 0 means 0.
func NewAssembler(maxBlocks int) *Assembler {
	return &Assembler{blocks: make([]byte, max(0, min(maxBlocks, maxBlocksPerPacket))*maxBlockOctets)}
}

type store struct {
	second       pdu.ProprietaryHeader
	responseData ResponseData
}

// Reset discards any partial packet. It keeps AcceptBadCRC.
func (a *Assembler) Reset() {
	if a == nil {
		return
	}
	a.msg = Message{}
	a.active, a.confirmed, a.hasBlocks = false, false, false
	a.total, a.expect, a.count, a.per, a.ns = 0, 0, 0, 0, 0
	clear(a.valid[:])
	clear(a.bad[:])
}

// InProgress reports whether a header has started a packet that has not
// completed or been reset. It is true for confirmed, unconfirmed and
// response packets, after a proprietary header, and while a confirmed
// packet waits for retried blocks.
func (a *Assembler) InProgress() bool {
	if a == nil {
		return false
	}
	return a.active
}

// AddBurst adds a decoded burst. A burst that is not data, or whose data did
// not decode, is ignored. A data block whose FEC.PDU or FEC.Payload result
// is uncorrectable, such as a rate 3/4 block whose trellis decode failed,
// is counted but not used, as Lost counts it, so the message CRC or the
// SACK catches it. A nil b, or a b whose Data is a nil pointer, is ignored.
func (a *Assembler) AddBurst(b *layer2.Burst) (Message, bool, error) {
	if a == nil {
		return Message{}, false, dmrgo.ErrNilReceiver
	}
	if b == nil || b.Kind != layer2.KindData || nilData(b.Data) {
		return Message{}, false, nil
	}
	if _, ok := blockOf(b.Data); ok && (b.FEC.PDU.Uncorrectable || b.FEC.Payload.Uncorrectable) {
		return a.addBlock(block{}, false)
	}
	return a.Add(b.Data)
}

// Lost tells the Assembler that one block burst of the packet in progress
// was not received, for example because its slot passed without a SYNC on
// RF. The block counts as a block whose CRC failed, so the last block of
// an attempt still completes the attempt: an unconfirmed packet returns its
// Message with ErrMessageCRC, and a confirmed packet returns
// ErrBlocksMissing, so that Response builds the SACK. Lost does nothing
// when no packet waits for blocks.
func (a *Assembler) Lost() (Message, bool, error) {
	if a == nil {
		return Message{}, false, dmrgo.ErrNilReceiver
	}
	if !a.active || a.count >= a.expect {
		return Message{}, false, nil
	}
	return a.addBlock(block{}, false)
}

// Add adds a decoded PDU: a *pdu.DataHeader, *pdu.Rate12Data,
// *pdu.Rate34Data or *pdu.Rate1Data, and ignores other PDUs. done is true
// when the packet is complete, with its Message; on ErrMessageCRC the
// Message is returned too. A header with pdu.DataHeader.FEC.Uncorrectable
// set returns ErrHeaderCRC and changes nothing. A nil pointer of one of
// these types returns dmrgo.ErrNilArgument and changes nothing. The package
// doc gives the rules for headers.
func (a *Assembler) Add(d layer2.Data) (Message, bool, error) {
	if a == nil {
		return Message{}, false, dmrgo.ErrNilReceiver
	}
	switch v := d.(type) {
	case *pdu.DataHeader:
		if v == nil {
			return Message{}, false, dmrgo.ErrNilArgument
		}
		return a.addHeader(v)
	case *pdu.Rate12Data, *pdu.Rate34Data, *pdu.Rate1Data:
		b, ok := blockOf(d)
		if !ok {
			return Message{}, false, dmrgo.ErrNilArgument
		}
		return a.addBlock(b, true)
	default:
		return Message{}, false, nil
	}
}

func nilData(d layer2.Data) bool {
	switch v := d.(type) {
	case nil:
		return true
	case *pdu.DataHeader:
		return v == nil
	case *pdu.Rate12Data:
		return v == nil
	case *pdu.Rate34Data:
		return v == nil
	case *pdu.Rate1Data:
		return v == nil
	}
	return false
}

func (a *Assembler) addHeader(h *pdu.DataHeader) (Message, bool, error) {
	if h.FEC.Uncorrectable {
		return Message{}, false, ErrHeaderCRC
	}
	if p, ok := h.AsProprietary(); ok {
		if !a.active || a.count != 0 || a.msg.Proprietary != nil || a.expect == 0 {
			return Message{}, false, ErrNoHeader
		}
		a.store.second = p
		a.msg.Proprietary = &a.store.second
		a.expect--
		a.total--
		return a.finishIfEmpty()
	}
	if a.isRetry(h) {
		a.expect, a.count = blocksOf(h), 0
		return Message{}, false, nil
	}
	if !fullMessage(h) && a.active && a.count < a.expect {
		return Message{}, false, ErrUnexpectedRetry
	}
	a.Reset()
	confirmed, ok := isConfirmed(h)
	if _, isResponse := h.AsResponse(); !ok && !isResponse {
		return Message{}, false, nil
	}
	if blocksOf(h)*maxBlockOctets > len(a.blocks) {
		return Message{}, false, ErrNoRoom
	}
	a.active, a.confirmed = true, confirmed
	a.msg.Header = *h
	a.total = blocksOf(h)
	a.expect = a.total
	if c, ok := h.AsConfirmed(); ok {
		a.ns = c.SendSequenceNumber
	}
	return a.finishIfEmpty()
}

func (a *Assembler) isRetry(h *pdu.DataHeader) bool {
	if !a.active || !a.confirmed || h.Format() != a.msg.Header.Format() || fullMessage(h) && a.count < a.expect {
		return false
	}
	prev := &a.msg.Header
	if c, ok := h.AsConfirmed(); ok {
		p, _ := prev.AsConfirmed()
		return c.SAP == p.SAP && c.LLIDSource == p.LLIDSource &&
			c.LLIDDestination == p.LLIDDestination && c.SendSequenceNumber == a.ns
	}
	if c, ok := h.AsDefinedData(); ok {
		p, _ := prev.AsDefinedData()
		return c.SARQ && c.SAP == p.SAP && c.LLIDSource == p.LLIDSource &&
			c.LLIDDestination == p.LLIDDestination && c.DefinedData == p.DefinedData
	}
	if c, ok := h.AsRawData(); ok {
		p, pok := prev.AsRawData()
		return pok && c.SARQ && c.SAP == p.SAP && c.LLIDSource == p.LLIDSource && c.LLIDDestination == p.LLIDDestination &&
			c.SourcePort == p.SourcePort && c.DestinationPort == p.DestinationPort
	}
	return false
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
		if !a.hasBlocks {
			per, err := blockOctets(b.dataType, a.confirmed)
			if err != nil {
				return Message{}, false, err
			}
			a.per, a.msg.DataType, a.hasBlocks = per, b.dataType, true
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
	if !a.confirmed || a.acceptBadCRC() && a.allPresent() {
		return a.finish()
	}
	return Message{}, false, ErrBlocksMissing
}

func (a *Assembler) storeBlock(idx int, b block) {
	if !a.confirmed {
		if idx < a.total {
			copy(a.blocks[idx*a.per:], b.raw)
			a.valid[idx] = true
		}
		return
	}
	dbsn := int(b.raw[0] >> 1)
	if dbsn >= a.total || a.valid[dbsn] {
		return
	}
	ok := b.confirmedOK()
	if !ok && !a.acceptBadCRC() {
		return
	}
	copy(a.blocks[dbsn*a.per:], b.raw[2:])
	a.valid[dbsn], a.bad[dbsn] = ok, !ok
}

func (a *Assembler) acceptBadCRC() bool {
	return a.AcceptBadCRC && fullMessage(&a.msg.Header)
}

func (a *Assembler) allPresent() bool {
	for i, ok := range a.valid[:a.total] {
		if !ok && !a.bad[i] {
			return false
		}
	}
	return true
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
// arrived with a good CRC-9. It returns at most 127 block numbers. It
// allocates the slice; AppendMissing does not.
func (a *Assembler) Missing() []int {
	if a == nil {
		return nil
	}
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
// the extended slice. It does not allocate when dst has room for 127 block
// numbers.
func (a *Assembler) AppendMissing(dst []int) []int {
	if a == nil {
		return dst
	}
	for i, ok := range a.valid[:a.total] {
		if !ok || !a.hasBlocks {
			dst = append(dst, i)
		}
	}
	return dst
}

// Response builds the response packet for the current confirmed packet: a
// SACK for the blocks Missing returns. It returns ErrFormat when there is no
// confirmed packet in progress, and ErrNoFirstTry for a packet that a later
// try started. After Add returns a confirmed Message, use
// NewResponse with ResponseACK, or ResponseNACKPacketCRC on ErrMessageCRC.
func (a *Assembler) Response() (Packet, error) {
	if a == nil {
		return Packet{}, dmrgo.ErrNilReceiver
	}
	h, ok := a.msg.Header.AsConfirmed()
	if !a.active || !ok {
		return Packet{}, errNoConfirmed
	}
	if !fullMessage(&a.msg.Header) {
		return Packet{}, ErrNoFirstTry
	}
	var rd ResponseData
	for i, ok := range a.valid[:a.total] {
		rd.Retry[i] = !ok || !a.hasBlocks
	}
	return a.resp.build(&h, ResponseSACK, &rd)
}

func (a *Assembler) finish() (Message, bool, error) {
	m := a.msg
	buf := a.blocks[:0]
	complete := a.hasBlocks
	if a.hasBlocks {
		buf = a.blocks[:a.total*a.per]
		m.Blocks = a.total
		for i, ok := range a.valid[:a.total] {
			switch {
			case a.bad[i]:
				m.BadCRC = true
			case !ok:
				complete = false
				clear(buf[i*a.per : (i+1)*a.per])
			}
		}
	}
	accept := a.acceptBadCRC()
	a.Reset()
	err := a.fill(&m, buf, complete)
	if err != nil && m.Header.Format() != pdu.FormatResponsePacket && complete && accept {
		err = nil
	}
	return m, true, err
}

func (a *Assembler) fill(m *Message, buf []byte, complete bool) error {
	if m.Header.Format() == pdu.FormatResponsePacket {
		if len(buf) != 12 && len(buf) != 24 {
			return errResponseLength
		}
		err := decodeResponseData(&a.store.responseData, buf)
		m.Response = &a.store.responseData
		m.Data = buf[:len(buf)-crcOctets]
		return err
	}
	if len(buf) < crcOctets {
		m.BadCRC = true
		return ErrMessageCRC
	}
	n := len(buf) - crcOctets
	m.CRC = binary.LittleEndian.Uint32(buf[n:])
	m.Data = buf[:n]
	crcOK := checkMessageCRC(buf)
	switch pad := padOf(&m.Header); {
	case fullMessage(&m.Header) || shortData(&m.Header):
		m.Data = buf[:max(n-pad, 0)]
		m.BadCRC = m.BadCRC || pad > n
	case complete && crcOK && pad <= n && m.Header.Format() == pdu.FormatConfirmed:
		m.Data = buf[:n-pad]
	}
	if !crcOK {
		m.BadCRC = true
	}
	if !complete || m.BadCRC {
		return ErrMessageCRC
	}
	return nil
}
