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

// Byte returns s as the Service Options octet, with the low 2 bits of
// Reserved and of PriorityLevel. It is the inverse of
// DecodeServiceOptions.
func (s ServiceOptions) Byte() byte {
	var b byte
	for _, f := range [...]struct {
		set  bool
		mask byte
	}{{s.IsEmergency, 0x80}, {s.IsPrivacy, 0x40}, {s.IsBroadcast, 0x08}, {s.IsOpenVoiceCallMode, 0x04}} {
		if f.set {
			b |= f.mask
		}
	}
	return b | (s.Reserved&0x03)<<4 | s.PriorityLevel&0x03
}
