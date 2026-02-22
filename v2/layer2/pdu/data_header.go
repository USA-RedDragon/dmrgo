package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-1 - 9.1.8 Unconfirmed Data Header
type UnconfirmedDataHeader struct {
	Group             bool `dmr:"bit:0"`
	ResponseRequested bool `dmr:"bit:1"`
	Reserved          bool `dmr:"bit:2"`
	// 4th bit is MSB, 12-15th bits are LSBs
	PadOctetCount          uint8       `dmr:"bits:3+12-15"`
	SAP                    uint8       `dmr:"bits:8-11"`
	LLIDDestination        [24]bit.Bit `dmr:"bits:16-39,raw"`
	LLIDSource             [24]bit.Bit `dmr:"bits:40-63,raw"`
	FullMessage            bool        `dmr:"bit:64"`
	BlocksToFollow         uint8       `dmr:"bits:65-71"`
	Reserved2              [4]bit.Bit  `dmr:"bits:72-75,raw"`
	FragmentSequenceNumber uint8       `dmr:"bits:76-79"`
}

// dmr:crc crc_ccitt
// dmr:crc_mask 0xCCCC
// dmr:input_size 96
// ETSI TS 102 361-1 - 9.1.8 Data Header PDU
type DataHeader struct {
	DataType       elements.DataType `dmr:"-"`
	crc            uint16            `dmr:"-"` //nolint:unused
	FEC            fec.FECResult     `dmr:"-"`
	Format         Format            `dmr:"bits:4-7"`
	AppendedBlocks uint8             `dmr:"bits:2-3+12-15,no_encode"`

	UnconfirmedDataHeader *UnconfirmedDataHeader `dmr:"bits:0-79,dispatch:Format=FormatUnconfirmed"`
	ConfirmedDataHeader   *ConfirmedDataHeader   `dmr:"bits:0-79,dispatch:Format=FormatConfirmed"`
	ResponsePacketHeader  *ResponsePacketHeader  `dmr:"bits:0-79,dispatch:Format=FormatResponsePacket"`
	DefinedDataHeader     *DefinedDataHeader     `dmr:"bits:0-79,dispatch:Format=FormatShortDataDefined"`
	StatusPrecodedHeader  *StatusPrecodedHeader  `dmr:"bits:0-79,dispatch:Format=FormatShortDataRawOrStatusPrecoded,when:AppendedBlocks==0"`
	RawDataHeader         *RawDataHeader         `dmr:"bits:0-79,dispatch:Format=FormatShortDataRawOrStatusPrecoded"`
}

type ServiceAccessPointID uint8

const (
	ServiceAccessPointIDUnifiedDataTransport      ServiceAccessPointID = 0b00000000
	ServiceAccessPointIDTCPIPHeaderCompression    ServiceAccessPointID = 0b00000010
	ServiceAccessPointIDUDPIPHeaderCompression    ServiceAccessPointID = 0b00000011
	ServiceAccessPointIDIPBasedPacketData         ServiceAccessPointID = 0b00000100
	ServiceAccessPointIDAddressResolutionProtocol ServiceAccessPointID = 0b00000101
	ServiceAccessPointIDProprietaryPacketData     ServiceAccessPointID = 0b00001001
	ServiceAccessPointIDShortData                 ServiceAccessPointID = 0b00001010
)

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

func (dh *DataHeader) GetDataType() elements.DataType {
	return dh.DataType
}

// FormatToName returns a human-readable name for a DataHeader Format.
func FormatToName(f Format) string {
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
type ConfirmedDataHeader struct {
	Group                  bool  `dmr:"bit:0"`
	ResponseRequested      bool  `dmr:"bit:1"`
	PadOctetCount          uint8 `dmr:"bits:3+12-15"`
	SAP                    uint8 `dmr:"bits:8-11"`
	LLIDDestination        int   `dmr:"bits:16-39"`
	LLIDSource             int   `dmr:"bits:40-63"`
	FullMessageFlag        bool  `dmr:"bit:64"`
	BlocksToFollow         uint8 `dmr:"bits:65-71"`
	ReSynchronizeFlag      bool  `dmr:"bit:72"`
	SendSequenceNumber     uint8 `dmr:"bits:73-75"`
	FragmentSequenceNumber uint8 `dmr:"bits:76-79"`
}

// ETSI TS 102 361-1 - Table 9.13: Response Packet Header (C_RHEAD) PDU content
type ResponsePacketHeader struct {
	SAP             uint8 `dmr:"bits:8-11"`
	LLIDDestination int   `dmr:"bits:16-39"`
	LLIDSource      int   `dmr:"bits:40-63"`
	BlocksToFollow  uint8 `dmr:"bits:65-71"`
	ResponseClass   uint8 `dmr:"bits:72-73"`
	ResponseType    uint8 `dmr:"bits:74-76"`
	ResponseStatus  uint8 `dmr:"bits:77-79"`
}

// ETSI TS 102 361-1 - Table 9.17C: Defined Data Header (DD_HEAD) PDU content
type DefinedDataHeader struct {
	Group             bool  `dmr:"bit:0"`
	ResponseRequested bool  `dmr:"bit:1"`
	AppendedBlocks    uint8 `dmr:"bits:2-3+12-15"`
	SAP               uint8 `dmr:"bits:8-11"`
	LLIDDestination   int   `dmr:"bits:16-39"`
	LLIDSource        int   `dmr:"bits:40-63"`
	DefinedData       uint8 `dmr:"bits:64-69"`
	SARQ              bool  `dmr:"bit:70"`
	FullMessageFlag   bool  `dmr:"bit:71"`
	BitPadding        uint8 `dmr:"bits:72-79"`
}

// ETSI TS 102 361-1 - Table 9.17B: Raw Data Header (R_HEAD) PDU content
// Used when AppendedBlocks != 0 (short data with appended data blocks).
type RawDataHeader struct {
	Group             bool  `dmr:"bit:0"`
	ResponseRequested bool  `dmr:"bit:1"`
	AppendedBlocks    uint8 `dmr:"bits:2-3+12-15"`
	SAP               uint8 `dmr:"bits:8-11"`
	LLIDDestination   int   `dmr:"bits:16-39"`
	LLIDSource        int   `dmr:"bits:40-63"`
	SourcePort        uint8 `dmr:"bits:64-66"`
	DestinationPort   uint8 `dmr:"bits:67-69"`
	SARQ              bool  `dmr:"bit:70"`
	FullMessageFlag   bool  `dmr:"bit:71"`
	BitPadding        uint8 `dmr:"bits:72-79"`
}

// ETSI TS 102 361-1 - Table 9.17A: Status/Precoded Data Header (SP_HEAD) PDU content
// Used when AppendedBlocks == 0 (status/precoded message carried entirely in header).
type StatusPrecodedHeader struct {
	Group             bool   `dmr:"bit:0"`
	ResponseRequested bool   `dmr:"bit:1"`
	SAP               uint8  `dmr:"bits:8-11"`
	LLIDDestination   int    `dmr:"bits:16-39"`
	LLIDSource        int    `dmr:"bits:40-63"`
	SourcePort        uint8  `dmr:"bits:64-66"`
	DestinationPort   uint8  `dmr:"bits:67-69"`
	StatusPrecoded    uint16 `dmr:"bits:70-79"`
}
