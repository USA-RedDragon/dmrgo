package pdu

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// The CSBK opcodes of MOTOTRBO Capacity Plus and Linked Capacity Plus (FID
// 0x10). No public spec defines them; the layouts come from recordings of
// real systems. They share values with standard opcodes, as the other
// Motorola opcodes do.
const (
	CSBKCapacityPlusDataRevertAnnouncement CSBKOpcode = 0b00101001 // enhanced data revert window announcement
	CSBKCapacityPlusDataRevertGrant        CSBKOpcode = 0b00101010 // enhanced data revert window grant
	CSBKCapacityPlusNeighbors              CSBKOpcode = 0b00111011 // neighbor sites of a Linked Capacity Plus site
	CSBKCapacityPlusPreamble               CSBKOpcode = 0b00111101 // preamble with the rest LSN
	CSBKCapacityPlusSiteStatus             CSBKOpcode = 0b00111110 // rest LSN and channel activity
)

// The Full LC opcodes of the Capacity Plus call grants (FID 0x10), sent in
// the Voice LC Header on the rest channel.
const (
	FLCOCapacityPlusGroupVoiceGrant   enums.FLCO = 0b000100 // group voice call grant
	FLCOCapacityPlusPrivateVoiceGrant enums.FLCO = 0b000111 // private voice call grant
)

// SLCOCapacityPlusRestChannel is the Short LC opcode of the Capacity Plus
// rest channel announcement.
const SLCOCapacityPlusRestChannel enums.SLCO = 0b1111

// CapacityPlusMaxLSN is the highest logical slot number (LSN) of a
// Capacity Plus site: 8 repeaters of 2 timeslots.
const CapacityPlusMaxLSN = 16

// CapacityPlusSegment shows where a CSBK block sits in a site status or
// neighbor report that spans several CSBKs.
type CapacityPlusSegment uint8

// The CapacityPlusSegment values.
const (
	CapacityPlusSegmentContinuation CapacityPlusSegment = iota // a middle block
	CapacityPlusSegmentLast                                    // the last block
	CapacityPlusSegmentFirst                                   // the first block
	CapacityPlusSegmentSingle                                  // the only block
)

// String returns the name of the CapacityPlusSegment. It allocates only for
// a value without a name.
func (s CapacityPlusSegment) String() string {
	switch s {
	case CapacityPlusSegmentContinuation:
		return "Continuation"
	case CapacityPlusSegmentLast:
		return "Last"
	case CapacityPlusSegmentFirst:
		return "First"
	case CapacityPlusSegmentSingle:
		return "Single"
	}
	return "Unknown CapacityPlusSegment: " + text.Int(int64(s))
}

// Errors of the Capacity Plus site status. Match them with errors.Is.
var (
	// ErrCapacityPlusSequence is returned by
	// CapacityPlusSiteStatusAssembler.Add for a continuation or last block
	// without a first block, and for a status longer than
	// CapacityPlusMaxBlocks blocks. The assembler drops the partial status.
	ErrCapacityPlusSequence = errors.New("pdu: capacity plus site status block out of sequence")
	// ErrCapacityPlusIncomplete is returned by
	// CapacityPlusSiteStatusAssembler.Activity when no status is complete,
	// and when a bitmap names more talkgroups or radios than the status
	// holds. The activity then holds what the status does hold.
	ErrCapacityPlusIncomplete = errors.New("pdu: capacity plus site status incomplete")
)

// CapacityPlusChannel returns the repeater and timeslot of a logical slot
// number: LSN 1 and 2 are timeslots 1 and 2 of repeater 1, LSN 3 and 4 of
// repeater 2, and so on. ok is false for LSN 0 and above
// CapacityPlusMaxLSN.
func CapacityPlusChannel(lsn uint8) (repeater uint8, slot enums.Slot, ok bool) {
	if lsn == 0 || lsn > CapacityPlusMaxLSN {
		return 0, enums.SlotUnknown, false
	}
	return (lsn + 1) / 2, enums.Slot1 + enums.Slot(1-lsn%2), true
}

// CapacityPlusRestChannel is the payload of the Capacity Plus rest channel
// Short LC (SLCO 15): Short LC bits 4 to 27.
//
//dmr:subpdu
type CapacityPlusRestChannel struct {
	Marker   uint8 `dmr:"bits:0-7"`   // 0x10, the Motorola FID
	Flags    uint8 `dmr:"bits:8-10"`  // meaning unknown, seen 0 and 1
	RestLSN  uint8 `dmr:"bits:11-15"` // rest LSN, where the next call starts
	Site     uint8 `dmr:"bits:16-20"` // site number of a Linked Capacity Plus site, 0 for a single site
	Reserved uint8 `dmr:"bits:21-23"` // 0
}

