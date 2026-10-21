package dmrd_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var d *dmrd.Data
	if h := d.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil Data Hint %+v", h)
	}
	var b *dmrd.Bridge
	if out, err := b.AppendBinary([]byte{1}); !errors.Is(err, dmrd.ErrCommand) || len(out) != 1 {
		t.Errorf("nil Bridge AppendBinary %x, %v", out, err)
	}
}
