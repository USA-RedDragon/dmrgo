package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestTalkerAliasOverlongLength(t *testing.T) {
	t.Parallel()

	got, err := pdu.TalkerAlias(&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: 255})
	if !errors.Is(err, pdu.ErrTalkerAliasIncomplete) || got != "" {
		t.Errorf("TalkerAlias = %q, %v; want ErrTalkerAliasIncomplete", got, err)
	}
}
