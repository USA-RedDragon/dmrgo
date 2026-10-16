package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// Call is the identity of a voice call, from its LC.
type Call struct {
	// Group reports whether Target is a talkgroup (Grp_V_Ch_Usr) rather
	// than a radio (UU_V_Ch_Usr).
	Group bool
	// Target is the 24-bit talkgroup or radio address.
	Target uint32
	// Source is the 24-bit address of the talking radio.
	Source uint32
	// Options is the Service Options of the LC.
	Options layer3.ServiceOptions
	// FID is the feature set ID of the LC.
	FID enums.FeatureSetID
	// LC is the 9 LC octets as received. It is zero for OriginUnknown and
	// for an OriginExpected call until a header or embedded LC with the
	// same addresses arrives.
	LC pdu.LCBytes
	// Origin is where the Receiver learned the call.
	Origin Origin
	// Stream is Hint.Stream when the transport gives one, else 0.
	Stream uint32
}

// callOf returns the Call of the LC octets lc, and ok false when lc is not
// a Grp_V_Ch_Usr or UU_V_Ch_Usr LC.
func callOf(lc pdu.LCBytes, origin Origin) (c Call, ok bool) {
	target, source, ok := lc.Addresses()
	if !ok {
		return c, false
	}
	group, _ := lc.Group()
	return Call{
		Group:   group,
		Target:  target,
		Source:  source,
		Options: layer3.ServiceOptionsFromByte(lc.ServiceOptions()),
		FID:     lc.FID(),
		LC:      lc,
		Origin:  origin,
	}, true
}

// sameParties reports whether c and o have the same target, source and
// call type.
func (c *Call) sameParties(o *Call) bool {
	return c.Group == o.Group && c.Target == o.Target && c.Source == o.Source
}

// Origin is where a Receiver learned the identity of a call.
type Origin uint8

// The Origin values.
const (
	// OriginUnknown is voice whose LC is not known yet.
	OriginUnknown Origin = iota
	// OriginHeader is a Voice LC Header.
	OriginHeader
	// OriginEmbedded is the embedded LC of bursts B to E (late entry).
	OriginEmbedded
	// OriginTerminator is a Terminator with LC, seen in the call hang time.
	OriginTerminator
	// OriginExpected is the call that Receiver.Expect gave, for voice that
	// came without an LC, such as after a Tier III grant.
	OriginExpected
)

// String returns the name of the origin.
func (o Origin) String() string {
	switch o {
	case OriginUnknown:
		return "Unknown"
	case OriginHeader:
		return "Header"
	case OriginEmbedded:
		return "Embedded"
	case OriginTerminator:
		return "Terminator"
	case OriginExpected:
		return "Expected"
	}
	return fmt.Sprintf("Origin(%d)", uint8(o))
}

// EndReason is why a call ended.
type EndReason uint8

// The EndReason values.
const (
	// EndTerminator is a Terminator with LC.
	EndTerminator EndReason = iota + 1
	// EndDataSync is a data burst other than a terminator.
	EndDataSync
	// EndIdle is an idle burst.
	EndIdle
	// EndLost is a time of ReceiverConfig.Lost without a burst.
	EndLost
	// EndSyncLost is a transport that lost the signal: Hint.SyncLost or
	// Receiver.Lost.
	EndSyncLost
	// EndTransport is the end flag of the transport, Hint.End EndStrong.
	EndTransport
	// EndSuperseded is a new call: an LC with other addresses, or another
	// transport stream.
	EndSuperseded
)

// String returns the name of the reason.
func (r EndReason) String() string {
	switch r {
	case EndTerminator:
		return "Terminator"
	case EndDataSync:
		return "DataSync"
	case EndIdle:
		return "Idle"
	case EndLost:
		return "Lost"
	case EndSyncLost:
		return "SyncLost"
	case EndTransport:
		return "Transport"
	case EndSuperseded:
		return "Superseded"
	}
	return fmt.Sprintf("EndReason(%d)", uint8(r))
}

