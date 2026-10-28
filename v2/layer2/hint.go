package layer2

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// Hint is the data of a transport about a burst besides its 264 bits, such
// as the fields of a HomeBrew DMRD, IPSC or MMDVM frame. A machine that
// takes a RawBurst with a Hint decodes the burst itself, and uses the Hint
// where the burst alone is not enough. The zero Hint means nothing is
// known, as for a burst from an RF receiver.
type Hint struct {
	// Slot is the TDMA channel, or enums.SlotUnknown.
	Slot enums.Slot
	// Kind is the burst kind from the transport, for example the DMRD frame
	// type. KindUnknown classifies the burst by its SYNC field.
	Kind Kind
	// VoiceBurst is the superframe position from the transport, for example
	// the DMRD voice sequence or an IPSC burst type. VoiceBurstUnknown means
	// not known.
	VoiceBurst enums.VoiceBurstType
	// DataType is the data type from the transport. HasDataType reports
	// whether it is set.
	DataType enums.DataType
	// HasDataType reports whether DataType is set.
	HasDataType bool
	// Seq is the transport sequence number, for example the RTP or DMRD
	// sequence number. HasSeq reports whether it is set.
	Seq uint16
	// HasSeq reports whether Seq is set.
	HasSeq bool
	// SeqBits is the width of Seq in bits, after which it wraps to 0. 0
	// means 16, as for an RTP sequence number. The DMRD sequence number of
	// HomeBrew has 8.
	SeqBits uint8
	// At is the time at which the packet arrived, on the clock of the
	// caller, as the now of the session machines. 0 means not known, as for
	// a replay without times. PositionTracker counts the 60 ms burst periods
	// since the burst before it to find the bursts lost after a sync loss, or
	// fewer than the missing sequence numbers.
	At time.Duration
	// Stream is the transport stream ID. HasStream reports whether it is
	// set.
	Stream uint32
	// HasStream reports whether Stream is set.
	HasStream bool
	// End is the end-of-call flag of the transport and how reliable it is.
	End EndHint
	// SyncLost reports that the transport lost the signal before this
	// burst, for example an MMDVM TAG_LOST or an IPSC "sync not detected"
	// burst.
	SyncLost bool
}

// EndHint is the end-of-call data that a transport gives.
type EndHint uint8

// The EndHint values.
const (
	// EndNone is a burst without an end flag.
	EndNone EndHint = iota
	// EndWeak is a flag that real equipment also sets in the middle of a
	// call, such as the IPSC 0x40 bit, which MOTOTRBO sets on weak voice
	// bursts. It never ends a call.
	EndWeak
	// EndStrong is a flag that marks only the end of a call.
	EndStrong
)

// String returns the name of the hint. It allocates only for a value without
// a name.
func (e EndHint) String() string {
	switch e {
	case EndNone:
		return "None"
	case EndWeak:
		return "Weak"
	case EndStrong:
		return "Strong"
	}
	return "EndHint(" + text.Int(int64(e)) + ")"
}
