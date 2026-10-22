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
	AckReasonMessageAccepted       AckReason = 0b0110_0000 // Message_Accepted
	AckReasonStoreForward          AckReason = 0b0110_0001 // Store_Forward
	AckReasonRegAccepted           AckReason = 0b0110_0010 // Reg_Accepted
	AckReasonStatusPollAcceptedTS  AckReason = 0b0110_0011 // accepted for the status polling service
	AckReasonAuthResponseTS        AckReason = 0b0110_0100 // authentication response of the TS
	AckReasonRegSubscription       AckReason = 0b0110_0101 // Reg_Subscription/Attachment service
	AckReasonNotSupported          AckReason = 0b0010_0000 // Not_Supported
	AckReasonPermUserRefused       AckReason = 0b0010_0001 // Perm_User_Refused
	AckReasonTempUserRefused       AckReason = 0b0010_0010 // Temp_User_Refused
	AckReasonTransientSysRefused   AckReason = 0b0010_0011 // Transient_Sys_Refused
	AckReasonNoregMSAwayRefused    AckReason = 0b0010_0100 // NoregMSaway_Refused
	AckReasonMSAwayRefused         AckReason = 0b0010_0101 // MSaway_Refused
	AckReasonDivCauseFail          AckReason = 0b0010_0110 // Div_Cause_Fail
	AckReasonSysBusyRefused        AckReason = 0b0010_0111 // SYSbusy_Refused
	AckReasonSysNotReady           AckReason = 0b0010_1000 // SYS_NotReady
	AckReasonCallCancelRefused     AckReason = 0b0010_1001 // Call_Cancel_Refused
	AckReasonRegRefused            AckReason = 0b0010_1010 // Reg_Refused
	AckReasonRegDenied             AckReason = 0b0010_1011 // Reg_Denied
	AckReasonIPConnectionFailed    AckReason = 0b0010_1100 // IP_Connection_failed
	AckReasonMSNotRegistered       AckReason = 0b0010_1101 // MS_Not_Registered
	AckReasonCalledPartyBusy       AckReason = 0b0010_1110 // Called_Party_Busy
	AckReasonCalledGroupNotAllowed AckReason = 0b0010_1111 // Called_Group_Not_Allowed
	AckReasonUDTUploadCRCError     AckReason = 0b0011_0000 // CRC error in the UDT upload phase
	AckReasonDuplexCongestion      AckReason = 0b0011_0001 // Duplex_Congestion
	AckReasonRefusedUnknownTS      AckReason = 0b0011_1111 // Refused_Reason_Unknown of the TS
	AckReasonQueuedForResource     AckReason = 0b1010_0000 // Queued-for-resource, such as a payload channel
	AckReasonQueuedForBusy         AckReason = 0b1010_0001 // Queued-for-busy, the called party is in another call
	AckReasonWait                  AckReason = 0b1110_0000 // Wait
)

// The AckReason values that an MS sends, which a TS can mirror.
const (
	AckReasonMSAccepted           AckReason = 0b0100_0100 // MS_Accepted
	AckReasonCallBack             AckReason = 0b0100_0101 // CallBack
	AckReasonMSAlerting           AckReason = 0b0100_0110 // MS_ALERTING
	AckReasonStatusPollAcceptedMS AckReason = 0b0100_0111 // accepted for the status polling service
	AckReasonAuthResponseMS       AckReason = 0b0100_1000 // authentication response of the MS
	AckReasonMSNotSupported       AckReason = 0b0000_0000 // MSNot_Supported
	AckReasonLineNotSupported     AckReason = 0b0001_0001 // LineNot_Supported
	AckReasonStackFullRefused     AckReason = 0b0001_0010 // StackFull_Refused
	AckReasonEquipBusyRefused     AckReason = 0b0001_0011 // EquipBusy_Refused
	AckReasonRecipientRefused     AckReason = 0b0001_0100 // Recipient_Refused
	AckReasonCustomRefused        AckReason = 0b0001_0101 // Custom_Refused
	AckReasonMSDuplexNotSupported AckReason = 0b0001_0110 // MS_Duplex_Not_Supported
	AckReasonRefusedUnknownMS     AckReason = 0b0001_1111 // Refused_Reason_Unknown of the MS
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
	case AckReasonMessageAccepted:
		return "Message_Accepted"
	case AckReasonStoreForward:
		return "Store_Forward"
	case AckReasonRegAccepted:
		return "Reg_Accepted"
	case AckReasonStatusPollAcceptedTS, AckReasonStatusPollAcceptedMS:
		return "Status_Poll_Accepted"
	case AckReasonAuthResponseTS, AckReasonAuthResponseMS:
		return "Authentication_Response"
	case AckReasonRegSubscription:
		return "Reg_Subscription"
	case AckReasonNotSupported:
		return "Not_Supported"
	case AckReasonPermUserRefused:
		return "Perm_User_Refused"
	case AckReasonTempUserRefused:
		return "Temp_User_Refused"
	case AckReasonTransientSysRefused:
		return "Transient_Sys_Refused"
	case AckReasonNoregMSAwayRefused:
		return "NoregMSaway_Refused"
	case AckReasonMSAwayRefused:
		return "MSaway_Refused"
	case AckReasonDivCauseFail:
		return "Div_Cause_Fail"
	case AckReasonSysBusyRefused:
		return "SYSbusy_Refused"
	case AckReasonSysNotReady:
		return "SYS_NotReady"
	case AckReasonCallCancelRefused:
		return "Call_Cancel_Refused"
	case AckReasonRegRefused:
		return "Reg_Refused"
	case AckReasonRegDenied:
		return "Reg_Denied"
	case AckReasonIPConnectionFailed:
		return "IP_Connection_failed"
	case AckReasonMSNotRegistered:
		return "MS_Not_Registered"
	case AckReasonCalledPartyBusy:
		return "Called_Party_Busy"
	case AckReasonCalledGroupNotAllowed:
		return "Called_Group_Not_Allowed"
	case AckReasonUDTUploadCRCError:
		return "CRC_error_in_the_UDT_Upload_phase"
	case AckReasonDuplexCongestion:
		return "Duplex_Congestion"
	case AckReasonRefusedUnknownTS, AckReasonRefusedUnknownMS:
		return "Refused_Reason_Unknown"
	case AckReasonQueuedForResource:
		return "Queued-for-resource"
	case AckReasonQueuedForBusy:
		return "Queued-for-busy"
	case AckReasonWait:
		return "Wait"
	default:
		return ""
	}
}

func (r AckReason) msName() string {
	switch r {
	case AckReasonMSAccepted:
		return "MS_Accepted"
	case AckReasonCallBack:
		return "CallBack"
	case AckReasonMSAlerting:
		return "MS_ALERTING"
	case AckReasonMSNotSupported:
		return "MSNot_Supported"
	case AckReasonLineNotSupported:
		return "LineNot_Supported"
	case AckReasonStackFullRefused:
		return "StackFull_Refused"
	case AckReasonEquipBusyRefused:
		return "EquipBusy_Refused"
	case AckReasonRecipientRefused:
		return "Recipient_Refused"
	case AckReasonCustomRefused:
		return "Custom_Refused"
	case AckReasonMSDuplexNotSupported:
		return "MS_Duplex_Not_Supported"
	default:
		return ""
	}
}
