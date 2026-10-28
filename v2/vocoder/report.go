package vocoder

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// FrameStatus is what a Codec found in the slot of one vocoder frame.
type FrameStatus uint8

// The FrameStatus values.
const (
	// FrameGood is a frame that the codec decoded, after any FEC
	// correction.
	FrameGood FrameStatus = iota
	// FrameLost is a frame that never arrived. DecodeLost reports it.
	FrameLost
	// FrameErased is a frame that the codec received and did not use,
	// because it found too many bit errors in it.
	FrameErased
	// FrameInvalid is a slot whose bits are not a valid frame of the
	// vocoder, such as a frame with padding bits set.
	FrameInvalid
)

// String returns the name of the status. It allocates only for a value
// without a name.
func (s FrameStatus) String() string {
	switch s {
	case FrameGood:
		return "Good"
	case FrameLost:
		return "Lost"
	case FrameErased:
		return "Erased"
	case FrameInvalid:
		return "Invalid"
	}
	return "FrameStatus(" + text.Int(int64(s)) + ")"
}

// FrameOutput is what a Codec put in the samples of one vocoder frame.
type FrameOutput uint8

// The FrameOutput values.
const (
	// OutputVoice is speech that the codec decoded from the frame.
	OutputVoice FrameOutput = iota
	// OutputTone is a tone, such as a DTMF digit, that the codec decoded
	// from a tone frame.
	OutputTone
	// OutputRepeat is a repeat of the frame before, in place of a frame that
	// is lost or that the codec did not use.
	OutputRepeat
	// OutputSilence is silence or comfort noise. It is the decode of a
	// valid silence frame, with Frame FrameGood, or it replaces a frame that
	// is lost or that the codec did not use.
	OutputSilence
)

// String returns the name of the output. It allocates only for a value
// without a name.
func (o FrameOutput) String() string {
	switch o {
	case OutputVoice:
		return "Voice"
	case OutputTone:
		return "Tone"
	case OutputRepeat:
		return "Repeat"
	case OutputSilence:
		return "Silence"
	}
	return "FrameOutput(" + text.Int(int64(o)) + ")"
}

// FrameReport gives what a Codec found in one vocoder frame and what it put
// in the samples for it. The zero FrameReport is a good frame decoded to
// speech, with unknown bit counts.
type FrameReport struct {
	// Frame is what the codec found in the slot.
	Frame FrameStatus
	// Output is what the codec put in the samples.
	Output FrameOutput
	// CorrectedA is the number of bits, 0 to 4, that the Golay (24,12)
	// code of section A corrected.
	CorrectedA uint8
	// CorrectedB is the number of bits, 0 to 3, that the Golay (23,12)
	// code of section B corrected after the descrambling.
	CorrectedB uint8
	// HighErrorRate reports that the bit error rate that the codec
	// estimates from the recent frames is so high that it mutes or
	// replaces frames. It is false when the codec makes no such estimate.
	HighErrorRate bool
	// CountsKnown reports that the codec measured CorrectedA and
	// CorrectedB. When it is false both are 0 and unknown, not a count of 0
	// bits.
	CountsKnown bool
}
