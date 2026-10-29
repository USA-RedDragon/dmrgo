package trunk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestTypedNilData(t *testing.T) {
	t.Parallel()

	good, nils := fuzztest.TypedNilBursts(t)
	for _, r := range good {
		for _, n := range nils {
			f, err := trunk.NewFollower(nil)
			if err != nil {
				t.Fatal(err)
			}
			ms, err := trunk.NewMS(&trunk.MSConfig{Address: 1})
			if err != nil {
				t.Fatal(err)
			}
			c, err := trunk.NewCapacityPlusFollower(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range []*layer2.Burst{r, n} {
				f.Decoded(0, b, layer2.Hint{}, nil, nil)
				ms.Decoded(0, b, layer2.Hint{}, nil, nil)
				c.Decoded(0, b, layer2.Hint{}, nil, nil)
			}
		}
	}
}
