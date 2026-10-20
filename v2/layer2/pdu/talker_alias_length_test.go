package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestTalkerAliasNegativeLength(t *testing.T) {
	t.Parallel()

	got, err := pdu.TalkerAlias(&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: -1})
	if err != nil || got != "" {
		t.Errorf("TalkerAlias = %q, %v; want empty", got, err)
	}
}