// Stats counts what a Receiver saw during a call.
type Stats struct {
	// Bursts is the number of voice bursts of the call.
	Bursts uint16
	// Lost is the number of voice bursts that did not arrive.
	Lost uint16
	// Duplicates is the number of bursts dropped as copies of an earlier
	// one, voice bursts and Voice LC Headers.
	Duplicates uint16
	// Late is the number of voice bursts dropped because their sequence
	// number was behind the last one.
	Late uint16
	// BitsChecked is the number of vocoder bits that the AMBE+2 FEC
	// covered.
	BitsChecked uint32
	// BitErrors is the number of bit errors that the AMBE+2 FEC corrected.
	// BitErrors / BitsChecked is the bit error rate, as fec.BERCalculator
	// counts it.
	BitErrors uint32
	// Duration is the time from the start of the call to its end.
	Duration time.Duration
}

// FrameQuality is the FEC result of one 72-bit AMBE+2 vocoder slot.
type FrameQuality struct {
	// ErrorsCorrected is the number of bit errors that the Golay codes
	// corrected, at most 255.
	ErrorsCorrected uint8
	// Bad reports that section A of the frame was uncorrectable.
	Bad bool
}

// EventKind is the kind of an Event.
type EventKind uint8

// The EventKind values.
const (
	// EventCallStart starts a call. Call is set.
	EventCallStart EventKind = iota + 1
	// EventVoice is a voice burst. Call, Position, Burst, Frames and Lost
	// are set.
	EventVoice
	// EventEmbedded is an embedded LC that is not the call LC, such as GPS
	// or a talker alias block. Call and LC are set.
	EventEmbedded
	// EventTalkerAlias is a complete talker alias. Call and Text are set.
	EventTalkerAlias
	// EventPIHeader is a PI Header of the call. Call and Burst are set.
	EventPIHeader
	// EventCallEnd ends a call. Call, Reason and Stats are set.
	EventCallEnd
	// EventSessionStart joins a call session in its hang time, from a
	// Terminator with LC. Call is set.
	EventSessionStart
	// EventSessionEnd ends the call hang time with no new call to the same
	// target. Call is the last call of the session.
	EventSessionEnd
)

// String returns the name of the kind.
func (k EventKind) String() string {
	switch k {
	case EventCallStart:
		return "CallStart"
	case EventVoice:
		return "Voice"
	case EventEmbedded:
		return "Embedded"
	case EventTalkerAlias:
		return "TalkerAlias"
	case EventPIHeader:
		return "PIHeader"
	case EventCallEnd:
		return "CallEnd"
	case EventSessionStart:
		return "SessionStart"
	case EventSessionEnd:
		return "SessionEnd"
	}
	return fmt.Sprintf("EventKind(%d)", uint8(k))
}

// Event is one result of a Receiver. Fields that do not apply to Kind are
// zero. LC and Text point into the storage of the Receiver and are valid
// until its next method call.
type Event struct {
	// Kind is the kind of the event.
	Kind EventKind
	// Call is the call the event belongs to. For voice before the LC is
	// known, its Origin is OriginUnknown.
	Call Call
	// Position is the superframe position of a voice burst, or
	// VoiceBurstUnknown when no evidence placed it.
	Position enums.VoiceBurstType
	// Burst is the voice burst or the PI Header as received.
	Burst layer2.RawBurst
	// Frames is the FEC result of each vocoder slot of a voice burst.
	Frames [layer2.VoiceSlotsPerBurst]FrameQuality
	// Lost is the number of voice bursts lost just before this one, at most
	// 255.
	Lost uint8
	// LC is the embedded LC of an EventEmbedded.
	LC pdu.FullLinkControl
	// Text is the alias of an EventTalkerAlias, in UTF-8.
	Text []byte
	// Reason is why the call ended.
	Reason EndReason
	// Stats counts what the Receiver saw during the call.
	Stats Stats
}

// State is the state of a Receiver.
type State uint8

// The State values.
const (
	// StateIdle has no call and no session.
	StateIdle State = iota
	// StateLateEntry has voice whose LC is not known yet.
	StateLateEntry
	// StateInCall has a call.
	StateInCall
	// StateHang is the call hang time after a call, in which a new call to
	// the same target continues the session.
	StateHang
)

// String returns the name of the state.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateLateEntry:
		return "LateEntry"
	case StateInCall:
		return "InCall"
	case StateHang:
		return "Hang"
	}
	return fmt.Sprintf("State(%d)", uint8(s))
}
