package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - 9.1.8 Unconfirmed Data Header
//
//dmr:const 4-7=FormatUnconfirmed
type UnconfirmedDataHeader struct {
	Group             bool `dmr:"bit:0"`
	ResponseRequested bool `dmr:"bit:1"`
	Reserved          bool `dmr:"bit:2"`
	// 4th bit is MSB, 12-15th bits are LSBs
	PadOctetCount          uint8                      `dmr:"bits:3+12-15"`
	SAP                    enums.ServiceAccessPointID `dmr:"bits:8-11"`
	LLIDDestination        uint32                     `dmr:"bits:16-39"`
	LLIDSource             uint32                     `dmr:"bits:40-63"`
	FullMessageFlag        bool                       `dmr:"bit:64"`
	BlocksToFollow         uint8                      `dmr:"bits:65-71"`
	Reserved2              uint8                      `dmr:"bits:72-75"`
	FragmentSequenceNumber uint8                      `dmr:"bits:76-79"`
}

// ETSI TS 102 361-1 - 9.1.8 Data Header PDU
//
// DataHeader.Encode returns an error if more than one header field is set, or
// if Format and the encoded Appended Blocks do not select the header field
// that is set. An R_HEAD needs a non-zero AppendedBlocks; with zero appended
// blocks, use StatusPrecodedHeader.
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0xCCCC
//dmr:input_size 96
type DataHeader struct {
	crc            uint16     `dmr:"-"` //nolint:unused
	FEC            fec.Result `dmr:"-"`
	Format         Format     `dmr:"bits:4-7"`
	AppendedBlocks uint8      `dmr:"bits:2-3+12-15,no_encode"`

	UnconfirmedDataHeader *UnconfirmedDataHeader `dmr:"bits:0-79,dispatch:Format=FormatUnconfirmed"`
	ConfirmedDataHeader   *ConfirmedDataHeader   `dmr:"bits:0-79,dispatch:Format=FormatConfirmed"`
	ResponsePacketHeader  *ResponsePacketHeader  `dmr:"bits:0-79,dispatch:Format=FormatResponsePacket"`
	DefinedDataHeader     *DefinedDataHeader     `dmr:"bits:0-79,dispatch:Format=FormatShortDataDefined"`
	StatusPrecodedHeader  *StatusPrecodedHeader  `dmr:"bits:0-79,dispatch:Format=FormatShortDataRawOrStatusPrecoded,when:AppendedBlocks==0"`
	RawDataHeader         *RawDataHeader         `dmr:"bits:0-79,dispatch:Format=FormatShortDataRawOrStatusPrecoded"`
	UDTHeader             *UDTHeader             `dmr:"bits:0-79,dispatch:Format=FormatUnifiedDataTransport"`
	ProprietaryHeader     *ProprietaryHeader     `dmr:"bits:0-79,dispatch:Format=FormatProprietary"`

	RawPayload [80]bit.Bit `dmr:"bits:0-79,raw,fallback"`
}

// Addresses returns the destination and source LLIDs of the header that is
// set. ok is false for a proprietary header and when no header is set.
func (dh *DataHeader) Addresses() (destination, source uint32, ok bool) {
	switch {
	case dh.UnconfirmedDataHeader != nil:
		return dh.UnconfirmedDataHeader.LLIDDestination, dh.UnconfirmedDataHeader.LLIDSource, true
	case dh.ConfirmedDataHeader != nil:
		return dh.ConfirmedDataHeader.LLIDDestination, dh.ConfirmedDataHeader.LLIDSource, true
	case dh.ResponsePacketHeader != nil:
		return dh.ResponsePacketHeader.LLIDDestination, dh.ResponsePacketHeader.LLIDSource, true
	case dh.DefinedDataHeader != nil:
		return dh.DefinedDataHeader.LLIDDestination, dh.DefinedDataHeader.LLIDSource, true
	case dh.StatusPrecodedHeader != nil:
		return dh.StatusPrecodedHeader.LLIDDestination, dh.StatusPrecodedHeader.LLIDSource, true
	case dh.RawDataHeader != nil:
		return dh.RawDataHeader.LLIDDestination, dh.RawDataHeader.LLIDSource, true
	case dh.UDTHeader != nil:
		return dh.UDTHeader.LLIDDestination, dh.UDTHeader.LLIDSource, true
	}
	return 0, 0, false
}

