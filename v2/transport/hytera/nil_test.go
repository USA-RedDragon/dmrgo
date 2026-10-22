package hytera_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var f *hytera.Frame
	if h := f.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil Frame Hint %+v", h)
	}
	if out, err := f.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Frame AppendBinary %x, %v", out, err)
	}
	if f.HasBurst() {
		t.Error("nil Frame HasBurst")
	}
	var h *hytera.Heartbeat
	if out, err := h.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Heartbeat AppendBinary %x, %v", out, err)
	}
	var c *hytera.P2P
	if out, err := c.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil P2P AppendBinary %x, %v", out, err)
	}
	if s, ok := c.Service(); ok || s != 0 {
		t.Errorf("nil P2P Service %v, %v", s, ok)
	}
	if port, ok := c.RedirectPort(); ok || port != 0 {
		t.Errorf("nil P2P RedirectPort %d, %v", port, ok)
	}
}
