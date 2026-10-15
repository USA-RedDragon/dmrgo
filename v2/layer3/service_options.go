package layer3

// ETSI TS 102 361-2 - 7.2.1  Service Options
type ServiceOptions struct {
	IsEmergency         bool  `dmr:"bit:0"`
	IsPrivacy           bool  `dmr:"bit:1"`
	Reserved            uint8 `dmr:"bits:2-3"`
	IsBroadcast         bool  `dmr:"bit:4"`
	IsOpenVoiceCallMode bool  `dmr:"bit:5"`
	PriorityLevel       uint8 `dmr:"bits:6-7"`
}
