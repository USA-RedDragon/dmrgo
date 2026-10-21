package hytera_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var f *hytera.Frame
	if h := f.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil Frame Hint %+v", h)
	}
	if out, err := f.AppendBinary([]byte{1}); err == nil || errors.Is(err, hytera.ErrShort) || len(out) != 1 {
		t.Errorf("nil Frame AppendBinary %x, %v", out, err)
	}
}