// SetAddresses sets the destination and source LLIDs of the header that is
// set. It returns false for a proprietary header and when no header is set.
func (dh *DataHeader) SetAddresses(destination, source uint32) bool {
	switch {
	case dh.UnconfirmedDataHeader != nil:
		dh.UnconfirmedDataHeader.LLIDDestination, dh.UnconfirmedDataHeader.LLIDSource = destination, source
	case dh.ConfirmedDataHeader != nil:
		dh.ConfirmedDataHeader.LLIDDestination, dh.ConfirmedDataHeader.LLIDSource = destination, source
	case dh.ResponsePacketHeader != nil:
		dh.ResponsePacketHeader.LLIDDestination, dh.ResponsePacketHeader.LLIDSource = destination, source
	case dh.DefinedDataHeader != nil:
		dh.DefinedDataHeader.LLIDDestination, dh.DefinedDataHeader.LLIDSource = destination, source
	case dh.StatusPrecodedHeader != nil:
		dh.StatusPrecodedHeader.LLIDDestination, dh.StatusPrecodedHeader.LLIDSource = destination, source
	case dh.RawDataHeader != nil:
		dh.RawDataHeader.LLIDDestination, dh.RawDataHeader.LLIDSource = destination, source
	case dh.UDTHeader != nil:
		dh.UDTHeader.LLIDDestination, dh.UDTHeader.LLIDSource = destination, source
	default:
		return false
	}
	return true
}

type Format uint8

const (
	FormatUnifiedDataTransport         Format = 0b00000000
	FormatResponsePacket               Format = 0b00000001
	FormatUnconfirmed                  Format = 0b00000010
	FormatConfirmed                    Format = 0b00000011
	FormatShortDataDefined             Format = 0b00001101
	FormatShortDataRawOrStatusPrecoded Format = 0b00001110
	FormatProprietary                  Format = 0b00001111
)

