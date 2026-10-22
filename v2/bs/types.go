package bs

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const rejectedName = "Rejected"

// Mode selects what a Repeater makes.
type Mode uint8

const (
	// ModeRF is a repeater or hotspot with a transmitter: one outbound burst
	// per 30 ms slot, hang time bursts, Idle bursts and hibernation.
	ModeRF Mode = iota
	// ModeNetwork is the slot arbiter of a network server without RF: it
	// regenerates and forwards the bursts of the owner of each slot and
	// makes no CACH, hang time or Idle bursts.
	ModeNetwork
)

// String returns the name of the mode. It allocates only for a value
// without a name.
func (m Mode) String() string {
	switch m {
	case ModeRF:
		return "RF"
	case ModeNetwork:
		return "Network"
	}
	return "Mode(" + text.Int(int64(m)) + ")"
}

// Activation selects what wakes up a hibernating Repeater (ETSI TS 102
// 361-1 clause 5.2.1.3).
type Activation uint8

const (
	// ActivationAny wakes up on any valid burst with the own color code, or
	// on any network burst ("MS established timing").
	ActivationAny Activation = iota
	// ActivationWakeup wakes up only on a BS_Dwn_Act CSBK with the own color
	// code, or on a network burst ("BS established timing", ETSI TS 102
	// 361-2 clause 5.1.1.1).
	ActivationWakeup
)

// String returns the name of the activation. It allocates only for a value
// without a name.
func (a Activation) String() string {
	switch a {
	case ActivationAny:
		return "Any"
	case ActivationWakeup:
		return "Wakeup"
	}
	return "Activation(" + text.Int(int64(a)) + ")"
}

// ChannelAuth selects the channel authorization of a Repeater in ModeRF:
// how it picks one MS among those that start a transmission in hang time
// (ETSI TS 102 361-2 clause 5.1.1.6).
type ChannelAuth uint8

const (
	// ChannelAuthOff gives the slot to the first burst that the hang time
	// admits.
	ChannelAuthOff ChannelAuth = iota
	// ChannelAuthSingleSite repeats the ChanAuth PDU of the first MS, a
	// Voice LC Header or a Pre_CSBK from RF, and gives the slot to the
	// bursts with its source address (ETSI TS 102 361-2 clause 5.1.1.6.1.1).
	ChannelAuthSingleSite
	// ChannelAuthMultiSite first collects the ChanAuth PDUs from RF and
	// from the network for T_BS_ChanAuthSel, picks the one with the lowest
	// source address, and then repeats it as ChannelAuthSingleSite does
	// (ETSI TS 102 361-2 clause 5.1.1.6.1.2).
	ChannelAuthMultiSite
)

// String returns the name of the channel authorization. It allocates only
// for a value without a name.
func (c ChannelAuth) String() string {
	switch c {
	case ChannelAuthOff:
		return "Off"
	case ChannelAuthSingleSite:
		return "SingleSite"
	case ChannelAuthMultiSite:
		return "MultiSite"
	}
	return "ChannelAuth(" + text.Int(int64(c)) + ")"
}

// Source is where an inbound burst came from. The zero Source is network
// peer 0.
type Source struct {
	// RF reports a burst from the receiver of the Repeater.
	RF bool
	// Peer is the network peer, such as a repeater ID. It is 0 for RF.
	Peer uint32
}

// Dest is a set of places an output burst goes to.
type Dest uint8

const (
	// ToRF sends the burst on the outbound channel.
	ToRF Dest = 1 << iota
	// ToNetwork sends the burst to the network: in ModeNetwork to the peers
	// other than Output.From, in ModeRF to the network that the hotspot or
	// repeater is linked to.
	ToNetwork
)

// OutputKind is the kind of an Output.
type OutputKind uint8

const (
	// OutputBurst is a burst to send at Due to the places in To. In ModeRF
	// with Config.CACH, an OutputBurst for ToRF also carries the CACH.
	OutputBurst OutputKind = iota + 1
	// OutputCallStart is a source that took a slot. Slot, From, Service and
	// Call are set.
	OutputCallStart
	// OutputCallEnd is the end of the call or data on a slot. Slot, From,
	// Service, Call and End are set.
	OutputCallEnd
	// OutputRejected is a source that the Repeater refused. Slot, From and
	// Reason are set. Call is set for ReasonTimeout, for ReasonRejected
	// when the Repeater knows the call, and for ReasonSlotBusy and
	// ReasonHangTime when the refused burst carries the full LC, as a Voice
	// LC header does; else it is zero.
	OutputRejected
	// OutputKey is the wakeup of the transmitter in ModeRF.
	OutputKey
	// OutputDekey is the start of hibernation in ModeRF.
	OutputDekey
)

// String returns the name of the kind. It allocates only for a value
// without a name.
func (k OutputKind) String() string {
	switch k {
	case OutputBurst:
		return "Burst"
	case OutputCallStart:
		return "CallStart"
	case OutputCallEnd:
		return "CallEnd"
	case OutputRejected:
		return rejectedName
	case OutputKey:
		return "Key"
	case OutputDekey:
		return "Dekey"
	}
	return "OutputKind(" + text.Int(int64(k)) + ")"
}

// Service is what a source uses a slot for.
type Service uint8

const (
	// ServiceVoice is a voice call.
	ServiceVoice Service = iota
	// ServiceData is a data transfer: a data header and its blocks.
	ServiceData
)

// String returns the name of the service. It allocates only for a value
// without a name.
func (s Service) String() string {
	switch s {
	case ServiceVoice:
		return "Voice"
	case ServiceData:
		return "Data"
	}
	return "Service(" + text.Int(int64(s)) + ")"
}

