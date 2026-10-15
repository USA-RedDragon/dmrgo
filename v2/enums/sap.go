package enums

import "fmt"

// ServiceAccessPointID is the 4-bit Service Access Point (SAP) identifier of
// a data header. Values that have no constant are reserved (ETSI TS 102 361-1
// clause 9.3.18, Table 9.31).
type ServiceAccessPointID uint8

const (
	SAPUnifiedDataTransport      ServiceAccessPointID = 0b0000 // Unified Data Transport (UDT)
	SAPTCPIPHeaderCompression    ServiceAccessPointID = 0b0010 // TCP/IP header compression
	SAPUDPIPHeaderCompression    ServiceAccessPointID = 0b0011 // UDP/IP header compression
	SAPIPBasedPacketData         ServiceAccessPointID = 0b0100 // IP based packet data
	SAPAddressResolutionProtocol ServiceAccessPointID = 0b0101 // Address Resolution Protocol (ARP)
	SAPProprietaryPacketData     ServiceAccessPointID = 0b1001 // proprietary packet data
	SAPShortData                 ServiceAccessPointID = 0b1010 // short data
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
