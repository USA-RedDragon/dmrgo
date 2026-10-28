package vocoder

import (
	"io"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// Codec converts between 8 kHz 16-bit mono PCM and the 72 raw on-air bits of
// one of the three vocoder frames of a DMR voice burst, packed, the first bit
// in the most significant bit of byte 0, as layer2.RawBurst.VoiceSlots
// returns them. The implementation applies any FEC, scrambling or
// interleaving that the vocoder needs. A Codec is not safe for concurrent use
// unless its documentation states it. Close releases its resources.
// (ETSI TS 102 361-1 clause 6.1)
type Codec interface {
	io.Closer

	// FrameSamples returns the number of PCM samples in one frame.
	FrameSamples() int
	// Encode encodes exactly FrameSamples samples into one 72-bit frame.
	Encode(pcm []int16) ([9]byte, error)
	// Decode decodes one 72-bit frame, appends its FrameSamples samples to
	// dst and returns the extended slice and a report on the frame. A
	// frame that the codec finds erased or invalid, and replaces, is
	// not an error: the report shows it. Decode returns an error only for
	// misuse or a failure of the codec, such as a closed Codec, and then
	// returns dst unchanged. It allocates only when dst has no room for
	// FrameSamples more samples.
	Decode(dst []int16, slot [9]byte) ([]int16, FrameReport, error)
	// DecodeLost appends FrameSamples samples in place of a frame that
	// never arrived, such as a repeat of the frame before or silence, and
	// returns the extended slice and a report with Frame FrameLost. It
	// returns errors as Decode does. It allocates only when dst has no room
	// for FrameSamples more samples.
	DecodeLost(dst []int16) ([]int16, FrameReport, error)
	// Reset starts a new call: the decoder drops the state that it got from
	// the frames of the call before, such as a detected mode. A Codec without
	// such state does nothing.
	Reset()
	// Silence returns a frame that decodes to silence, for padding the last
	// burst of a call.
	Silence() [9]byte
	// FID returns the Feature set ID to send in the Full LC of calls that use
	// this codec. The FID does not identify the vocoder, so a receiver selects
	// the codec by a name that both ends set out of band (ETSI TS 102
	// 361-1 Table 9.21 NOTE).
	FID() enums.FID
}