// DataType returns enums.DataTypeDataHeader.
func (dh *DataHeader) DataType() enums.DataType {
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

// ETSI TS 102 361-1 - Table 9.10: Confirmed Data Header (C_HEAD) PDU content
//
//dmr:const 4-7=FormatConfirmed
type ConfirmedDataHeader struct {
	Group                  bool                       `dmr:"bit:0"`
	ResponseRequested      bool                       `dmr:"bit:1"`
	Reserved               bool                       `dmr:"bit:2"`
	PadOctetCount          uint8                      `dmr:"bits:3+12-15"`
	SAP                    enums.ServiceAccessPointID `dmr:"bits:8-11"`
	LLIDDestination        uint32                     `dmr:"bits:16-39"`
	LLIDSource             uint32                     `dmr:"bits:40-63"`
	FullMessageFlag        bool                       `dmr:"bit:64"`
	BlocksToFollow         uint8                      `dmr:"bits:65-71"`
	ReSynchronizeFlag      bool                       `dmr:"bit:72"`
	SendSequenceNumber     uint8                      `dmr:"bits:73-75"`
	FragmentSequenceNumber uint8                      `dmr:"bits:76-79"`
}

// ETSI TS 102 361-1 - Table 9.13: Response Packet Header (C_RHEAD) PDU content
//
//dmr:const 4-7=FormatResponsePacket
type ResponsePacketHeader struct {
	Reserved          bool                       `dmr:"bit:0"`
	ResponseRequested bool                       `dmr:"bit:1"`
	Reserved2         uint8                      `dmr:"bits:2-3"`
	SAP               enums.ServiceAccessPointID `dmr:"bits:8-11"`
	Reserved3         uint8                      `dmr:"bits:12-15"`
	LLIDDestination   uint32                     `dmr:"bits:16-39"`
	LLIDSource        uint32                     `dmr:"bits:40-63"`
	FullMessageFlag   bool                       `dmr:"bit:64"`
	BlocksToFollow    uint8                      `dmr:"bits:65-71"`
	ResponseClass     uint8                      `dmr:"bits:72-73"`
	ResponseType      uint8                      `dmr:"bits:74-76"`
	ResponseStatus    uint8                      `dmr:"bits:77-79"`
}

// ETSI TS 102 361-1 - Table 9.17C: Defined Data Header (DD_HEAD) PDU content
//
//dmr:const 4-7=FormatShortDataDefined
type DefinedDataHeader struct {
	Group             bool                       `dmr:"bit:0"`
	ResponseRequested bool                       `dmr:"bit:1"`
	AppendedBlocks    uint8                      `dmr:"bits:2-3+12-15"`
	SAP               enums.ServiceAccessPointID `dmr:"bits:8-11"`
	LLIDDestination   uint32                     `dmr:"bits:16-39"`
	LLIDSource        uint32                     `dmr:"bits:40-63"`
	DefinedData       uint8                      `dmr:"bits:64-69"`
	SARQ              bool                       `dmr:"bit:70"`
	FullMessageFlag   bool                       `dmr:"bit:71"`
	BitPadding        uint8                      `dmr:"bits:72-79"`
}

// ETSI TS 102 361-1 - Table 9.17B: Raw Data Header (R_HEAD) PDU content
// Used when AppendedBlocks != 0 (short data with appended data blocks).
//
//dmr:const 4-7=FormatShortDataRawOrStatusPrecoded
type RawDataHeader struct {
	Group             bool                       `dmr:"bit:0"`
	ResponseRequested bool                       `dmr:"bit:1"`
	AppendedBlocks    uint8                      `dmr:"bits:2-3+12-15"`
	SAP               enums.ServiceAccessPointID `dmr:"bits:8-11"`
	LLIDDestination   uint32                     `dmr:"bits:16-39"`
	LLIDSource        uint32                     `dmr:"bits:40-63"`
	SourcePort        uint8                      `dmr:"bits:64-66"`
	DestinationPort   uint8                      `dmr:"bits:67-69"`
	SARQ              bool                       `dmr:"bit:70"`
	FullMessageFlag   bool                       `dmr:"bit:71"`
	BitPadding        uint8                      `dmr:"bits:72-79"`
}

// ETSI TS 102 361-1 - Table 9.17A: Status/Precoded Data Header (SP_HEAD) PDU content
// Used when AppendedBlocks == 0 (status/precoded message carried entirely in header).
//
//dmr:const 2-3+12-15=0
//dmr:const 4-7=FormatShortDataRawOrStatusPrecoded
type StatusPrecodedHeader struct {
	Group             bool                       `dmr:"bit:0"`
	ResponseRequested bool                       `dmr:"bit:1"`
	SAP               enums.ServiceAccessPointID `dmr:"bits:8-11"`
	LLIDDestination   uint32                     `dmr:"bits:16-39"`
	LLIDSource        uint32                     `dmr:"bits:40-63"`
	SourcePort        uint8                      `dmr:"bits:64-66"`
	DestinationPort   uint8                      `dmr:"bits:67-69"`
	StatusPrecoded    uint16                     `dmr:"bits:70-79"`
}

// UDTOpcode is the 6-bit UDT Opcode (UDTO) of a UDT header
// (ETSI TS 102 361-4 Table B.1, "UDT Header" group).
type UDTOpcode uint8

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

// UDTHeader takes octet 0 bits 5-4 from TS 102 361-4 Tables 7.24 (C_UDTHD)
// and 7.28 (C_UDTHU).
// ETSI TS 102 361-1 - Table 9.17D: UDT Header (UDT_HEAD) PDU content
//
//dmr:const 4-7=FormatUnifiedDataTransport
type UDTHeader struct {
	Group             bool `dmr:"bit:0"`
	ResponseRequested bool `dmr:"bit:1"`
	// Emergency in C_UDTHD; reserved in C_UDTHU.
	Emergency bool `dmr:"bit:2"`
	// UDT_Option_Flag in C_UDTHD; UDT_DIV in C_UDTHU.
	OptionFlag      bool                       `dmr:"bit:3"`
	SAP             enums.ServiceAccessPointID `dmr:"bits:8-11"`
	UDTFormat       uint8                      `dmr:"bits:12-15"`
	LLIDDestination uint32                     `dmr:"bits:16-39"`
	LLIDSource      uint32                     `dmr:"bits:40-63"`
	PadNibble       uint8                      `dmr:"bits:64-68"`
	Reserved        bool                       `dmr:"bit:69"`
	// AppendedBlocks (UAB) is the number of appended blocks minus one.
	AppendedBlocks    uint8     `dmr:"bits:70-71"`
	SupplementaryFlag bool      `dmr:"bit:72"`
	ProtectFlag       bool      `dmr:"bit:73"`
	Opcode            UDTOpcode `dmr:"bits:74-79"`
}

// ETSI TS 102 361-1 - Table 9.17: Proprietary Header (P_HEAD) PDU content
//
//dmr:const 4-7=FormatProprietary
type ProprietaryHeader struct {
	SAP              enums.ServiceAccessPointID `dmr:"bits:0-3"`
	MFID             enums.FeatureSetID         `dmr:"bits:8-15"`
	ManufacturerData [8]byte                    `dmr:"bits:16-79,packed"`
}
