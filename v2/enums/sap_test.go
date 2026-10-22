package enums

import "testing"

func TestSAP_String(t *testing.T) {
	t.Parallel()

	for sap, want := range map[SAP]string{
		SAPUnifiedDataTransport:      "UDT",
		SAPTCPIPHeaderCompression:    "TCP/IP header compression",
		SAPUDPIPHeaderCompression:    "UDP/IP header compression",
		SAPIPBasedPacketData:         "IP based Packet data",
		SAPAddressResolutionProtocol: "Address Resolution Protocol (ARP)",
		SAPProprietaryPacketData:     "Proprietary Packet data",
		SAPShortData:                 "Short Data",
		1:                            "SAP(1)",
	} {
		if got := sap.String(); got != want {
			t.Errorf("SAP %d: %q, want %q", uint8(sap), got, want)
		}
	}
}
