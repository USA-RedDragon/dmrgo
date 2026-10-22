package dmrd_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
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
		{"Data.Size", appendErr((&dmrd.Data{Size: 1}).AppendBinary), dmrd.ErrLength},
		{"TalkerAlias.Size", appendErr((&dmrd.TalkerAlias{Size: 1}).AppendBinary), dmrd.ErrLength},
		{"Position.Size", appendErr((&dmrd.Position{Size: 1}).AppendBinary), dmrd.ErrLength},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}
