package layer3

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// ServiceOptions is the Service Options information element (ETSI TS 102 361-2
// clause 7.2.1).
type ServiceOptions struct {
	IsEmergency         bool  `dmr:"bit:0"`
	IsPrivacy           bool  `dmr:"bit:1"`
	Reserved            uint8 `dmr:"bits:2-3"`
	IsBroadcast         bool  `dmr:"bit:4"`
	IsOpenVoiceCallMode bool  `dmr:"bit:5"`
	PriorityLevel       uint8 `dmr:"bits:6-7"`
}

// DefaultServiceOptions returns the Service Options octet 0x20 seen in
// Motorola IPSC voice traffic and sent by IPSC bridges: no flag set and the
// first Reserved bit set, so Reserved is 2. MMDVMHost sends 0x00, the zero
// ServiceOptions.
func DefaultServiceOptions() ServiceOptions {
	return ServiceOptions{Reserved: 2}
}

// ServiceOptionsFromByte decodes the Service Options octet b, as
// LCBytes.ServiceOptions returns it.
func ServiceOptionsFromByte(b byte) ServiceOptions {
	var bits [8]bit.Bit
	bit.PutUint(bits[:], uint64(b))
	return DecodeServiceOptions(bits)
}

// Byte returns s as the Service Options octet, with the low 2 bits of
// Reserved and of PriorityLevel. It is the inverse of
// ServiceOptionsFromByte.
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
