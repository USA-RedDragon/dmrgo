package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestNilArguments(t *testing.T) {
	t.Parallel()

	if _, err := layer2.EncodeEmbeddedFullLinkControl(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("EncodeEmbeddedFullLinkControl(nil) = %v, want dmrgo.ErrNilArgument", err)
	}
	if _, err := layer2.EncodeEmbeddedRC(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("EncodeEmbeddedRC(nil) = %v, want dmrgo.ErrNilArgument", err)
	}
}
