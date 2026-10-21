package layer3

// StatusServiceOptions is the 7-bit Service_Options of a Tier III status
// transport request, in the high bits of the octet as for
// VoiceServiceOptions. The 2 least significant bits of the status go in
// other fields of the PDU (ETSI TS 102 361-4 clause 7.2.13.6, Table 7.57).
type StatusServiceOptions struct {
	Group      bool  `dmr:"bit:0"`    // the target address is a talkgroup
	IsSuppData bool  `dmr:"bit:1"`    // supplementary_user data is asked for the call
	StatusHigh uint8 `dmr:"bits:2-6"` // the 5 most significant bits of the 7-bit status
}

// Field returns s as the 7-bit ServiceOptions field of a C_RAND, with the
// errors of Encode.
func (s StatusServiceOptions) Field() (uint8, error) {
	b, err := s.Encode()
	return b[0] >> 1, err
}
