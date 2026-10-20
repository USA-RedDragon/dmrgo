package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// ServiceKind is the 4-bit Service_Kind of a Tier III C_RAND, C_AHOY or
// C_ACKVIT (ETSI TS 102 361-4 clause 7.2.12, Table 7.49).
type ServiceKind uint8

// The ServiceKind values. 0b1100 is reserved.
const (
	ServiceIndividualVoice      ServiceKind = 0b0000 // individual voice call
	ServiceTalkgroupVoice       ServiceKind = 0b0001 // talkgroup voice call
	ServiceIndividualPacketData ServiceKind = 0b0010 // individual packet data call
	ServiceTalkgroupPacketData  ServiceKind = 0b0011 // packet data call to a talkgroup
	ServiceIndividualShortData  ServiceKind = 0b0100 // individual UDT short data call
	ServiceTalkgroupShortData   ServiceKind = 0b0101 // talkgroup UDT short data call
	ServiceShortDataPolling     ServiceKind = 0b0110 // UDT short data polling
	ServiceStatusTransport      ServiceKind = 0b0111 // status transport
	ServiceCallDiversion        ServiceKind = 0b1000 // call diversion
	ServiceCallAnswer           ServiceKind = 0b1001 // call answer
	ServiceDuplexVoice          ServiceKind = 0b1010 // full-duplex MS to MS voice call
	ServiceDuplexPacketData     ServiceKind = 0b1011 // full-duplex MS to MS packet data call
	ServiceSupplementary        ServiceKind = 0b1101 // supplementary service
	ServiceRegistration         ServiceKind = 0b1110 // registration, authentication, de-registration and MS radio check
	ServiceCancelCall           ServiceKind = 0b1111 // cancel call
)

// NeedsPayloadChannel reports whether the service uses a payload channel:
// the voice and packet data calls. The Service_Function of a C_ALOHA
// invites random access by this split (ETSI TS 102 361-4 Table 6.1).
func (k ServiceKind) NeedsPayloadChannel() bool {
	switch k {
	case ServiceIndividualVoice, ServiceTalkgroupVoice, ServiceIndividualPacketData,
		ServiceTalkgroupPacketData, ServiceDuplexVoice, ServiceDuplexPacketData:
		return true
	default:
		return false
	}
}

// String returns the name of the ServiceKind. It allocates only for a
// reserved value.
func (k ServiceKind) String() string {
	switch k {
	case ServiceIndividualVoice:
		return "Individual Voice"
	case ServiceTalkgroupVoice:
		return "Talkgroup Voice"
	case ServiceIndividualPacketData:
		return "Individual Packet Data"
	case ServiceTalkgroupPacketData:
		return "Talkgroup Packet Data"
	case ServiceIndividualShortData:
		return "Individual UDT Short Data"
	case ServiceTalkgroupShortData:
		return "Talkgroup UDT Short Data"
	case ServiceShortDataPolling:
		return "UDT Short Data Polling"
	case ServiceStatusTransport:
		return "Status Transport"
	case ServiceCallDiversion:
		return "Call Diversion"
	case ServiceCallAnswer:
		return "Call Answer"
	case ServiceDuplexVoice:
		return "Full-Duplex Voice"
	case ServiceDuplexPacketData:
		return "Full-Duplex Packet Data"
	case ServiceSupplementary:
		return "Supplementary"
	case ServiceRegistration:
		return "Registration"
	case ServiceCancelCall:
		return "Cancel Call"
	}
	return "Reserved ServiceKind(" + text.Int(int64(k)) + ")"
}

// ServiceFunction is the 2-bit Service_Function of a C_ALOHA, which tells
// for which services an MS may make a random access (ETSI TS 102 361-4
// clause 6.2.1.1.2, Table 6.1).
type ServiceFunction uint8

// The ServiceFunction values.
const (
	// ServiceFunctionAll invites random access for all services.
	ServiceFunctionAll ServiceFunction = 0b00
	// ServiceFunctionPayload invites random access for registration and
	// the services that need a payload channel.
	ServiceFunctionPayload ServiceFunction = 0b01
	// ServiceFunctionNoPayload invites random access for registration and
	// the services that do not need a payload channel.
	ServiceFunctionNoPayload ServiceFunction = 0b10
	// ServiceFunctionRegistration invites random access for registration
	// only.
	ServiceFunctionRegistration ServiceFunction = 0b11
)

// Invites reports whether the Service_Function invites a random access
// for the service kind k. An emergency request does not check it.
func (f ServiceFunction) Invites(k ServiceKind) bool {
	switch f {
	case ServiceFunctionAll:
		return true
	case ServiceFunctionPayload:
		return k == ServiceRegistration || k.NeedsPayloadChannel()
	case ServiceFunctionNoPayload:
		return k == ServiceRegistration || !k.NeedsPayloadChannel()
	case ServiceFunctionRegistration:
		return k == ServiceRegistration
	}
	return false
}

// String returns the name of the ServiceFunction.
func (f ServiceFunction) String() string {
	switch f {
	case ServiceFunctionAll:
		return "All Services"
	case ServiceFunctionPayload:
		return "Payload Channel Services and Registration"
	case ServiceFunctionNoPayload:
		return "Non-Payload Services and Registration"
	case ServiceFunctionRegistration:
		return "Registration Only"
	}
	return unknownName
}

// NRandWait is the 4-bit NRand_Wait of a C_ALOHA, the code for the number
// of TDMA frames by which the TSCC may delay its answer to a random access
// (ETSI TS 102 361-4 clause 6.2.1.1.5).
type NRandWait uint8

// Frames returns the number of TDMA frames, NWait, of the code (ETSI TS
// 102 361-4 Table 6.2). It returns 0 for a value over 15.
func (n NRandWait) Frames() int {
	switch {
	case n <= 13:
		return int(n)
	case n == 14:
		return 15
	case n == 15:
		return 24
	}
	return 0
}

// Backoff is the 4-bit Backoff number of a TSCC PDU such as C_ALOHA, the
// code for the number of TDMA frames from which an MS picks the frame of
// its next random access (ETSI TS 102 361-4 clause 6.2.1.1.6).
type Backoff uint8

// Frames returns the number of TDMA frames of the backoff number (ETSI TS
// 102 361-4 Table 6.3). It returns 0 for the reserved 0 and for a value
// over 15.
func (b Backoff) Frames() int {
	switch {
	case b <= 5:
		return int(b)
	case b <= 15:
		return [10]int{8, 11, 15, 20, 26, 33, 41, 50, 70, 100}[b-6]
	}
	return 0
}
