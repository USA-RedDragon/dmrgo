package enums

import "testing"

func TestServiceAccessPointID_String(t *testing.T) {
	for sap, want := range map[ServiceAccessPointID]string{
		SAPUnifiedDataTransport:      "UDT",
		SAPTCPIPHeaderCompression:    "TCP/IP header compression",
		SAPUDPIPHeaderCompression:    "UDP/IP header compression",
		SAPIPBasedPacketData:         "IP based Packet data",
		SAPAddressResolutionProtocol: "Address Resolution Protocol (ARP)",
		SAPProprietaryPacketData:     "Proprietary Packet data",
		SAPShortData:                 "Short Data",
		1:                            "Reserved SAP(1)",
	} {
		if got := sap.String(); got != want {
			t.Errorf("SAP %d: %q, want %q", uint8(sap), got, want)
		}
	}
}
