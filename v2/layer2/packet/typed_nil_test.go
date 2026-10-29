package packet_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestAssembler_TypedNilData(t *testing.T) {
	t.Parallel()

	good, nils := fuzztest.TypedNilBursts(t)
	for _, r := range good {
		for _, n := range nils {
			a := packet.NewAssembler(16)
			_, _, _ = a.AddBurst(r)
			if _, done, err := a.AddBurst(n); done || err != nil {
				t.Errorf("AddBurst(%v) after %v: done %v, err %v", n.SlotType.DataType, r.SlotType.DataType, done, err)
			}
			_, _, _ = a.Add(r.Data)
			var want error
			switch n.Data.(type) {
			case *pdu.DataHeader, *pdu.Rate12Data, *pdu.Rate34Data, *pdu.Rate1Data:
				want = dmrgo.ErrNilArgument
			}
			if _, done, err := a.Add(n.Data); done || !errors.Is(err, want) {
				t.Errorf("Add(%T(nil)) after %v: done %v, err %v, want %v", n.Data, r.SlotType.DataType, done, err, want)
			}
		}
	}
}
