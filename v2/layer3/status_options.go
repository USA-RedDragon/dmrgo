package layer3

// StatusServiceOptions is the 7-bit Service_Options of a Tier III status
// transport request, with Field and an Of function as for
// VoiceServiceOptions. The 2 least significant bits of the status go in
// other fields of the PDU (ETSI TS 102 361-4 clause 7.2.13.6, Table 7.57).
//
//dmr:field ServiceOptions of a C_RAND
type StatusServiceOptions struct {
	Group      bool  `dmr:"bit:0"`    // the target address is a talkgroup
	IsSuppData bool  `dmr:"bit:1"`    // supplementary_user data is asked for the call
	StatusHigh uint8 `dmr:"bits:2-6"` // the 5 most significant bits of the 7-bit status
}
