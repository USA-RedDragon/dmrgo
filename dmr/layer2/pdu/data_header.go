package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/crc"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

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

func (cdh *UnconfirmedDataHeader) ToString() string {
	return fmt.Sprintf("UnconfirmedDataHeader{ Group: %t, ResponseRequested: %t, Reserved: %t, PadOctetCount: %d, LLIDDestination: %08b, LLIDSource: %08b, FullMessage: %t, BlocksToFollow: %d, FragmentSequenceNumber: %d }",
		cdh.Group, cdh.ResponseRequested, cdh.Reserved, cdh.PadOctetCount, cdh.LLIDDestination, cdh.LLIDSource, cdh.FullMessage, cdh.BlocksToFollow, cdh.FragmentSequenceNumber)
}

type DataHeader struct {
	dataType       elements.DataType
	dataHeaderType DataHeaderType

	crc    uint16
	FEC    fec.FECResult
	Format Format
	SAPID  ServiceAccessPointID

	UnconfirmedDataHeader *UnconfirmedDataHeader
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

type DataHeaderType uint8

const (
	DataHeaderTypeUnconfirmed DataHeaderType = iota
	DataHeaderTypeConfirmed
	DataHeaderTypeResponse
	DataHeaderTypeProprietary
	DataHeaderTypeStatusPrecodedShort
	DataHeaderTypeRawShort
	DataHeaderTypeDefinedShort
	DataHeaderTypeUnifiedDataTransport
)

func (dht DataHeaderType) ToString() string {
	switch dht {
	case DataHeaderTypeUnconfirmed:
		return "Unconfirmed"
	case DataHeaderTypeConfirmed:
		return "Confirmed"
	case DataHeaderTypeResponse:
		return "Response"
	case DataHeaderTypeProprietary:
		return "Proprietary"
	case DataHeaderTypeStatusPrecodedShort:
		return "Status/Precoded Short"
	case DataHeaderTypeRawShort:
		return "Raw Short"
	case DataHeaderTypeDefinedShort:
		return "Defined Short"
	case DataHeaderTypeUnifiedDataTransport:
		return "Unified Data Transport"
	default:
		return "Unknown"
	}
}

func (dh *DataHeader) GetDataType() elements.DataType {
	return dh.dataType
}

func (dh *DataHeader) ToString() string {
	var extraData string
	switch dh.dataHeaderType {
	case DataHeaderTypeUnconfirmed:
		extraData = dh.UnconfirmedDataHeader.ToString()
	default:
		extraData = "Unknown"
	}
	return fmt.Sprintf("DataHeader{ dataType: %s, dataHeaderType: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t}, extraData: %s }",
		elements.DataTypeToName(dh.dataType), dh.dataHeaderType.ToString(), dh.FEC.BitsChecked, dh.FEC.ErrorsCorrected, dh.FEC.Uncorrectable, extraData)
}

func (dh *DataHeader) DecodeFromBits(infoBits []bit.Bit, dt elements.DataType) bool {
	dh.dataType = dt

	if len(infoBits) != 96 {
		fmt.Println("DataHeader: invalid infoBits length: ", len(infoBits))
		return false
	}

	// Pack 96 info bits into 12 bytes for CRC check
	// ETSI TS 102 361-1 §9.1.8: CRC-CCITT over the full 12 bytes (no XOR mask)
	var dataBytes [12]byte
	for i := range 12 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(infoBits[i*8+j])
		}
	}

	if !crc.CheckCRCCCITT(dataBytes[:]) {
		fmt.Println("DataHeader: CRC check failed")
		dh.FEC = fec.FECResult{BitsChecked: 96, Uncorrectable: true}
		return false
	}

	dh.FEC = fec.FECResult{BitsChecked: 96}
	dh.crc = uint16(dataBytes[10])<<8 | uint16(dataBytes[11])

	dh.Format = Format((byte(infoBits[4]) << 3) | (byte(infoBits[5]) << 2) | (byte(infoBits[6]) << 1) | byte(infoBits[7]))
	switch dh.Format {
	case FormatUnifiedDataTransport:
		dh.dataHeaderType = DataHeaderTypeUnifiedDataTransport
		return false
	case FormatResponsePacket:
		dh.dataHeaderType = DataHeaderTypeResponse
		return false
	case FormatUnconfirmed:
		dh.dataHeaderType = DataHeaderTypeUnconfirmed
		var uhBits [80]bit.Bit
		copy(uhBits[:], infoBits[:80])
		decoded, _ := DecodeUnconfirmedDataHeader(uhBits)
		dh.UnconfirmedDataHeader = &decoded
	case FormatConfirmed:
		dh.dataHeaderType = DataHeaderTypeConfirmed
		return false
	case FormatShortDataDefined:
		dh.dataHeaderType = DataHeaderTypeDefinedShort
		return false
	case FormatShortDataRawOrStatusPrecoded:
		// For status/precoded short, AB (2:3, 12:16) is always zeroed out
		if (infoBits[2] == 0) && (infoBits[3] == 0) && (infoBits[12] == 0) && (infoBits[13] == 0) && (infoBits[14] == 0) && (infoBits[15] == 0) {
			dh.dataHeaderType = DataHeaderTypeStatusPrecodedShort
		} else {
			dh.dataHeaderType = DataHeaderTypeRawShort
		}
		return false
	case FormatProprietary:
		dh.dataHeaderType = DataHeaderTypeProprietary
		return false
	default:
		fmt.Println("DataHeader: unknown format: ", dh.Format)
		return false
	}

	return true
}
