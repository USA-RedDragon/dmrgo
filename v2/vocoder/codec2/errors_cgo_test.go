//go:build cgo && codec2native && !nocodec2 && !tinygo

package codec2

import (
	"errors"
	"testing"
)

func TestStateErrorsAreErrUnavailable(t *testing.T) {
	t.Parallel()

	if err := checkState(nil, Mode2400); !errors.Is(err, ErrUnavailable) {
		t.Errorf("checkState(nil) = %v, want ErrUnavailable", err)
	}
	if !errors.Is(errFrameSize, ErrUnavailable) {
		t.Errorf("errFrameSize = %v, want ErrUnavailable", errFrameSize)
	}
}

func TestNativeTagSelectsNativeBackend(t *testing.T) {
	t.Parallel()

	if Backend != "native" {
		t.Errorf("Backend = %q with -tags codec2native, want \"native\"", Backend)
	}
}
