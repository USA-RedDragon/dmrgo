package data_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestTypedNilData(t *testing.T) {
	t.Parallel()

	good, nils := fuzztest.TypedNilBursts(t)
	for _, r := range good {
		for _, n := range nils {
			rx, err := data.NewReceiver(nil)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range []*layer2.Burst{r, n} {
				rx.Decoded(0, b, layer2.Hint{}, nil)
				tx.Decoded(0, b, layer2.Hint{}, nil)
			}
		}
	}
}
