//go:build cgo && !nocodec2 && !tinygo && (codec2native || !codec2wasm)

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