// AsCapacityPlusRestChannel decodes the RawPayload of a Capacity Plus rest
// channel Short LC. ok is false for another SLCO, or when the payload does
// not start with the Motorola FID 0x10.
func (s *ShortLC) AsCapacityPlusRestChannel() (p CapacityPlusRestChannel, ok bool) {
	if s == nil {
		return CapacityPlusRestChannel{}, false
	}
	if s.SLCO != SLCOCapacityPlusRestChannel || s.RawPayload[0] != byte(enums.FIDMotorola) {
		return p, false
	}
	return decodeCapacityPlusRestChannel(s.RawPayload), true
}

// CapacityPlusSiteStatusPDU is one block of the Capacity Plus site status
// CSBK (FID 0x10, opcode 0x3E). Activity holds 7 octets of the activity
// stream that CapacityPlusSiteStatusAssembler joins and decodes.
//
//dmr:subpdu
type CapacityPlusSiteStatusPDU struct {
	Segment  CapacityPlusSegment `dmr:"bits:0-1"`      // place of the block in the status
	Slot2    bool                `dmr:"bit:2"`         // true on timeslot 2, false on timeslot 1
	RestLSN  uint8               `dmr:"bits:3-7"`      // rest LSN
	Activity [7]byte             `dmr:"bits:8-63,raw"` // 7 octets of the activity stream
}

// AsCapacityPlusSiteStatus decodes the RawPayload of a Capacity Plus site
// status CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsCapacityPlusSiteStatus() (p CapacityPlusSiteStatusPDU, ok bool) {
	if s == nil {
		return CapacityPlusSiteStatusPDU{}, false
	}
	if s.Opcode != CSBKCapacityPlusSiteStatus || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusSiteStatusPDU(s.RawPayload), true
}

// CapacityPlusNeighborsPDU is the payload of the Linked Capacity Plus
// neighbor report CSBK (FID 0x10, opcode 0x3B): the site number and the
// rest LSN of up to 6 neighbor sites.
//
//dmr:subpdu
type CapacityPlusNeighborsPDU struct {
	Segment   CapacityPlusSegment `dmr:"bits:0-1"`       // place of the block, CapacityPlusSegmentSingle seen
	Slot2     bool                `dmr:"bit:2"`          // true on timeslot 2, false on timeslot 1
	RestLSN   uint8               `dmr:"bits:3-7"`       // rest LSN of this site
	Async     bool                `dmr:"bit:8"`          // true for an asynchronous beacon, false for a periodic one
	Site      uint8               `dmr:"bits:9-12"`      // site number of this site
	Count     uint8               `dmr:"bits:13-15"`     // number of neighbors in Neighbors, 0 to 6
	Neighbors [6]byte             `dmr:"bits:16-63,raw"` // per neighbor, its site number in the high 4 bits and its rest LSN in the low 4 bits
}

// Neighbor returns the site number and rest LSN of neighbor i, from 0. ok
// is false when i is not below Count or 6.
func (p *CapacityPlusNeighborsPDU) Neighbor(i int) (site, restLSN uint8, ok bool) {
	if p == nil {
		return 0, 0, false
	}
	if i < 0 || i >= int(p.Count) || i >= len(p.Neighbors) {
		return 0, 0, false
	}
	return p.Neighbors[i] >> 4, p.Neighbors[i] & 0x0F, true
}

// AsCapacityPlusNeighbors decodes the RawPayload of a Linked Capacity Plus
// neighbor report CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsCapacityPlusNeighbors() (p CapacityPlusNeighborsPDU, ok bool) {
	if s == nil {
		return CapacityPlusNeighborsPDU{}, false
	}
	if s.Opcode != CSBKCapacityPlusNeighbors || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusNeighborsPDU(s.RawPayload), true
}

// CapacityPlusPreamblePDU is the payload of the Capacity Plus preamble CSBK
// (FID 0x10, opcode 0x3D). It has the layout of Pre_CSBK, with a 16-bit
// source and the rest LSN in place of the top octet of the source.
//
//dmr:subpdu
type CapacityPlusPreamblePDU struct {
	Data           bool   `dmr:"bit:0"`      // true when data follows the preambles, false when a CSBK follows
	Group          bool   `dmr:"bit:1"`      // true when TargetAddress is a talkgroup
	Reserved       uint8  `dmr:"bits:2-7"`   // 0
	BlocksToFollow uint8  `dmr:"bits:8-15"`  // number of blocks after this one
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target address
	Reserved2      uint8  `dmr:"bits:40-42"` // 0
	RestLSN        uint8  `dmr:"bits:43-47"` // rest LSN for the radios that do not take part
	SourceAddress  uint16 `dmr:"bits:48-63"` // 16-bit source address
}

// AsCapacityPlusPreamble decodes the RawPayload of a Capacity Plus
// preamble CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsCapacityPlusPreamble() (p CapacityPlusPreamblePDU, ok bool) {
	if s == nil {
		return CapacityPlusPreamblePDU{}, false
	}
	if s.Opcode != CSBKCapacityPlusPreamble || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusPreamblePDU(s.RawPayload), true
}

