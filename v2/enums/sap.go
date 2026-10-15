package enums

import "fmt"

// ServiceAccessPointID is the Service Access Point (SAP) identifier of a
// data header (ETSI TS 102 361-1 9.3.18, Table 9.31).
type ServiceAccessPointID uint8

const (
	SAPUnifiedDataTransport      ServiceAccessPointID = 0b0000
	SAPTCPIPHeaderCompression    ServiceAccessPointID = 0b0010
	SAPUDPIPHeaderCompression    ServiceAccessPointID = 0b0011
	SAPIPBasedPacketData         ServiceAccessPointID = 0b0100
	SAPAddressResolutionProtocol ServiceAccessPointID = 0b0101
	SAPProprietaryPacketData     ServiceAccessPointID = 0b1001
	SAPShortData                 ServiceAccessPointID = 0b1010
)

// String returns the name of the SAP.
func (s ServiceAccessPointID) String() string {
	switch s {
	case SAPUnifiedDataTransport:
		return "UDT"
	case SAPTCPIPHeaderCompression:
		return "TCP/IP header compression"
	case SAPUDPIPHeaderCompression:
		return "UDP/IP header compression"
	case SAPIPBasedPacketData:
		return "IP based Packet data"
	case SAPAddressResolutionProtocol:
		return "Address Resolution Protocol (ARP)"
	case SAPProprietaryPacketData:
		return "Proprietary Packet data"
	case SAPShortData:
		return "Short Data"
	}
	return fmt.Sprintf("Reserved SAP(%d)", uint8(s))
}
