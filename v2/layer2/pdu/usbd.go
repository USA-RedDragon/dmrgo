package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ServiceType represents the higher layer service supported by a USBD.
// ETSI TS 102 361-1 §9.3.43
type ServiceType uint8

const (
	ServiceTypeLIP                   ServiceType = 0b0000
	ServiceTypeManufacturerSpecific1 ServiceType = 0b1000
	ServiceTypeManufacturerSpecific2 ServiceType = 0b1001
	ServiceTypeManufacturerSpecific3 ServiceType = 0b1010
	ServiceTypeManufacturerSpecific4 ServiceType = 0b1011
	ServiceTypeManufacturerSpecific5 ServiceType = 0b1100
	ServiceTypeManufacturerSpecific6 ServiceType = 0b1101
	ServiceTypeManufacturerSpecific7 ServiceType = 0b1110
	ServiceTypeManufacturerSpecific8 ServiceType = 0b1111
)

// String returns a human-readable name for a ServiceType.
func (st ServiceType) String() string {
	switch st {
	case ServiceTypeLIP:
		return "Location Information Protocol"
	case ServiceTypeManufacturerSpecific1:
		return "Manufacturer Specific Service 1"
	case ServiceTypeManufacturerSpecific2:
		return "Manufacturer Specific Service 2"
	case ServiceTypeManufacturerSpecific3:
		return "Manufacturer Specific Service 3"
	case ServiceTypeManufacturerSpecific4:
		return "Manufacturer Specific Service 4"
	case ServiceTypeManufacturerSpecific5:
		return "Manufacturer Specific Service 5"
	case ServiceTypeManufacturerSpecific6:
		return "Manufacturer Specific Service 6"
	case ServiceTypeManufacturerSpecific7:
		return "Manufacturer Specific Service 7"
	case ServiceTypeManufacturerSpecific8:
		return "Manufacturer Specific Service 8"
	default:
		return fmt.Sprintf("Reserved (%04b)", uint8(st))
	}
}

// ETSI TS 102 361-1 - 8.2.3.1 Unified Single Block Data (USBD)
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0x3333
//dmr:input_size 96
type UnifiedSingleBlockData struct {
	FEC         fec.Result  `dmr:"-"`
	ServiceType ServiceType `dmr:"bits:0-3"`
	// Payload holds the 76 bits 4 to 79, packed most significant bit
	// first: bit 4 is the top bit of byte 0 and bit 79 is bit 4 of byte 9.
	// The low 4 bits of byte 9 are padding and must be zero; Encode returns
	// a *bit.FieldRangeError otherwise.
	Payload [10]byte `dmr:"bits:4-79,raw"`
}

// DataType returns enums.DataTypeUnifiedSingleBlock.
func (s *UnifiedSingleBlockData) DataType() enums.DataType {
	return enums.DataTypeUnifiedSingleBlock
}