// CapacityPlusDataRevertAnnouncementPDU is the payload of the MOTOTRBO
// enhanced data revert window announcement CSBK (FID 0x10, opcode 0x29).
//
//dmr:subpdu
type CapacityPlusDataRevertAnnouncementPDU struct {
	Reserved      uint8  `dmr:"bits:0-7"`   // 0
	TargetAddress uint16 `dmr:"bits:8-23"`  // radio that the window is for, 0 for none
	Window        uint8  `dmr:"bits:24-31"` // window number
	Superframe    uint8  `dmr:"bits:32-39"` // superframe number
	Flags         uint8  `dmr:"bits:40-47"` // meaning unknown, 0xFC seen
	Reserved2     uint16 `dmr:"bits:48-63"` // meaning unknown, 0 seen
}

// AsCapacityPlusDataRevertAnnouncement decodes the RawPayload of an
// enhanced data revert window announcement CSBK. ok is false for another
// opcode or FID.
func (s *CSBK) AsCapacityPlusDataRevertAnnouncement() (p CapacityPlusDataRevertAnnouncementPDU, ok bool) {
	if s == nil {
		return CapacityPlusDataRevertAnnouncementPDU{}, false
	}
	if s.Opcode != CSBKCapacityPlusDataRevertAnnouncement || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusDataRevertAnnouncementPDU(s.RawPayload), true
}

// CapacityPlusDataRevertGrantPDU is the payload of the MOTOTRBO enhanced
// data revert window grant CSBK (FID 0x10, opcode 0x2A). No recording
// checks this layout.
//
//dmr:subpdu
type CapacityPlusDataRevertGrantPDU struct {
	Reserved      uint8  `dmr:"bits:0-7"`   // 0
	TargetAddress uint16 `dmr:"bits:8-23"`  // radio that gets the window
	Window        uint8  `dmr:"bits:24-31"` // window number
	Superframe    uint8  `dmr:"bits:32-39"` // superframe number
	Reserved2     uint32 `dmr:"bits:40-63"` // meaning unknown
}

// AsCapacityPlusDataRevertGrant decodes the RawPayload of an enhanced data
// revert window grant CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsCapacityPlusDataRevertGrant() (p CapacityPlusDataRevertGrantPDU, ok bool) {
	if s == nil {
		return CapacityPlusDataRevertGrantPDU{}, false
	}
	if s.Opcode != CSBKCapacityPlusDataRevertGrant || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusDataRevertGrantPDU(s.RawPayload), true
}

// CapacityPlusGroupVoiceGrant is the payload of the Capacity Plus group
// voice call grant Full LC (FID 0x10, FLCO 4): LC bits 16 to 71.
//
//dmr:subpdu
type CapacityPlusGroupVoiceGrant struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // service options of the call
	Reserved       uint16                `dmr:"bits:8-23"`         // 0
	GroupAddress   uint8                 `dmr:"bits:24-31"`        // 8-bit talkgroup
	Reserved2      uint8                 `dmr:"bits:32-34"`        // 0
	RestLSN        uint8                 `dmr:"bits:35-39"`        // rest LSN for the radios not in the call
	SourceAddress  uint16                `dmr:"bits:40-55"`        // 16-bit source address
}

// AsCapacityPlusGroupVoiceGrant decodes the RawPayload of a Capacity Plus
// group voice call grant. ok is false for another FLCO or FID.
func (s *FullLinkControl) AsCapacityPlusGroupVoiceGrant() (p CapacityPlusGroupVoiceGrant, ok bool) {
	if s == nil {
		return CapacityPlusGroupVoiceGrant{}, false
	}
	if s.FLCO != FLCOCapacityPlusGroupVoiceGrant || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusGroupVoiceGrant(s.RawPayload), true
}

// CapacityPlusPrivateVoiceGrant is the payload of the Capacity Plus private
// voice call grant Full LC (FID 0x10, FLCO 7): LC bits 16 to 71.
//
//dmr:subpdu
type CapacityPlusPrivateVoiceGrant struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // service options of the call
	TargetAddress  uint32                `dmr:"bits:8-31"`         // 24-bit target address
	Reserved       uint8                 `dmr:"bits:32-34"`        // 0
	RestLSN        uint8                 `dmr:"bits:35-39"`        // rest LSN for the radios not in the call
	SourceAddress  uint16                `dmr:"bits:40-55"`        // 16-bit source address
}

// AsCapacityPlusPrivateVoiceGrant decodes the RawPayload of a Capacity Plus
// private voice call grant. ok is false for another FLCO or FID.
func (s *FullLinkControl) AsCapacityPlusPrivateVoiceGrant() (p CapacityPlusPrivateVoiceGrant, ok bool) {
	if s == nil {
		return CapacityPlusPrivateVoiceGrant{}, false
	}
	if s.FLCO != FLCOCapacityPlusPrivateVoiceGrant || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeCapacityPlusPrivateVoiceGrant(s.RawPayload), true
}
