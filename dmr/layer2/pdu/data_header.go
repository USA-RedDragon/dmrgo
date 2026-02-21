package pdu

import (
	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.8 Unconfirmed Data Header
type UnconfirmedDataHeader struct {
	Group             bool `dmr:"bit:0"`
	ResponseRequested bool `dmr:"bit:1"`
	Reserved          bool `dmr:"bit:2"`
	// 4th bit is MSB, 12-15th bits are LSBs
	PadOctetCount          uint8       `dmr:"bits:3+12-15"`
	LLIDDestination        [24]bit.Bit `dmr:"bits:16-39,raw"`
	LLIDSource             [24]bit.Bit `dmr:"bits:40-63,raw"`
	FullMessage            bool        `dmr:"bit:64"`
	BlocksToFollow         uint8       `dmr:"bits:65-71"`
	Reserved2              [4]bit.Bit  `dmr:"bits:72-75,raw"`
	FragmentSequenceNumber uint8       `dmr:"bits:76-79"`
}

// dmr:crc crc_ccitt
// dmr:input_size 96
// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.8 Data Header PDU
type DataHeader struct {
	DataType elements.DataType `dmr:"-"`
	crc      uint16            `dmr:"-"` //nolint:unused
	FEC      fec.FECResult     `dmr:"-"`
	Format   Format            `dmr:"bits:4-7"`

	UnconfirmedDataHeader *UnconfirmedDataHeader `dmr:"bits:0-79,dispatch:Format=FormatUnconfirmed"`
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
