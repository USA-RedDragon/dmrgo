//go:build !tinygo

package fsktx

import "github.com/USA-RedDragon/dmrgo/v2/layer2"

type rxTier int

const (
	rxTier0 rxTier = iota + 1
	rxTier1
	rxTier2
	rxTier3
)

type RxBurst struct {
	Data, Fixed     layer2.RawBurst
	CACH, FixedCACH [3]byte
	HasFixed        bool
	HasCACH         bool
	Seconds         float64
}

func (b *RxBurst) Decoded() (layer2.RawBurst, [3]byte) {
	if b.HasFixed {
		return b.Fixed, b.FixedCACH
	}
	return b.Data, b.CACH
}

const (
	RxCS8 = iota
	RxCF32
)

var (
	RxAudio func(a []int16, hz, tier, chunk int) []RxBurst
	RxIQ    func(b []byte, rate, format, tier, chunk int) []RxBurst
)
