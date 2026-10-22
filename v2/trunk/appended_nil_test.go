package trunk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAppended_NilNMEA(t *testing.T) {
	t.Parallel()

	var a trunk.Appended
	if err := a.SetNMEAShort(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("SetNMEAShort(nil) = %v, want dmrgo.ErrNilArgument", err)
	}
	if err := a.SetNMEALong(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("SetNMEALong(nil) = %v, want dmrgo.ErrNilArgument", err)
	}
}
