package data

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const slotPeriod = 60 * time.Millisecond

// Message is one data packet: its header, rate and user data.
type Message struct {
	// Header is the first header of the packet. Its format and payload give
	// the kind of packet and the addresses. The zero value is a UDT_HEAD with
	// LLIDs 0; set it with a SetX method of pdu.DataHeader.
	Header pdu.DataHeader
	// Proprietary is the P_HEAD sent as the second header, or nil.
	Proprietary *pdu.ProprietaryHeader
	// Rate is the data type of the blocks: enums.DataTypeRate12,
	// enums.DataTypeRate34 or enums.DataTypeRate1. For Sender.Send the
	// zero value means enums.DataTypeRate12.
	Rate enums.DataType
	// Payload is the user data, without the pad octets and the message CRC.
	// A packet whose first header was lost keeps its pad octets, as only a
	// first try counts them, unless a later try resent every block, as
	// packet.Message.Data describes. It holds at most MaxMessage octets
	// (ETSI TS 102 361-1 clause 9.3.20).
	Payload []byte
	// BadCRC reports a packet whose CRC failed, which a Receiver delivers
	// only with ReceiverConfig.AcceptBadCRC.
	BadCRC bool
	// Fragmented reports a packet that continues a message of several
	// fragments, with an FSN of 1 to 7 in its low bits, and so is not a
	// whole message. The first fragment, FSN 0000, is delivered as a whole
	// message, because repeaters and radios also send single packets with
	// FSN 0000; EventReassembled then carries the whole message.
	Fragmented bool
	// Fragments is the number of fragments of a reassembled message.
	Fragments uint16
}

// Call is the parties of a data call.
type Call struct {
	// Group reports whether Target is a talkgroup rather than a radio.
	Group bool
	// Target is the 24-bit destination LLID.
	Target uint32
	// Source is the 24-bit source LLID.
	Source uint32
}

func callOf(h *pdu.DataHeader) Call {
	target, source, _ := h.Addresses()
	return Call{Group: groupOf(h), Target: target, Source: source}
}

func sapOf(h *pdu.DataHeader) enums.SAP {
	if p, ok := h.AsUnconfirmed(); ok {
		return p.SAP
	}
	if p, ok := h.AsConfirmed(); ok {
		return p.SAP
	}
	return enums.SAPUnifiedDataTransport
}

func groupOf(h *pdu.DataHeader) bool {
	if p, ok := h.AsUnconfirmed(); ok {
		return p.Group
	}
	if p, ok := h.AsConfirmed(); ok {
		return p.Group
	}
	if p, ok := h.AsDefinedData(); ok {
		return p.Group
	}
	if p, ok := h.AsRawData(); ok {
		return p.Group
	}
	if p, ok := h.AsStatusPrecoded(); ok {
		return p.Group
	}
	if p, ok := h.AsUDT(); ok {
		return p.Group
	}
	return false
}

type peer struct {
	source, target uint32
	seq            uint8
	acked          uint8
	hasAcked       bool
	stamp          uint32
	used           bool
	valid          bool
}

type peers struct {
	list  []peer
	clock uint32
}

func (p *peers) lookup(source, target uint32) (e *peer, found bool) {
	p.clock++
	oldest := 0
	for i := range p.list {
		e := &p.list[i]
		if e.used && e.source == source && e.target == target {
			e.stamp = p.clock
			return e, true
		}
		if o := &p.list[oldest]; o.used && (!e.used || e.stamp < o.stamp) {
			oldest = i
		}
	}
	if len(p.list) == 0 {
		return nil, false
	}
	e = &p.list[oldest]
	*e = peer{source: source, target: target, stamp: p.clock, used: true}
	return e, false
}

func (p *peers) reset() {
	clear(p.list)
	p.clock = 0
}
