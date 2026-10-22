package dmrd_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestNilReceivers(t *testing.T) {
	t.Parallel()

	var d *dmrd.Data
	if h := d.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil Data Hint %+v", h)
	}
	var b *dmrd.Bridge
	if out, err := b.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Bridge AppendBinary %x, %v", out, err)
	}
	var c *dmrd.Control
	if out, err := c.AppendBinary([]byte{1}, dmrd.DialectMMDVM); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil Control AppendBinary %x, %v", out, err)
	}
	var bc *dmrd.BridgeControl
	if out, err := bc.AppendBinary([]byte{1}); !errors.Is(err, dmrgo.ErrNilReceiver) || len(out) != 1 {
		t.Errorf("nil BridgeControl AppendBinary %x, %v", out, err)
	}
	var ta *dmrd.TalkerAlias
	if _, err := ta.LC(); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil TalkerAlias LC %v", err)
	}
	var lc pdu.FullLinkControl
	var pl pdu.FullLinkControlPayloads
	if err := ta.LCInto(&lc, &pl); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil TalkerAlias LCInto %v", err)
	}
	var lta *dmrd.LocalTalkerAlias
	if _, err := lta.LC(); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil LocalTalkerAlias LC %v", err)
	}
	var pos *dmrd.Position
	if err := pos.LCInto(&lc, &pl); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Position LCInto %v", err)
	}
	var lp *dmrd.LocalPosition
	if _, err := lp.LC(); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil LocalPosition LC %v", err)
	}
	if err := (&dmrd.Position{}).LCInto(nil, &pl); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Position LCInto nil dst %v", err)
	}
}
