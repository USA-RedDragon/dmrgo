package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// AckReason is the 8-bit Reason of a Tier III acknowledgement, such as
// C_ACKD, C_NACKD, C_QACKD, C_WACKD, C_ACKU and P_ACKD. Its 8 bits, from
// the most significant, are the 2-bit acknowledgement type, the direction
// and the 5-bit reason (ETSI TS 102 361-4 clause 7.2.8, Tables 7.42 to
// 7.45).
type AckReason uint8

// The AckReason values that a TS sends.
const (
	ReasonMessageAccepted       AckReason = 0b0110_0000 // Message_Accepted
	ReasonStoreForward          AckReason = 0b0110_0001 // Store_Forward
	ReasonRegAccepted           AckReason = 0b0110_0010 // Reg_Accepted
	ReasonStatusPollAcceptedTS  AckReason = 0b0110_0011 // accepted for the status polling service
	ReasonAuthResponseTS        AckReason = 0b0110_0100 // authentication response of the TS
	ReasonRegSubscription       AckReason = 0b0110_0101 // Reg_Subscription/Attachment service
	ReasonNotSupported          AckReason = 0b0010_0000 // Not_Supported
	ReasonPermUserRefused       AckReason = 0b0010_0001 // Perm_User_Refused
	ReasonTempUserRefused       AckReason = 0b0010_0010 // Temp_User_Refused
	ReasonTransientSysRefused   AckReason = 0b0010_0011 // Transient_Sys_Refused
	ReasonNoregMSAwayRefused    AckReason = 0b0010_0100 // NoregMSaway_Refused
	ReasonMSAwayRefused         AckReason = 0b0010_0101 // MSaway_Refused
	ReasonDivCauseFail          AckReason = 0b0010_0110 // Div_Cause_Fail
	ReasonSysBusyRefused        AckReason = 0b0010_0111 // SYSbusy_Refused
	ReasonSysNotReady           AckReason = 0b0010_1000 // SYS_NotReady
	ReasonCallCancelRefused     AckReason = 0b0010_1001 // Call_Cancel_Refused
	ReasonRegRefused            AckReason = 0b0010_1010 // Reg_Refused
	ReasonRegDenied             AckReason = 0b0010_1011 // Reg_Denied
	ReasonIPConnectionFailed    AckReason = 0b0010_1100 // IP_Connection_failed
	ReasonMSNotRegistered       AckReason = 0b0010_1101 // MS_Not_Registered
	ReasonCalledPartyBusy       AckReason = 0b0010_1110 // Called_Party_Busy
	ReasonCalledGroupNotAllowed AckReason = 0b0010_1111 // Called_Group_Not_Allowed
	ReasonUDTUploadCRCError     AckReason = 0b0011_0000 // CRC error in the UDT upload phase
	ReasonDuplexCongestion      AckReason = 0b0011_0001 // Duplex_Congestion
	ReasonRefusedUnknownTS      AckReason = 0b0011_1111 // Refused_Reason_Unknown of the TS
	ReasonQueuedForResource     AckReason = 0b1010_0000 // Queued-for-resource, such as a payload channel
	ReasonQueuedForBusy         AckReason = 0b1010_0001 // Queued-for-busy, the called party is in another call
	ReasonWait                  AckReason = 0b1110_0000 // Wait
)

// The AckReason values that an MS sends, which a TS can mirror.
const (
	ReasonMSAccepted           AckReason = 0b0100_0100 // MS_Accepted
	ReasonCallBack             AckReason = 0b0100_0101 // CallBack
	ReasonMSAlerting           AckReason = 0b0100_0110 // MS_ALERTING
	ReasonStatusPollAcceptedMS AckReason = 0b0100_0111 // accepted for the status polling service
	ReasonAuthResponseMS       AckReason = 0b0100_1000 // authentication response of the MS
	ReasonMSNotSupported       AckReason = 0b0000_0000 // MSNot_Supported
	ReasonLineNotSupported     AckReason = 0b0001_0001 // LineNot_Supported
	ReasonStackFullRefused     AckReason = 0b0001_0010 // StackFull_Refused
	ReasonEquipBusyRefused     AckReason = 0b0001_0011 // EquipBusy_Refused
	ReasonRecipientRefused     AckReason = 0b0001_0100 // Recipient_Refused
	ReasonCustomRefused        AckReason = 0b0001_0101 // Custom_Refused
	ReasonMSDuplexNotSupported AckReason = 0b0001_0110 // MS_Duplex_Not_Supported
	ReasonRefusedUnknownMS     AckReason = 0b0001_1111 // Refused_Reason_Unknown of the MS
)

