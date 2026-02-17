package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type UnconfirmedDataHeader struct {
	Group             bool
	ResponseRequested bool
	Reserved          bool
	// 4th bit is MSB, 12-15th bits are LSBs
	PadOctetCount          uint8
	LLIDDestination        [24]bit.Bit
	LLIDSource             [24]bit.Bit
	FullMessage            bool
	BlocksToFollow         uint8
	Reserved2              [4]bit.Bit
	FragmentSequenceNumber uint8
}

func (cdh *UnconfirmedDataHeader) ToString() string {
	return fmt.Sprintf("UnconfirmedDataHeader{ Group: %t, ResponseRequested: %t, Reserved: %t, PadOctetCount: %d, LLIDDestination: %08b, LLIDSource: %08b, FullMessage: %t, BlocksToFollow: %d, FragmentSequenceNumber: %d }",
		cdh.Group, cdh.ResponseRequested, cdh.Reserved, cdh.PadOctetCount, cdh.LLIDDestination, cdh.LLIDSource, cdh.FullMessage, cdh.BlocksToFollow, cdh.FragmentSequenceNumber)
}

func (cdh *UnconfirmedDataHeader) DecodeFromBits(infoBits []bit.Bit) bool {
	if len(infoBits) != 96 {
		fmt.Println("UnconfirmedDataHeader: invalid infoBits length: ", len(infoBits))
		return false
	}

	cdh.Group = infoBits[0] == 1
	cdh.ResponseRequested = infoBits[1] == 1
	cdh.Reserved = infoBits[2] == 1
	cdh.PadOctetCount = byte(infoBits[3])
	cdh.PadOctetCount <<= 4
	cdh.PadOctetCount |= (byte(infoBits[12]) << 3) | (byte(infoBits[13]) << 2) | (byte(infoBits[14]) << 1) | byte(infoBits[15])

	for i := range 24 {
		cdh.LLIDDestination[i] = infoBits[16+i]
	}

	for i := range 24 {
		cdh.LLIDSource[i] = infoBits[40+i]
	}

	cdh.FullMessage = infoBits[64] == 1
	cdh.BlocksToFollow = (byte(infoBits[65]) << 6) | (byte(infoBits[66]) << 5) | (byte(infoBits[67]) << 4) | (byte(infoBits[68]) << 3) | (byte(infoBits[69]) << 2) | (byte(infoBits[70]) << 1) | byte(infoBits[71])
	for i := range 4 {
		cdh.Reserved2[i] = infoBits[72+i]
	}
	cdh.FragmentSequenceNumber = (byte(infoBits[76]) << 3) | (byte(infoBits[77]) << 2) | (byte(infoBits[78]) << 1) | byte(infoBits[79])

	return true
}

type DataHeader struct {
	dataType       elements.DataType
	dataHeaderType DataHeaderType

	crc    uint16
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
	return fmt.Sprintf("DataHeader{ dataType: %s, dataHeaderType: %s, extraData: %s }", elements.DataTypeToName(dh.dataType), dh.dataHeaderType.ToString(), extraData)
}

func (dh *DataHeader) DecodeFromBits(infoBits []bit.Bit, dt elements.DataType) bool {
	dh.dataType = dt

	if len(infoBits) != 96 {
		fmt.Println("DataHeader: invalid infoBits length: ", len(infoBits))
		return false
	}

	// TODO: CRC

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
		dh.UnconfirmedDataHeader = &UnconfirmedDataHeader{}
		if !dh.UnconfirmedDataHeader.DecodeFromBits(infoBits) {
			return false
		}
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
