package ip_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

func TestIPv4OptionsLengthIsErrLength(t *testing.T) {
	t.Parallel()

	p := ip.IPv4{Header: ip.IPv4Header{Version: 4, IHL: 5}, Options: []byte{1, 2, 3, 4}}
	if _, err := p.AppendBinary(nil); !errors.Is(err, ip.ErrLength) {
		t.Errorf("AppendBinary = %v, want ErrLength", err)
	}
}
