package mmdvm_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	appendErr := func(f func([]byte) ([]byte, error)) error {
		_, err := f(nil)
		return err
	}
	_, frame := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, make([]byte, mmdvm.MaxFrameLen))
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"StatusV1.Size", appendErr((&mmdvm.StatusV1{Size: 1}).AppendBinary), mmdvm.ErrLength},
		{"SetFreq.Size", appendErr((&mmdvm.SetFreq{Size: 1}).AppendBinary), mmdvm.ErrLength},
		{"AppendFrame", frame, mmdvm.ErrLength},
		{"Debug.Command", appendErr((&mmdvm.Debug{Command: mmdvm.CommandFMData}).AppendBinary), mmdvm.ErrCommand},
		{"SetHint voice", (&mmdvm.DMRData{}).SetHint(layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstUnknown}), layer2.ErrVoicePosition},
		{"SetHint data", (&mmdvm.DMRData{}).SetHint(layer2.Hint{Kind: layer2.KindData}), layer2.ErrDataType},
		{"SetHint kind", (&mmdvm.DMRData{}).SetHint(layer2.Hint{Kind: layer2.KindReverseChannel}), layer2.ErrKind},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}
