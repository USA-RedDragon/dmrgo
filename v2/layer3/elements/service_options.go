package elements

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// ETSI TS 102 361-2 V2.4.1 (2017-10) - 7.2.1  Service Options
type ServiceOptions struct {
	IsEmergency         bool       `dmr:"bit:0"`
	IsPrivacy           bool       `dmr:"bit:1"`
	Reserved            [2]bit.Bit `dmr:"bits:2-3,raw"`
	IsBroadcast         bool       `dmr:"bit:4"`
	IsOpenVoiceCallMode bool       `dmr:"bit:5"`
	PriorityLevel       uint8      `dmr:"bits:6-7"`
}
