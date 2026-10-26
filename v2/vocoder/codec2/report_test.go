//go:build !nocodec2 && !tinygo

package codec2

import (
	"errors"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestDecodeReport(t *testing.T) {
	t.Parallel()

	good := vocoder.FrameReport{Frame: vocoder.FrameGood, Output: vocoder.OutputVoice, CountsKnown: true}
	invalid := vocoder.FrameReport{Frame: vocoder.FrameInvalid, Output: vocoder.OutputVoice, CountsKnown: true}
	pcm := voiced(6)
	for _, mode := range []Mode{Mode2400, Mode3200, ModeAuto} {
		enc := newCodec(t, Config{Mode: mode})
		dec := newCodec(t, Config{Mode: mode})
		nbytes := mode.frameBytes()
		for i, slot := range encodeAll(t, enc, pcm) {
			out, rep, err := dec.Decode(nil, slot)
			if err != nil || len(out) != FrameSamples || rep != good {
				t.Errorf("mode %d frame %d: Decode = %d samples, %+v, %v", mode, i, len(out), rep, err)
			}
			for b := nbytes; b < 9; b++ {
				bad := slot
				bad[b] |= 0x01
				out, rep, err := dec.Decode([]int16{3}, bad)
				if err != nil || len(out) != 1+FrameSamples || out[0] != 3 || rep != invalid {
					t.Errorf("mode %d frame %d, byte %d set: Decode = %d samples, %+v, %v", mode, i, b, len(out), rep, err)
				}
			}
		}
	}
}

func TestDecodeLost(t *testing.T) {
	t.Parallel()

	c := newCodec(t, Config{})
	slot, err := c.Encode(voiced(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Decode(nil, slot); err != nil {
		t.Fatal(err)
	}
	dst := make([]int16, 2, 2+FrameSamples)
	dst[0], dst[1] = 7, 8
	out, rep, err := c.DecodeLost(dst)
	if err != nil || len(out) != 2+FrameSamples || &out[0] != &dst[0] || out[0] != 7 || out[1] != 8 {
		t.Fatalf("DecodeLost = %d samples, %v", len(out), err)
	}
	if slices.ContainsFunc(out[2:], func(s int16) bool { return s != 0 }) {
		t.Error("DecodeLost did not output silence")
	}
	if rep != (vocoder.FrameReport{Frame: vocoder.FrameLost, Output: vocoder.OutputSilence}) {
		t.Errorf("DecodeLost report %+v", rep)
	}
	if _, rep, err := c.Decode(nil, slot); err != nil || rep.Frame != vocoder.FrameGood {
		t.Errorf("Decode after DecodeLost = %+v, %v", rep, err)
	}
	_ = c.Close()
	if got, rep, err := c.DecodeLost(dst); len(got) != len(dst) || rep != (vocoder.FrameReport{}) || !errors.Is(err, ErrClosed) {
		t.Errorf("DecodeLost after Close = %d samples, %+v, %v", len(got), rep, err)
	}
	if got, rep, err := c.Decode(dst, slot); len(got) != len(dst) || rep != (vocoder.FrameReport{}) || !errors.Is(err, ErrClosed) {
		t.Errorf("Decode after Close = %d samples, %+v, %v", len(got), rep, err)
	}
	var n *Codec
	if _, _, err := n.DecodeLost(nil); err == nil {
		t.Error("DecodeLost of a nil Codec returned no error")
	}
}
