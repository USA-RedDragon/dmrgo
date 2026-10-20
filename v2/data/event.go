package data

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

const idleName = "Idle"

func unnamed(typ string, v uint8) string {
	return typ + "(" + text.Int(int64(v)) + ")"
}

// EventKind is the kind of an Event.
type EventKind uint8

// The EventKind values.
const (
	// EventCallStart starts a data call, at a preamble CSBK or the first
	// header. Call is set.
	EventCallStart EventKind = iota + 1
	// EventDelivered is a complete packet. Call and Message are set.
	EventDelivered
	// EventResponse is a response packet heard on the channel. Call,
	// Response and, for a SACK, Retry are set.
	EventResponse
	// EventRespond is a response burst to send. Call, Response, Burst and
	// Due are set.
	EventRespond
	// EventCallEnd ends a data call. Call, Reason and Stats are set.
	EventCallEnd
	// EventReassembled is a confirmed message joined from its fragments,
	// after the EventDelivered of its last fragment. Call and Message are
	// set, with Message.Fragments.
	EventReassembled
)

// String returns the name of the kind. It allocates only for a value
// without a name.
func (k EventKind) String() string {
	switch k {
	case EventCallStart:
		return "CallStart"
	case EventDelivered:
		return "Delivered"
	case EventResponse:
		return "Response"
	case EventRespond:
		return "Respond"
	case EventCallEnd:
		return "CallEnd"
	case EventReassembled:
		return "Reassembled"
	}
	return unnamed("EventKind", uint8(k))
}

// EndReason is why a data call ended.
type EndReason uint8

// The EndReason values.
const (
	// EndQuiet is a time of ReceiverConfig.Hang without a burst.
	EndQuiet EndReason = iota + 1
	// EndIdle is an idle burst after the last packet.
	EndIdle
	// EndVoice is a voice burst or a Voice LC Header.
	EndVoice
	// EndSyncLost is a transport that lost the signal: Hint.SyncLost or
	// Receiver.Lost.
	EndSyncLost
	// EndTransport is the end flag of the transport, Hint.End EndStrong.
	EndTransport
	// EndSuperseded is a header or preamble of other parties.
	EndSuperseded
)

// String returns the name of the reason. It allocates only for a value
// without a name.
func (r EndReason) String() string {
	switch r {
	case EndQuiet:
		return "Quiet"
	case EndIdle:
		return idleName
	case EndVoice:
		return "Voice"
	case EndSyncLost:
		return "SyncLost"
	case EndTransport:
		return "Transport"
	case EndSuperseded:
		return "Superseded"
	}
	return unnamed("EndReason", uint8(r))
}

// Stats counts what a Receiver saw during a data call.
type Stats struct {
	// Packets is the number of packets that completed, delivered or not.
	Packets uint16
	// Delivered is the number of EventDelivered.
	Delivered uint16
	// Blocks is the number of data blocks received.
	Blocks uint16
	// Lost is the number of data blocks that did not arrive or did not
	// decode.
	Lost uint16
	// HeaderCRC is the number of data headers dropped for a bad CRC.
	HeaderCRC uint16
	// BadCRC is the number of packets whose message CRC or block CRC
	// failed.
	BadCRC uint16
	// Duplicates is the number of confirmed packets dropped as a copy of
	// the packet before (N(S) equal to VI).
	Duplicates uint16
	// OutOfSequence is the number of confirmed packets refused for an N(S)
	// that is neither VI nor VI + 1.
	OutOfSequence uint16
	// Dropped is the number of other bursts dropped: blocks without a
	// header, packets over MaxBlocks, and headers that the Assembler
	// refused.
	Dropped uint16
	// Duration is the time from the start of the call to its end.
	Duration time.Duration
}

// Event is one result of a Receiver. Fields that do not apply to Kind are
// zero. Message.Payload, the sub-headers of Message.Header and Retry point
// into the storage of the Receiver and are valid until its next method
// call.
type Event struct {
	// Kind is the kind of the event.
	Kind EventKind
	// Call is the data call the event belongs to.
	Call Call
	// Message is the delivered packet.
	Message Message
	// Response is the response heard or sent.
	Response packet.Response
	// Retry is the selective retry flags of a SACK heard, or nil.
	Retry *packet.ResponseData
	// Burst is the response burst to send.
	Burst layer2.RawBurst
	// Due is the time at which the response burst goes on air, 60 ms after
	// the one before it.
	Due time.Duration
	// Reason is why the call ended.
	Reason EndReason
	// Stats counts what the Receiver saw during the call.
	Stats Stats
}

// State is the state of a Receiver.
type State uint8

// The State values.
const (
	// StateIdle has no data call.
	StateIdle State = iota
	// StateCall has a data call, between packets.
	StateCall
	// StateReceiving has a packet that waits for blocks.
	StateReceiving
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s State) String() string {
	switch s {
	case StateIdle:
		return idleName
	case StateCall:
		return "Call"
	case StateReceiving:
		return "Receiving"
	}
	return unnamed("State", uint8(s))
}
