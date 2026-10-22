package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// UnconfirmedDataHeader is the unconfirmed data header (U_HEAD) (ETSI TS 102
// 361-1 clause 9.2.6).
//
//dmr:const 4-7=FormatUnconfirmed
type UnconfirmedDataHeader struct {
	Group                  bool      `dmr:"bit:0"`        // true when LLIDDestination is a group
	ResponseRequested      bool      `dmr:"bit:1"`        // Response Requested (A)
	Reserved               bool      `dmr:"bit:2"`        // reserved
	PadOctetCount          uint8     `dmr:"bits:3+12-15"` // Pad Octet Count (POC); bit 3 is the MSB
	SAP                    enums.SAP `dmr:"bits:8-11"`    // SAP identifier
	LLIDDestination        uint32    `dmr:"bits:16-39"`   // destination LLID
	LLIDSource             uint32    `dmr:"bits:40-63"`   // source LLID
	FullMessageFlag        bool      `dmr:"bit:64"`       // Full Message Flag (F)
	BlocksToFollow         uint8     `dmr:"bits:65-71"`   // Blocks to Follow (BF)
	Reserved2              uint8     `dmr:"bits:72-75"`   // reserved
	FragmentSequenceNumber uint8     `dmr:"bits:76-79"`   // Fragment Sequence Number (FSN)
}

// DataHeader is the data header PDU. The Data Packet Format in RawPayload
// selects its view: AsConfirmed, for example, decodes it for
// FormatConfirmed. Format 0 is FormatUnifiedDataTransport, so the zero value
// is a valid UDT_HEAD with LLIDs 0 that encodes with a good CRC: set a header
// with a SetX method before it goes on air. (ETSI TS 102 361-1 clause 9.2)
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0xCCCC
//dmr:input_size 96
//dmr:get Format Format bits:4-7 the Data Packet Format (DPF) of the header
//dmr:get appendedBlocks uint8 bits:2-3+12-15,only:Format=FormatShortDataDefined|FormatShortDataRawOrStatusPrecoded the Appended Blocks (AB)
//dmr:variant Unconfirmed UnconfirmedDataHeader Format=FormatUnconfirmed
//dmr:variant Confirmed ConfirmedDataHeader Format=FormatConfirmed
//dmr:variant Response ResponsePacketHeader Format=FormatResponsePacket
//dmr:variant DefinedData DefinedDataHeader Format=FormatShortDataDefined
//dmr:variant StatusPrecoded StatusPrecodedHeader Format=FormatShortDataRawOrStatusPrecoded appendedBlocks==0
//dmr:variant RawData RawDataHeader Format=FormatShortDataRawOrStatusPrecoded appendedBlocks!=0
//dmr:variant UDT UDTHeader Format=FormatUnifiedDataTransport
//dmr:variant Proprietary ProprietaryHeader Format=FormatProprietary
type DataHeader struct {
	FEC fec.Result `dmr:"-"` // result of the header CRC check

	// RawPayload holds header bits 0 to 79, packed most significant bit
	// first: bit 0 is the top bit of byte 0.
	RawPayload [10]byte `dmr:"bits:0-79,raw,union"`
}

