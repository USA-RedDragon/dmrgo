package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TestTalkerAliasNegativeLength checks that a header with a negative length
// decodes to an empty alias instead of panicking.
func TestTalkerAliasNegativeLength(t *testing.T) {
	t.Parallel()

	got, err := pdu.DecodeTalkerAlias(&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: -1})
	if err != nil || got != "" {
		t.Errorf("DecodeTalkerAlias = %q, %v; want empty", got, err)
	}
}
