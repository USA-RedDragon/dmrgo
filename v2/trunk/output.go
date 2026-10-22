package trunk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// MaxOutputs is the most outputs that one call of a Follower or MS method
// appends.
const MaxOutputs = 10

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputTune asks the consumer to tune the receiver, and the
	// transmitter of an MS, to Output.Tune.
	OutputTune OutputKind = iota + 1
	// OutputSite reports that the machine confirmed a TSCC, or that the
	// system parameters of the TSCC changed. Site returns them.
	OutputSite
	// OutputLost reports that the machine lost the TSCC. Output.Reason
	// tells why.
	OutputLost
	// OutputGrant reports a channel grant in Output.Grant, whether or not
	// the machine follows it.
	OutputGrant
	// OutputCallEnd reports that the machine left the payload channel of
	// Output.Grant. Output.Reason tells why.
	OutputCallEnd
	// OutputBurst is a burst for the MS to transmit in Output.Slot at
	// Output.Due.
	OutputBurst
	// OutputRegistered reports that the TSCC accepted the registration of
	// the MS.
	OutputRegistered
	// OutputRefused reports that the TSCC refused a request of the MS, or
	// that the request failed. Output.Reason tells why and Output.Ack holds
	// the reason of a C_NACKD.
	OutputRefused
	// OutputAhoy reports a C_AHOY for the MS, with the call in
	// Output.Grant: an individual call that the MS answers, or a radio
	// check.
	OutputAhoy
	// OutputUDT reports a UDT delivery, which UDT returns until the next
	// call of the machine.
	OutputUDT
	// OutputProtect reports a P_PROTECT for the MS on its payload channel,
	// with its kind in Output.Protect.
	OutputProtect
	// OutputPowerOff reports that the MS de-registered, or gave up after
	// T_Dereg, and may be switched off.
	OutputPowerOff
	// OutputAuthChallenge reports the authentication challenge of the TSCC
	// in Output.Challenge. Answer it with MS.AuthResponse, with the PSN of
	// the MS in trunkauth.Response only when the MS has one.
	OutputAuthChallenge
	// OutputDelivered reports that the TSCC completed a UDT short data,
	// polling, call diversion or DGNA request of the MS, with the reason in
	// Output.Ack.
	OutputDelivered
	// OutputPolled reports that the TSCC polled the MS, with a C_AHOY from
	// Output.Grant.Source or with a USBD Poll Request, and that the MS sent
	// its poll response or refused the poll.
	OutputPolled
	// OutputGroups reports that a DGNA transfer changed the talkgroups that
	// MS.Groups returns.
	OutputGroups
	// OutputAmbient reports that the MS joined the Ambient Listening
	// Service call of Output.Grant: it transmits without an action of its
	// user until it ends the call at T_ALS or T_ALS_E.
	OutputAmbient
	// OutputStatus reports a status in Output.Status: a status call to the
	// MS or to one of its talkgroups, with the parties in Output.Grant, or
	// the answer to a status poll of the MS, from Output.Grant.Source.
	OutputStatus
	// OutputAccepted reports that the TSCC accepted a request of the MS
	// that needs no payload channel, such as a status call, a transmit
	// interrupt or an IP connection advice. Output.Ack holds the reason.
	OutputAccepted
	// OutputStun reports that the TSCC stunned, revived or killed the MS,
	// with the action in Output.Stun.
	OutputStun
	// OutputReverseChannel reports the RC command in Output.RC on the
	// payload channel of the MS. An MS that transmits obeys it.
	OutputReverseChannel
	// OutputSleep reports that the MS may switch its receiver off until
	// Output.Due, its next power save frame.
	OutputSleep
)

// String returns the name of the OutputKind.
func (k OutputKind) String() string {
	switch k {
	case OutputTune:
		return "Tune"
	case OutputSite:
		return "Site"
	case OutputLost:
		return "Lost"
	case OutputGrant:
		return "Grant"
	case OutputCallEnd:
		return "CallEnd"
	case OutputBurst:
		return "Burst"
	case OutputRegistered:
		return "Registered"
	case OutputRefused:
		return "Refused"
	case OutputAhoy:
		return "Ahoy"
	case OutputUDT:
		return "UDT"
	case OutputProtect:
		return "Protect"
	case OutputPowerOff:
		return "PowerOff"
	case OutputAuthChallenge:
		return "AuthChallenge"
	case OutputDelivered:
		return "Delivered"
	case OutputPolled:
		return "Polled"
	case OutputGroups:
		return "Groups"
	case OutputAmbient:
		return "Ambient"
	case OutputStatus:
		return "Status"
	case OutputAccepted:
		return "Accepted"
	case OutputStun:
		return stunName
	case OutputReverseChannel:
		return "ReverseChannel"
	case OutputSleep:
		return "Sleep"
	}
	return unknownName
}

