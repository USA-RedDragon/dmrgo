package layer3

// ServiceOptions is the Service Options information element (ETSI TS 102 361-2
// clause 7.2.1).
type ServiceOptions struct {
	IsEmergency         bool  `dmr:"bit:0"`    // emergency service
	IsPrivacy           bool  `dmr:"bit:1"`    // privacy, which ETSI TS 102 361-2 does not define
	Reserved            uint8 `dmr:"bits:2-3"` // reserved bits
	IsBroadcast         bool  `dmr:"bit:4"`    // broadcast service, for group calls only
	IsOpenVoiceCallMode bool  `dmr:"bit:5"`    // Open Voice Call Mode (OVCM) call
	PriorityLevel       uint8 `dmr:"bits:6-7"` // 0 is no priority, 1 to 3 are priorities 1 to 3 (3 is highest)
}

// MotorolaServiceOptions returns the Service Options octet 0x20 seen in
// Motorola IPSC voice traffic: no flag set and only the first Reserved bit
// (0x20) set, so Reserved is 2. MMDVMHost sends 0x00, the zero
// ServiceOptions.
func MotorolaServiceOptions() ServiceOptions {
	return ServiceOptions{Reserved: 2}
}

// Field returns s as the 8-bit Service Options field, the octet of Encode,
// with the errors of Encode. It is the inverse of DecodeServiceOptions.
func (s ServiceOptions) Field() (uint8, error) {
	b, err := s.Encode()
	return b[0], err
}
