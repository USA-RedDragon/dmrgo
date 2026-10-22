package mmdvm_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var d *mmdvm.DMRData
	if h := d.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil DMRData Hint %+v", h)
	}
	if out, err := d.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil DMRData AppendBinary %x, %v", out, err)
	}
	if err := d.SetHint(layer2.Hint{}); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil DMRData SetHint %v", err)
	}
	if _, err := mmdvm.NewDMRShortLC(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("NewDMRShortLC(nil) %v", err)
	}
	if c := d.Command(); c != 0 {
		t.Errorf("nil DMRData Command %v", c)
	}
	var v *mmdvm.Version
	if out, err := v.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Version AppendBinary %x, %v", out, err)
	}
	var g *mmdvm.Debug
	if out, err := g.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 || g.Count() != 0 {
		t.Errorf("nil Debug AppendBinary %x, %v", out, err)
	}
	var s *mmdvm.DMRShortLC
	if lc := s.ShortLC(); lc != (pdu.ShortLC{}) {
		t.Errorf("nil DMRShortLC ShortLC %+v", lc)
	}
}