// Reason tells why a machine lost a TSCC, left a payload channel or failed
// a request.
type Reason uint8

// The Reason values.
const (
	// ReasonNone is the zero Reason.
	ReasonNone Reason = iota
	// ReasonNoSignal is T_Nosig without a TSCC burst, or a hunt list
	// channel without a TSCC.
	ReasonNoSignal
	// ReasonSysCode is a TSCC with another C_SYScode, NSYSerr times.
	ReasonSysCode
	// ReasonClear is a P_CLEAR on the payload channel.
	ReasonClear
	// ReasonInactive is TV_Inactive or TD_Inactive without a burst on the
	// payload channel.
	ReasonInactive
	// ReasonHangtime is the end of the call hang time after a terminator.
	ReasonHangtime
	// ReasonMaintenance is the end of the own call: the MS sent P_MAINT.
	ReasonMaintenance
	// ReasonNack is a C_NACKD for the request, with its reason in
	// Output.Ack.
	ReasonNack
	// ReasonNoAnswer is a random access that got no answer after the most
	// attempts, or after TRand_TC.
	ReasonNoAnswer
	// ReasonTimeout is TP_Timer, TNP_Timer or T_Pending.
	ReasonTimeout
	// ReasonCancelled is a request that the consumer cancelled.
	ReasonCancelled
	// ReasonMove is a C_MOVE to another TSCC.
	ReasonMove
	// ReasonQuality is a TSCC whose signal level or bit error rate is
	// below the thresholds of the MS.
	ReasonQuality
	// ReasonDenied is a TSCC in a SYS_AREA that denied the registration of
	// the MS.
	ReasonDenied
	// ReasonParked is a P_PROTECT ILLEGALLY_PARKED for an MS that is
	// neither the source nor the target of the call.
	ReasonParked
	// ReasonDiverted is a request that the TSCC answered with the diverted
	// address of the called party in a UDT, which UDT returns.
	ReasonDiverted
	// ReasonInterrupt is a transmit interrupt: the MS left the payload
	// channel to ask for it, or the TSCC stopped the transmission of the
	// call.
	ReasonInterrupt
	// ReasonCallTimer is the end of the longest call time of a TSCC.
	ReasonCallTimer
	// ReasonAuthentication is a failed authentication: no key for the MS,
	// or a response that the MS refused.
	ReasonAuthentication
)

// String returns the name of the Reason.
func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "None"
	case ReasonNoSignal:
		return "NoSignal"
	case ReasonSysCode:
		return "SysCode"
	case ReasonClear:
		return "Clear"
	case ReasonInactive:
		return "Inactive"
	case ReasonHangtime:
		return "Hangtime"
	case ReasonMaintenance:
		return "Maintenance"
	case ReasonNack:
		return "Nack"
	case ReasonNoAnswer:
		return "NoAnswer"
	case ReasonTimeout:
		return "Timeout"
	case ReasonCancelled:
		return "Cancelled"
	case ReasonMove:
		return "Move"
	case ReasonQuality:
		return "Quality"
	case ReasonDenied:
		return "Denied"
	case ReasonParked:
		return "Parked"
	case ReasonDiverted:
		return "Diverted"
	case ReasonInterrupt:
		return "Interrupt"
	case ReasonCallTimer:
		return "CallTimer"
	case ReasonAuthentication:
		return "Authentication"
	}
	return unknownName
}

// Tune is a channel to tune to.
type Tune struct {
	// Channel is the channel number, or CHNULL for the TSCC that the
	// consumer tuned last without a hunt list.
	Channel uint16
	// Slot is the TDMA channel to follow on a payload channel, or
	// enums.SlotUnknown on a TSCC.
	Slot enums.Slot
	// RXHz is the receive frequency from the channel plan or a CG_AP
	// block, or 0 when not known.
	RXHz uint64
	// TXHz is the transmit frequency, or 0 when not known.
	TXHz uint64
	// ColorCode is the color code of the channel, when HasColorCode is set.
	ColorCode enums.ColorCode
	// HasColorCode reports whether ColorCode is known.
	HasColorCode bool
}

