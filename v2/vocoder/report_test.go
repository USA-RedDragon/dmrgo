package vocoder_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestFrameStatusString(t *testing.T) {
	t.Parallel()

	for s, want := range map[vocoder.FrameStatus]string{
		vocoder.FrameGood:    "Good",
		vocoder.FrameLost:    "Lost",
		vocoder.FrameErased:  "Erased",
		vocoder.FrameInvalid: "Invalid",
		4:                    "FrameStatus(4)",
	} {
		if got := s.String(); got != want {
			t.Errorf("FrameStatus %d: String = %q, want %q", uint8(s), got, want)
		}
	}
	if vocoder.FrameGood != 0 || vocoder.FrameLost != 1 || vocoder.FrameErased != 2 || vocoder.FrameInvalid != 3 {
		t.Error("FrameStatus values moved")
	}
}

func TestFrameOutputString(t *testing.T) {
	t.Parallel()

	for o, want := range map[vocoder.FrameOutput]string{
		vocoder.OutputVoice:   "Voice",
		vocoder.OutputTone:    "Tone",
		vocoder.OutputRepeat:  "Repeat",
		vocoder.OutputSilence: "Silence",
		200:                   "FrameOutput(200)",
	} {
		if got := o.String(); got != want {
			t.Errorf("FrameOutput %d: String = %q, want %q", uint8(o), got, want)
		}
	}
	if vocoder.OutputVoice != 0 || vocoder.OutputTone != 1 || vocoder.OutputRepeat != 2 || vocoder.OutputSilence != 3 {
		t.Error("FrameOutput values moved")
	}
}

func TestFrameReportZero(t *testing.T) {
	t.Parallel()

	var r vocoder.FrameReport
	if r.Frame != vocoder.FrameGood || r.Output != vocoder.OutputVoice || r.CountsKnown {
		t.Errorf("zero FrameReport = %+v", r)
	}
}
