//go:build !nocodec2 && !tinygo

package codec2

import "github.com/USA-RedDragon/dmrgo/v2/vocoder"

func (m Mode) report(slot *[9]byte) vocoder.FrameReport {
	r := vocoder.FrameReport{Frame: vocoder.FrameGood, Output: vocoder.OutputVoice, CountsKnown: true}
	for i := m.frameBytes(); i < len(slot); i++ {
		if slot[i] != 0 {
			r.Frame = vocoder.FrameInvalid
		}
	}
	return r
}
