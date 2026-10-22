package packet_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestAssemblerAddTypedNil(t *testing.T) {
	t.Parallel()

	for _, d := range []layer2.Data{(*pdu.DataHeader)(nil), (*pdu.Rate12Data)(nil), (*pdu.Rate34Data)(nil), (*pdu.Rate1Data)(nil)} {
		a := packet.NewAssembler(4)
		if _, done, err := a.Add(d); done || !errors.Is(err, dmrgo.ErrNilArgument) {
			t.Errorf("Add(%T(nil)) = %v, %v, want dmrgo.ErrNilArgument", d, done, err)
		}
	}
}