// Reason is why a Repeater refused a source.
type Reason uint8

const (
	// ReasonSlotBusy is a slot that another source owns. The source waits in
	// the queue of Config.Pending when it has room.
	ReasonSlotBusy Reason = iota + 1
	// ReasonHangTime is a call to another target from another Source in
	// the call hang time of the slot (ETSI TS 102 361-1 clause 5.2.1.4).
	// All RF MSs are one Source, so the call of the slot owner, or of any
	// RF MS after an RF call, takes the slot for any target, as MMDVMHost
	// does; the politeness rules of the MSs keep the others off.
	ReasonHangTime
	// ReasonTimeout is a call that ran longer than Config.Timeout. The rest
	// of the call is dropped.
	ReasonTimeout
	// ReasonRejected is a call that the caller refused with
	// Repeater.Reject. The rest of the call is dropped.
	ReasonRejected
)

// String returns the name of the reason. It allocates only for a value
// without a name.
func (r Reason) String() string {
	switch r {
	case ReasonSlotBusy:
		return "SlotBusy"
	case ReasonHangTime:
		return "HangTime"
	case ReasonTimeout:
		return "Timeout"
	case ReasonRejected:
		return rejectedName
	}
	return "Reason(" + text.Int(int64(r)) + ")"
}

// Output is one result of a Repeater. Fields that do not apply to Kind are
// zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Slot is the TDMA channel.
	Slot enums.Slot
	// To is where an OutputBurst goes.
	To Dest
	// HasCACH reports that CACH is set.
	HasCACH bool
	// CACH is the CACH field before the burst, in transmit order, packed as
	// layer2.DecodeCACHField takes it.
	CACH [3]byte
	// Burst is the burst to send, with the color code and BS SYNC of the
	// Repeater.
	Burst layer2.RawBurst
	// Hint is what Burst is, for a transport: Slot, and KindVoice with the
	// superframe position or KindData with the data type.
	Hint layer2.Hint
	// Due is the time at which the burst goes out.
	Due time.Duration
	// From is the source of the call that the output belongs to.
	From Source
	// Service is what the source uses the slot for.
	Service Service
	// Call is the identity of the call. For data it has Group, Target and
	// Source from the data header.
	Call voice.Call
	// End is why the call ended.
	End voice.EndReason
	// Reason is why the source was refused.
	Reason Reason
}

// State is the state of both slots of a Repeater (ETSI TS 102 361-1
// Figure G.5).
type State uint8

const (
	// StateHibernating has the transmitter off. ModeNetwork never
	// hibernates.
	StateHibernating State = iota
	// StateHangtime is keyed with both slots free.
	StateHangtime
	// StateRepeatingSlot1 repeats slot 1.
	StateRepeatingSlot1
	// StateRepeatingSlot2 repeats slot 2.
	StateRepeatingSlot2
	// StateRepeatingBoth repeats both slots.
	StateRepeatingBoth
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s State) String() string {
	switch s {
	case StateHibernating:
		return "Hibernating"
	case StateHangtime:
		return "Hangtime"
	case StateRepeatingSlot1:
		return "RepeatingSlot1"
	case StateRepeatingSlot2:
		return "RepeatingSlot2"
	case StateRepeatingBoth:
		return "RepeatingBoth"
	}
	return "State(" + text.Int(int64(s)) + ")"
}

// SlotState is the state of one slot of a Repeater (ETSI TS 102 361-1
// Figure G.6, ETSI TS 102 361-3 clause 5.4.3.3).
type SlotState uint8

const (
	// SlotIdle is the channel hang time: the slot is free and gets Idle
	// bursts.
	SlotIdle SlotState = iota
	// SlotVoice repeats a voice call.
	SlotVoice
	// SlotData repeats a data header and its blocks.
	SlotData
	// SlotCallHang is the call hang time after a voice call: only a call to
	// the same target takes the slot, and it gets Terminators with the call
	// LC.
	SlotCallHang
	// SlotDataHang is the data hang time after confirmed data: it gets
	// TD_LC and waits for the response.
	SlotDataHang
	// SlotRejected drops a call that timed out or was rejected until it
	// ends.
	SlotRejected
	// SlotChanAuthSelect collects ChanAuth PDUs for T_BS_ChanAuthSel and
	// keeps sending the hang time bursts (ETSI TS 102 361-2 clause
	// 5.1.1.6.1.2).
	SlotChanAuthSelect
	// SlotChanAuth repeats the ChanAuth PDU of the authorized MS until
	// T_BS_ChanAuthRsp runs out or a burst with its source address comes
	// (ETSI TS 102 361-2 clause 5.1.1.6.1).
	SlotChanAuth
	// SlotPreamble repeats the Pre_CSBKs of one source before its data or
	// CSBK. The CACH marks the slot busy and the Activity Update carries
	// the data or CSBK activity until the data header or the CSBK comes, or
	// Config.Lost passes without a Pre_CSBK (ETSI TS 102 361-1 clause 6.3,
	// ETSI TS 102 361-2 clause 7.1.2.5).
	SlotPreamble
)

// String returns the name of the slot state. It allocates only for a value
// without a name.
func (s SlotState) String() string {
	switch s {
	case SlotIdle:
		return "Idle"
	case SlotVoice:
		return "Voice"
	case SlotData:
		return "Data"
	case SlotCallHang:
		return "CallHang"
	case SlotDataHang:
		return "DataHang"
	case SlotRejected:
		return rejectedName
	case SlotChanAuthSelect:
		return "ChanAuthSelect"
	case SlotChanAuth:
		return "ChanAuth"
	case SlotPreamble:
		return "Preamble"
	}
	return "SlotState(" + text.Int(int64(s)) + ")"
}
