package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestNilArguments(t *testing.T) {
	t.Parallel()

	if _, err := pdu.EncodeTalkerAliasInto(nil, "a", layer3.TalkerAliasSevenBit); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("EncodeTalkerAliasInto nil dst: %v", err)
	}
	if err := pdu.Seal(nil, enums.DataTypeCSBK); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Seal nil: %v", err)
	}
	var a pdu.TalkerAliasAssembler
	if _, _, err := a.AddInto(nil, &pdu.FullLinkControl{}); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("AddInto nil dst: %v", err)
	}
	var rc *pdu.ReverseChannel
	if _, err := rc.Encode(); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil ReverseChannel Encode: %v", err)
	}
}
