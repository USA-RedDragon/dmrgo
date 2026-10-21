package ipsc_test

import (
	"errors"
	"testing"

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
}
