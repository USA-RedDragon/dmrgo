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
// unless its documentation says so. Close releases its resources.
// (ETSI TS 102 361-1 clause 6.1)
type Codec interface {
	io.Closer

	// FrameSamples returns the number of PCM samples in one frame.
	FrameSamples() int
	// Encode encodes exactly FrameSamples samples into one 72-bit frame.
	Encode(pcm []int16) ([9]byte, error)
	// Decode decodes one 72-bit frame into FrameSamples samples. It returns
	// at most FrameSamples() samples.
	Decode(slot [9]byte) ([]int16, error)
	// Silence returns a frame that decodes to silence, for padding the last
	// burst of a call.
	Silence() [9]byte
	// FID returns the Feature set ID to send in the Full LC of calls that use
	// this codec. The FID does not identify the vocoder, so a receiver selects
	// the codec by a name that both ends agree on out of band (ETSI TS 102
	// 361-1 Table 9.21 NOTE).
	FID() enums.FeatureSetID
}

// AppendDecoder is a Codec that can decode into a buffer of the caller.
// Match it with a type assertion on a Codec.
type AppendDecoder interface {
	Codec

	// AppendDecode decodes one 72-bit frame as Decode does, appends the
	// FrameSamples samples to dst and returns the extended slice; dst needs
	// room for FrameSamples() samples. On error it returns dst unchanged.
	AppendDecode(dst []int16, slot [9]byte) ([]int16, error)
}
