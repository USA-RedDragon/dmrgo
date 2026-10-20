package layer3

// VoiceServiceOptions is the 7-bit Service_Options of a Tier III voice
// service request, such as a C_RAND for a talkgroup or individual voice
// call. Its layout differs from the 8-bit ServiceOptions of Tier II (ETSI
// TS 102 361-4 clause 7.2.13.1, Table 7.52). The 7 bits are the high bits
// of the octet: shift the ServiceOptions of a C_RAND left by 1 for
// DecodeVoiceServiceOptions.
type VoiceServiceOptions struct {
	IsEmergency   bool  `dmr:"bit:0"`    // emergency service
	IsPrivacy     bool  `dmr:"bit:1"`    // privacy, which ETSI TS 102 361-4 does not define
	IsSuppData    bool  `dmr:"bit:2"`    // a Supplementary Data Transfer Service is asked for the call
	IsBroadcast   bool  `dmr:"bit:3"`    // broadcast service, for talkgroups only
	Reserved      bool  `dmr:"bit:4"`    // reserved, 0
	PriorityLevel uint8 `dmr:"bits:5-6"` // 0 is normal, 1 to 3 are priorities 1 to 3 (3 is highest)
}

// RegistrationServiceOptions is the 7-bit Service_Options of a Tier III
// registration request, in the high bits of the octet as for
// VoiceServiceOptions (ETSI TS 102 361-4 clause 7.2.13.4, Table 7.55).
type RegistrationServiceOptions struct {
	Reserved  bool  `dmr:"bit:0"`    // reserved, 0
	IsPrivacy bool  `dmr:"bit:1"`    // privacy, which ETSI TS 102 361-4 does not define
	IPInform  bool  `dmr:"bit:2"`    // the MS advises an IP connection
	PowerSave uint8 `dmr:"bits:3-5"` // PowerSave_RQ: 0 asks for no power save
	Register  bool  `dmr:"bit:6"`    // Reg_Dereg: true to register, false to de-register
}

// Field returns s as the 7-bit ServiceOptions field of a C_RAND, with the
// errors of Encode.
func (s VoiceServiceOptions) Field() (uint8, error) {
	b, err := s.Encode()
	return b[0] >> 1, err
}

// Field returns s as the 7-bit ServiceOptions field of a C_RAND, with the
// errors of Encode.
func (s RegistrationServiceOptions) Field() (uint8, error) {
	b, err := s.Encode()
	return b[0] >> 1, err
}