// AckType is the type of an acknowledgement, the two most significant bits
// of its AckReason (ETSI TS 102 361-4 clause 7.2.8.0).
type AckType uint8

// The AckType values.
const (
	AckNACK AckType = 0b00 // C_NACK, the request is refused
	AckACK  AckType = 0b01 // C_ACK, the request is accepted
	AckQACK AckType = 0b10 // C_QACK, the request is queued
	AckWACK AckType = 0b11 // C_WACK, more signalling follows
)

// String returns "NACK", "ACK", "QACK" or "WACK".
func (t AckType) String() string {
	switch t {
	case AckNACK:
		return "NACK"
	case AckACK:
		return "ACK"
	case AckQACK:
		return "QACK"
	case AckWACK:
		return "WACK"
	}
	return unknownName
}

// Type returns the acknowledgement type of r.
func (r AckReason) Type() AckType {
	return AckType(r >> 6)
}

// FromTS reports whether the d bit of r is 1: the TS sent the reason. It is
// false for a reason of an MS, also when a TS mirrors it.
func (r AckReason) FromTS() bool {
	return r&0x20 != 0
}

// String returns the mnemonic of r. It allocates only for a value without
// one.
func (r AckReason) String() string {
	if name := r.tsName(); name != "" {
		return name
	}
	if name := r.msName(); name != "" {
		return name
	}
	return "AckReason(" + text.Digits(uint64(r), 8, 1) + ")"
}

func (r AckReason) tsName() string {
	switch r {
	case ReasonMessageAccepted:
		return "Message_Accepted"
	case ReasonStoreForward:
		return "Store_Forward"
	case ReasonRegAccepted:
		return "Reg_Accepted"
	case ReasonStatusPollAcceptedTS, ReasonStatusPollAcceptedMS:
		return "Status_Poll_Accepted"
	case ReasonAuthResponseTS, ReasonAuthResponseMS:
		return "Authentication_Response"
	case ReasonRegSubscription:
		return "Reg_Subscription"
	case ReasonNotSupported:
		return "Not_Supported"
	case ReasonPermUserRefused:
		return "Perm_User_Refused"
	case ReasonTempUserRefused:
		return "Temp_User_Refused"
	case ReasonTransientSysRefused:
		return "Transient_Sys_Refused"
	case ReasonNoregMSAwayRefused:
		return "NoregMSaway_Refused"
	case ReasonMSAwayRefused:
		return "MSaway_Refused"
	case ReasonDivCauseFail:
		return "Div_Cause_Fail"
	case ReasonSysBusyRefused:
		return "SYSbusy_Refused"
	case ReasonSysNotReady:
		return "SYS_NotReady"
	case ReasonCallCancelRefused:
		return "Call_Cancel_Refused"
	case ReasonRegRefused:
		return "Reg_Refused"
	case ReasonRegDenied:
		return "Reg_Denied"
	case ReasonIPConnectionFailed:
		return "IP_Connection_failed"
	case ReasonMSNotRegistered:
		return "MS_Not_Registered"
	case ReasonCalledPartyBusy:
		return "Called_Party_Busy"
	case ReasonCalledGroupNotAllowed:
		return "Called_Group_Not_Allowed"
	case ReasonUDTUploadCRCError:
		return "CRC_error_in_the_UDT_Upload_phase"
	case ReasonDuplexCongestion:
		return "Duplex_Congestion"
	case ReasonRefusedUnknownTS, ReasonRefusedUnknownMS:
		return "Refused_Reason_Unknown"
	case ReasonQueuedForResource:
		return "Queued-for-resource"
	case ReasonQueuedForBusy:
		return "Queued-for-busy"
	case ReasonWait:
		return "Wait"
	default:
		return ""
	}
}

func (r AckReason) msName() string {
	switch r {
	case ReasonMSAccepted:
		return "MS_Accepted"
	case ReasonCallBack:
		return "CallBack"
	case ReasonMSAlerting:
		return "MS_ALERTING"
	case ReasonMSNotSupported:
		return "MSNot_Supported"
	case ReasonLineNotSupported:
		return "LineNot_Supported"
	case ReasonStackFullRefused:
		return "StackFull_Refused"
	case ReasonEquipBusyRefused:
		return "EquipBusy_Refused"
	case ReasonRecipientRefused:
		return "Recipient_Refused"
	case ReasonCustomRefused:
		return "Custom_Refused"
	case ReasonMSDuplexNotSupported:
		return "MS_Duplex_Not_Supported"
	default:
		return ""
	}
}
