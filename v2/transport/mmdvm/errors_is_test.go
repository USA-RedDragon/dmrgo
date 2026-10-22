package mmdvm_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	appendErr := func(f func([]byte) ([]byte, error)) error {
		_, err := f(nil)
		return err
	}
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"StatusV1.Size", appendErr((&mmdvm.StatusV1{Size: 1}).AppendBinary), mmdvm.ErrLength},
		{"SetFreq.Size", appendErr((&mmdvm.SetFreq{Size: 1}).AppendBinary), mmdvm.ErrLength},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}
