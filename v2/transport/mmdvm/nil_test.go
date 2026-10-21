package mmdvm_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var d *mmdvm.DMRData
	if h := d.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil DMRData Hint %+v", h)
	}
	if out, err := d.AppendBinary([]byte{1}); !errors.Is(err, mmdvm.ErrCommand) || len(out) != 1 {
		t.Errorf("nil DMRData AppendBinary %x, %v", out, err)
	}
}
