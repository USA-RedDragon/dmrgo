package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
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

// ServiceTypeToName returns a human-readable name for a ServiceType.
func ServiceTypeToName(st ServiceType) string {
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

// dmr:crc crc_ccitt
// dmr:crc_mask 0x3333
// dmr:input_size 96
// ETSI TS 102 361-1 - 8.2.3.1 Unified Single Block Data (USBD)
type UnifiedSingleBlockData struct {
	DataType    elements.DataType `dmr:"-"`
	crc         uint16            `dmr:"-"` //nolint:unused
	FEC         fec.FECResult     `dmr:"-"`
	ServiceType ServiceType       `dmr:"bits:0-3"`
	Payload     [76]bit.Bit       `dmr:"bits:4-79,raw"`
}

func (u *UnifiedSingleBlockData) GetDataType() elements.DataType {
	return u.DataType
}