// Grant is a channel grant of a TSCC, such as TV_GRANT or PV_GRANT (ETSI
// TS 102 361-4 clause 7.1.1.1.1).
type Grant struct {
	// Opcode is the CSBK opcode of the grant.
	Opcode pdu.CSBKOpcode
	// Service is the service that the grant is for.
	Service enums.ServiceKind
	// Group reports whether Target is a talkgroup.
	Group bool
	// Target is the 24-bit talkgroup or MS address.
	Target uint32
	// Source is the 24-bit address of the calling MS.
	Source uint32
	// Channel is the payload channel, from the grant or its CG_AP block.
	Channel uint16
	// Slot is the TDMA channel of the call on the payload channel.
	Slot enums.Slot
	// RXHz and TXHz are the frequencies of Channel from the plan or the
	// CG_AP block, 0 when not known.
	RXHz, TXHz uint64
	// ColorCode is the color code from a CG_AP block, when HasColorCode is
	// set.
	ColorCode enums.ColorCode
	// HasColorCode reports whether ColorCode is known.
	HasColorCode bool
	// Emergency reports an emergency call.
	Emergency bool
	// LateEntry reports a grant sent again after the call set-up.
	LateEntry bool
	// Offset reports offset timing on the payload channel.
	Offset bool
	// HiRate reports dual slot data.
	HiRate bool
}

// Call returns the voice.Call of a voice grant, for voice.Receiver.Expect.
// Trunked voice can start without a Voice LC Header.
func (g *Grant) Call() voice.Call {
	if g == nil {
		return voice.Call{}
	}

	return voice.Call{Group: g.Group, Target: g.Target, Source: g.Source}
}

// Data reports whether the grant is for packet data.
func (g *Grant) Data() bool {
	if g == nil {
		return false
	}

	return g.Service == enums.ServiceKindIndividualPacketData || g.Service == enums.ServiceKindTalkgroupPacketData ||
		g.Service == enums.ServiceKindDuplexPacketData
}

func (g *Grant) tune() Tune {
	return Tune{Channel: g.Channel, Slot: g.Slot, RXHz: g.RXHz, TXHz: g.TXHz, ColorCode: g.ColorCode, HasColorCode: g.HasColorCode}
}

// Output is one result of a Follower or MS. Fields that do not apply to
// Kind are zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Tune is the channel of OutputTune.
	Tune Tune
	// Grant is the grant of OutputGrant and OutputCallEnd, and the call of
	// OutputAhoy.
	Grant Grant
	// Reason tells why for OutputLost, OutputCallEnd and OutputRefused.
	Reason Reason
	// Ack is the reason code of the TSCC for OutputRefused with ReasonNack
	// and for OutputRegistered.
	Ack enums.AckReason
	// Burst is the burst of OutputBurst.
	Burst layer2.RawBurst
	// Hint is what Burst is, for a transport: its Slot and KindData with
	// enums.DataTypeCSBK.
	Hint layer2.Hint
	// Slot is the TDMA channel of OutputBurst.
	Slot enums.Slot
	// Due is the time at which to transmit OutputBurst.
	Due time.Duration
	// Protect is the kind of OutputProtect.
	Protect enums.ProtectKind
	// Challenge is the 24-bit challenge of OutputAuthChallenge.
	Challenge uint32
	// Status is the status of OutputStatus.
	Status Status
	// Stun is the action of OutputStun.
	Stun StunAction
	// RC is the command of OutputReverseChannel.
	RC enums.RCCommand
}

// State is the state of a Follower or an MS.
type State uint8

// The State values.
const (
	// StateHunting looks for a TSCC.
	StateHunting State = iota
	// StateControl is on a confirmed TSCC.
	StateControl
	// StatePayload follows a call on a payload channel.
	StatePayload
)

// String returns the name of the State.
func (s State) String() string {
	switch s {
	case StateHunting:
		return "Hunting"
	case StateControl:
		return "Control"
	case StatePayload:
		return "Payload"
	}
	return unknownName
}

const (
	unknownName = "Unknown"
	stunName    = "Stun"
)

// UDT is a Unified Data Transport delivery of a TSCC: a C_UDTHD header
// and its 1 to 4 appended rate 1/2 blocks (ETSI TS 102 361-4 clause 6.5).
type UDT struct {
	// Header is the C_UDTHD header. Its AppendedBlocks plus one is the
	// number of appended blocks.
	Header pdu.UDTHeader
	// Data holds the 12*(Header.AppendedBlocks+1)-2 octets of the appended
	// blocks before the CRC of the last block.
	Data [MaxAppendedOctets]byte
	// CRCOK reports whether the CRC of the last block checks.
	CRCOK bool
}
