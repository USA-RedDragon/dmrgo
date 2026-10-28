package layer3

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// ShortDataServiceOptions is the 7-bit Service_Options of a Tier III UDT
// short data or DGNA request, with Field and an Of function as for
// VoiceServiceOptions. The other bits are not applicable and are 0. (ETSI
// TS 102 361-4 clause 7.2.13.7, Table 7.58)
//
//dmr:field ServiceOptions of a C_RAND
//dmr:const 0-0+3-6=0
//dmr:input_size 7
type ShortDataServiceOptions struct {
	IsPrivacy  bool `dmr:"bit:1"` // privacy, which ETSI TS 102 361-4 does not define
	IsSuppData bool `dmr:"bit:2"` // a Supplementary Data Transfer Service is requested for the call
}

// PollingServiceOptions is the 7-bit Service_Options of a Tier III UDT
// short data polling request, with Field and an Of function as for
// VoiceServiceOptions. (ETSI TS 102 361-4 clause 7.2.13.9, Table 7.60)
//
//dmr:field ServiceOptions of a C_RAND
//dmr:const 0-0+2-2=0
type PollingServiceOptions struct {
	IsPrivacy bool            `dmr:"bit:1"`    // privacy, which ETSI TS 102 361-4 does not define
	Format    enums.UDTFormat `dmr:"bits:3-6"` // POL_FMT: the format of the data to poll
}

// DiversionServiceOptions is the 7-bit Service_Options of a Tier III call
// diversion request, with Field and an Of function as for
// VoiceServiceOptions. The four kinds name the services to divert, or to
// stop diverting. (ETSI TS 102 361-4 clause 7.2.13.3, Table 7.54)
//
//dmr:field ServiceOptions of a C_RAND
//dmr:const 0-0=0
type DiversionServiceOptions struct {
	IsPrivacy  bool `dmr:"bit:1"` // privacy, which ETSI TS 102 361-4 does not define
	Set        bool `dmr:"bit:2"` // DIVONOFF: set the diversion, false to clear it
	Voice      bool `dmr:"bit:3"` // DIV_VD: voice calls
	PacketData bool `dmr:"bit:4"` // DIV_PD: packet data calls
	ShortData  bool `dmr:"bit:5"` // DIV_SD: UDT short data calls
	Status     bool `dmr:"bit:6"` // DIV_S: status calls
}
