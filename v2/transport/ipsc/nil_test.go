package ipsc_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var u *ipsc.User
	if h := u.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil User Hint %+v", h)
	}
	if out, err := u.AppendBinary([]byte{1}); err == nil || errors.Is(err, ipsc.ErrShort) || len(out) != 1 {
		t.Errorf("nil User AppendBinary %x, %v", out, err)
	}
	if u.IsVoice() {
		t.Error("nil User IsVoice")
	}
	var c *ipsc.Control
	if out, err := c.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Control AppendBinary %x, %v", out, err)
	}
	var l *ipsc.PeerList
	if n, e := l.Len(), l.Entry(0); n != 0 || e != (ipsc.PeerEntry{}) {
		t.Errorf("nil PeerList %d, %+v", n, e)
	}
	var r *ipsc.Raw
	if out := r.AppendBinary([]byte{1}); len(out) != 1 {
		t.Errorf("nil Raw AppendBinary %x", out)
	}
}