// Addresses returns the destination LLID as target and the source LLID of
// the header. ok is false for a proprietary header, a header of an unknown
// format and a nil s.
func (s *DataHeader) Addresses() (target, source uint32, ok bool) {
	if s == nil {
		return 0, 0, false
	}
	if p, ok := s.AsUnconfirmed(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsConfirmed(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsResponse(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsDefinedData(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsStatusPrecoded(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsRawData(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	if p, ok := s.AsUDT(); ok {
		return p.LLIDDestination, p.LLIDSource, true
	}
	return 0, 0, false
}

// SetAddresses sets the destination and source LLIDs of the header. It
// returns ErrNoAddresses for a proprietary header and a header of an unknown
// format, and a *dmrgo.FieldRangeError when an LLID does not fit in 24 bits,
// and then changes nothing.
func (s *DataHeader) SetAddresses(destination, source uint32) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	if err := checkAddresses(destination, source, errDataHeaderTargetRange, errDataHeaderSourceRange); err != nil {
		return err
	}
	if p, ok := s.AsUnconfirmed(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetUnconfirmed(p)
	}
	if p, ok := s.AsConfirmed(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetConfirmed(p)
	}
	if p, ok := s.AsResponse(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetResponse(p)
	}
	if p, ok := s.AsDefinedData(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetDefinedData(p)
	}
	if p, ok := s.AsStatusPrecoded(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetStatusPrecoded(p)
	}
	if p, ok := s.AsRawData(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetRawData(p)
	}
	if p, ok := s.AsUDT(); ok {
		p.LLIDDestination, p.LLIDSource = destination, source
		return s.SetUDT(p)
	}
	return ErrNoAddresses
}

// BlocksToFollow returns the number of blocks after the header: the Blocks
// to Follow of a U_HEAD, C_HEAD or C_RHEAD, the Appended Blocks of a DD_HEAD
// or R_HEAD, UAB plus one for a UDT_HEAD, and 0 for an SP_HEAD. ok is false
// for a P_HEAD and the reserved formats.
func (s *DataHeader) BlocksToFollow() (n int, ok bool) {
	if s == nil {
		return 0, false
	}
	if u, ok := s.AsUnconfirmed(); ok {
		return int(u.BlocksToFollow), true
	}
	if c, ok := s.AsConfirmed(); ok {
		return int(c.BlocksToFollow), true
	}
	if r, ok := s.AsResponse(); ok {
		return int(r.BlocksToFollow), true
	}
	if d, ok := s.AsDefinedData(); ok {
		return int(d.AppendedBlocks), true
	}
	if r, ok := s.AsRawData(); ok {
		return int(r.AppendedBlocks), true
	}
	if _, ok := s.AsStatusPrecoded(); ok {
		return 0, true
	}
	if u, ok := s.AsUDT(); ok {
		return int(u.UAB) + 1, true
	}
	return 0, false
}

// SAP returns the SAP identifier of the header. ok is false for a P_HEAD,
// whose SAP field has another place, and the reserved formats.
func (s *DataHeader) SAP() (sap enums.SAP, ok bool) {
	if s == nil {
		return 0, false
	}
	if u, ok := s.AsUnconfirmed(); ok {
		return u.SAP, true
	}
	if c, ok := s.AsConfirmed(); ok {
		return c.SAP, true
	}
	if r, ok := s.AsResponse(); ok {
		return r.SAP, true
	}
	if d, ok := s.AsDefinedData(); ok {
		return d.SAP, true
	}
	if r, ok := s.AsRawData(); ok {
		return r.SAP, true
	}
	if p, ok := s.AsStatusPrecoded(); ok {
		return p.SAP, true
	}
	if u, ok := s.AsUDT(); ok {
		return u.SAP, true
	}
	return 0, false
}

// FullMessage returns the Full Message Flag (F) of a U_HEAD, C_HEAD, DD_HEAD
// or R_HEAD: true for the first try of a packet. ok is false for the other
// formats. (ETSI TS 102 361-1 clause 9.3.20)
func (s *DataHeader) FullMessage() (full, ok bool) {
	if s == nil {
		return false, false
	}
	if u, ok := s.AsUnconfirmed(); ok {
		return u.FullMessageFlag, true
	}
	if c, ok := s.AsConfirmed(); ok {
		return c.FullMessageFlag, true
	}
	if d, ok := s.AsDefinedData(); ok {
		return d.FullMessageFlag, true
	}
	if r, ok := s.AsRawData(); ok {
		return r.FullMessageFlag, true
	}
	return false, false
}

// Format is the 4-bit Data Packet Format (DPF) of a data header (ETSI TS 102
// 361-1 clause 9.3.17).
type Format uint8

// Format values (ETSI TS 102 361-1 Table 9.30).
const (
	FormatUnifiedDataTransport         Format = 0b00000000 // UDT_HEAD
	FormatResponsePacket               Format = 0b00000001 // C_RHEAD
	FormatUnconfirmed                  Format = 0b00000010 // U_HEAD
	FormatConfirmed                    Format = 0b00000011 // C_HEAD
	FormatShortDataDefined             Format = 0b00001101 // DD_HEAD
	FormatShortDataRawOrStatusPrecoded Format = 0b00001110 // R_HEAD or SP_HEAD
	FormatProprietary                  Format = 0b00001111 // P_HEAD
)

// DataType returns enums.DataTypeDataHeader.
func (s *DataHeader) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeDataHeader
}

// String returns a human-readable name for a DataHeader Format.
func (f Format) String() string {
	switch f {
	case FormatUnifiedDataTransport:
		return "Unified Data Transport"
	case FormatResponsePacket:
		return "Response"
	case FormatUnconfirmed:
		return "Unconfirmed"
	case FormatConfirmed:
		return "Confirmed"
	case FormatShortDataDefined:
		return "Defined Short"
	case FormatShortDataRawOrStatusPrecoded:
		return "Raw/StatusPrecoded Short"
	case FormatProprietary:
		return "Proprietary"
	default:
		return "Unknown"
	}
}

// ConfirmedDataHeader is the confirmed data header (C_HEAD) (ETSI TS 102 361-1
// clause 9.2.1, Table 9.10).
//
//dmr:const 4-7=FormatConfirmed
type ConfirmedDataHeader struct {
	Group                  bool      `dmr:"bit:0"`        // true when LLIDDestination is a group
	ResponseRequested      bool      `dmr:"bit:1"`        // Response Requested (A)
	Reserved               bool      `dmr:"bit:2"`        // reserved
	PadOctetCount          uint8     `dmr:"bits:3+12-15"` // Pad Octet Count (POC); bit 3 is the MSB
	SAP                    enums.SAP `dmr:"bits:8-11"`    // SAP identifier
	LLIDDestination        uint32    `dmr:"bits:16-39"`   // destination LLID
	LLIDSource             uint32    `dmr:"bits:40-63"`   // source LLID
	FullMessageFlag        bool      `dmr:"bit:64"`       // Full Message Flag (F)
	BlocksToFollow         uint8     `dmr:"bits:65-71"`   // Blocks to Follow (BF)
	ReSynchronizeFlag      bool      `dmr:"bit:72"`       // Re-Synchronize Flag (S)
	SendSequenceNumber     uint8     `dmr:"bits:73-75"`   // send sequence number N(S)
	FragmentSequenceNumber uint8     `dmr:"bits:76-79"`   // Fragment Sequence Number (FSN)
}

// ResponsePacketHeader is the response packet header (C_RHEAD) (ETSI TS 102
// 361-1 clause 9.2.4, Table 9.13).
//
//dmr:const 4-7=FormatResponsePacket
type ResponsePacketHeader struct {
	Reserved          bool      `dmr:"bit:0"`      // reserved
	ResponseRequested bool      `dmr:"bit:1"`      // Response Requested (A)
	Reserved2         uint8     `dmr:"bits:2-3"`   // reserved
	SAP               enums.SAP `dmr:"bits:8-11"`  // SAP identifier
	Reserved3         uint8     `dmr:"bits:12-15"` // reserved
	LLIDDestination   uint32    `dmr:"bits:16-39"` // destination LLID
	LLIDSource        uint32    `dmr:"bits:40-63"` // source LLID
	FullMessageFlag   bool      `dmr:"bit:64"`     // Full Message Flag (F)
	BlocksToFollow    uint8     `dmr:"bits:65-71"` // Blocks to Follow (BF)
	ResponseClass     uint8     `dmr:"bits:72-73"` // response Class (Table 8.2)
	ResponseType      uint8     `dmr:"bits:74-76"` // response Type (Table 8.2)
	ResponseStatus    uint8     `dmr:"bits:77-79"` // response Status (Table 8.2)
}

// DefinedDataHeader is the defined data header (DD_HEAD) (ETSI TS 102 361-1
// clause 9.2.12, Table 9.17C).
//
//dmr:const 4-7=FormatShortDataDefined
type DefinedDataHeader struct {
	Group             bool           `dmr:"bit:0"`           // true when LLIDDestination is a group
	ResponseRequested bool           `dmr:"bit:1"`           // Response Requested (A)
	AppendedBlocks    uint8          `dmr:"bits:2-3+12-15"`  // Appended Blocks (AB); bits 2 and 3 are the MSBs
	SAP               enums.SAP      `dmr:"bits:8-11"`       // SAP identifier
	LLIDDestination   uint32         `dmr:"bits:16-39"`      // destination LLID
	LLIDSource        uint32         `dmr:"bits:40-63"`      // source LLID
	DefinedData       enums.DDFormat `dmr:"bits:64-69,enum"` // Defined Data format (DD)
	SARQ              bool           `dmr:"bit:70"`          // Selective Automatic Repeat reQuest (SARQ)
	FullMessageFlag   bool           `dmr:"bit:71"`          // Full Message Flag (F)
	BitPadding        uint8          `dmr:"bits:72-79"`      // Bit Padding field
}

// RawDataHeader is the raw data header (R_HEAD), used when AppendedBlocks is
// not 0 (ETSI TS 102 361-1 clause 9.2.11, Table 9.17B).
//
//dmr:const 4-7=FormatShortDataRawOrStatusPrecoded
type RawDataHeader struct {
	Group             bool      `dmr:"bit:0"`          // true when LLIDDestination is a group
	ResponseRequested bool      `dmr:"bit:1"`          // Response Requested (A)
	AppendedBlocks    uint8     `dmr:"bits:2-3+12-15"` // Appended Blocks (AB); bits 2 and 3 are the MSBs
	SAP               enums.SAP `dmr:"bits:8-11"`      // SAP identifier
	LLIDDestination   uint32    `dmr:"bits:16-39"`     // destination LLID
	LLIDSource        uint32    `dmr:"bits:40-63"`     // source LLID
	SourcePort        uint8     `dmr:"bits:64-66"`     // Source Port (SP)
	DestinationPort   uint8     `dmr:"bits:67-69"`     // Destination Port (DP)
	SARQ              bool      `dmr:"bit:70"`         // Selective Automatic Repeat reQuest (SARQ)
	FullMessageFlag   bool      `dmr:"bit:71"`         // Full Message Flag (F)
	BitPadding        uint8     `dmr:"bits:72-79"`     // Bit Padding field
}

// StatusPrecodedHeader is the status or precoded data header (SP_HEAD), which
// has no appended blocks (ETSI TS 102 361-1 clause 9.2.10, Table 9.17A).
//
//dmr:const 2-3+12-15=0
//dmr:const 4-7=FormatShortDataRawOrStatusPrecoded
type StatusPrecodedHeader struct {
	Group             bool      `dmr:"bit:0"`      // true when LLIDDestination is a group
	ResponseRequested bool      `dmr:"bit:1"`      // Response Requested (A)
	SAP               enums.SAP `dmr:"bits:8-11"`  // SAP identifier
	LLIDDestination   uint32    `dmr:"bits:16-39"` // destination LLID
	LLIDSource        uint32    `dmr:"bits:40-63"` // source LLID
	SourcePort        uint8     `dmr:"bits:64-66"` // Source Port (SP)
	DestinationPort   uint8     `dmr:"bits:67-69"` // Destination Port (DP)
	StatusPrecoded    uint16    `dmr:"bits:70-79"` // 10-bit Status/Precoded (S_P) value
}

// UDTOpcode is the 6-bit UDT Opcode (UDTO) of a UDT header
// (ETSI TS 102 361-4 Table B.1, "UDT Header" group).
type UDTOpcode uint8

// UDTOpcode values, with the alias of each opcode (ETSI TS 102 361-4 Table
// B.1).
const (
	UDTOpcodeOutboundHeader     UDTOpcode = 0b011010 // C_UDTHD
	UDTOpcodeInboundHeader      UDTOpcode = 0b011011 // C_UDTHU
	UDTOpcodeDGNAOutboundHeader UDTOpcode = 0b100100 // C_DGNAHD
	UDTOpcodeDGNAInboundHeader  UDTOpcode = 0b100101 // C_DGNAHU
)

// String returns the alias of a UDT opcode.
func (op UDTOpcode) String() string {
	switch op {
	case UDTOpcodeOutboundHeader:
		return "C_UDTHD"
	case UDTOpcodeInboundHeader:
		return "C_UDTHU"
	case UDTOpcodeDGNAOutboundHeader:
		return "C_DGNAHD"
	case UDTOpcodeDGNAInboundHeader:
		return "C_DGNAHU"
	default:
		return "Unknown"
	}
}

// UDTHeader is the UDT data header (UDT_HEAD). Octet 0 bits 5 to 4 are the
// C_UDTHD and C_UDTHU fields. (ETSI TS 102 361-1 clause 9.2.13, Table 9.17D,
// ETSI TS 102 361-4 Tables 7.24 and 7.28)
//
//dmr:const 4-7=FormatUnifiedDataTransport
type UDTHeader struct {
	Group             bool            `dmr:"bit:0"`      // true when LLIDDestination is a group
	ResponseRequested bool            `dmr:"bit:1"`      // Response Requested (A)
	Emergency         bool            `dmr:"bit:2"`      // emergency in C_UDTHD, reserved in C_UDTHU
	OptionFlag        bool            `dmr:"bit:3"`      // UDT_Option_Flag in C_UDTHD, UDT_DIV in C_UDTHU
	SAP               enums.SAP       `dmr:"bits:8-11"`  // SAP identifier
	UDTFormat         enums.UDTFormat `dmr:"bits:12-15"` // UDT Format
	LLIDDestination   uint32          `dmr:"bits:16-39"` // destination LLID
	LLIDSource        uint32          `dmr:"bits:40-63"` // source LLID
	PadNibble         uint8           `dmr:"bits:64-68"` // number of pad nibbles appended to the user data
	Reserved          bool            `dmr:"bit:69"`     // reserved
	UAB               uint8           `dmr:"bits:70-71"` // UDT Appended Blocks (UAB): the number of appended blocks minus one
	SupplementaryFlag bool            `dmr:"bit:72"`     // Supplementary Flag (SF): true for supplementary data, false for short data
	ProtectFlag       bool            `dmr:"bit:73"`     // Protect Flag (PF)
	Opcode            UDTOpcode       `dmr:"bits:74-79"` // UDT Opcode (UDTO)
}

// ProprietaryHeader is the proprietary data header (P_HEAD) (ETSI TS 102 361-1
// clause 9.2.9, Table 9.17).
//
//dmr:const 4-7=FormatProprietary
type ProprietaryHeader struct {
	SAP              enums.SAP `dmr:"bits:0-3"`          // SAP identifier
	MFID             enums.FID `dmr:"bits:8-15"`         // manufacturer FID
	ManufacturerData [8]byte   `dmr:"bits:16-79,packed"` // manufacturer data, header octets 2 to 9
}
