// Package c2mode finds the Codec2 mode, 2400 or 3200 bit/s, of the voice
// frames of a call from the padding of each 72-bit DMR vocoder frame: a
// 2400 bit/s frame fills 6 bytes, a 3200 bit/s frame 8.
package c2mode

import "math/bits"

// Decide is the score at which the frames identify a mode, Limit the most
// score that a run of frames builds up, and Quiet the most set bits in the
// first 6 bytes of a frame that does not count, such as silence.
const (
	Decide = 5
	Limit  = 8
	Quiet  = 5
)

// Detector scores the frames of one call. Its zero value has seen no
// frame.
type Detector struct {
	score int8
	high  bool
}

// Reset starts a new call.
func (d *Detector) Reset() { *d = Detector{} }

// Add scores the frame slot and reports whether the frames so far favor
// 3200 bit/s over 2400.
func (d *Detector) Add(slot *[9]byte) bool {
	head := bits.OnesCount32(uint32(slot[0])<<16|uint32(slot[1])<<8|uint32(slot[2])) +
		bits.OnesCount32(uint32(slot[3])<<16|uint32(slot[4])<<8|uint32(slot[5]))
	if head > Quiet {
		switch tail := bits.OnesCount16(uint16(slot[6])<<8 | uint16(slot[7])); {
		case tail <= 1:
			d.score = max(d.score-1, -Limit)
		case tail >= 3:
			d.score = min(d.score+1, Limit)
		}
	}
	if d.score != 0 {
		d.high = d.score > 0
	}
	return d.high
}

// Mode returns 3200 or 2400 once the frames since Reset identify the mode,
// which 5 clean frames do, and 0 before that.
func (d *Detector) Mode() int {
	switch {
	case d.score >= Decide:
		return 3200
	case d.score <= -Decide:
		return 2400
	default:
		return 0
	}
}
