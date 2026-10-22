package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestTalkerAliasOverlongLength(t *testing.T) {
	t.Parallel()

	got, err := pdu.TalkerAlias(&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: 255})
	if !errors.Is(err, pdu.ErrTalkerAliasIncomplete) || got != "" {
		t.Errorf("TalkerAlias = %q, %v; want ErrTalkerAliasIncomplete", got, err)
	}
}

func TestTalkerAliasNilHeader(t *testing.T) {
	t.Parallel()

	if _, err := pdu.TalkerAlias(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("TalkerAlias(nil) = %v, want ErrNilArgument", err)
	}
	dst := []byte("x")
	if got, err := pdu.AppendTalkerAlias(dst, nil); !errors.Is(err, dmrgo.ErrNilArgument) || string(got) != "x" {
		t.Errorf("AppendTalkerAlias(nil) = %q, %v, want ErrNilArgument", got, err)
	}
}
