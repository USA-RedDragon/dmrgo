//go:build tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"

type txHost struct{}

const txHostBuilt = false

var errTxHost error = iqErrs.tier

func (*txHost) newIQ(Tier, int, float64) error { return errTxHost }

func (*txHost) newAudio(Tier, int, int, int) error { return errTxHost }

func (*txHost) key(bool) {}

func (*txHost) iqS16(int8) []int16 { return nil }

func (*txHost) audio(int8) []int16 { return nil }

func audioPeak(Tier, int, int) (int, error) { return 0, errTxHost }

func measuredLevel(_ Tier, _, bits int) int {
	return fsktx.Setup{Tier: fsktx.Tier0, Bits: bits}.Tier0Level()
}

func newChunk(int) []byte { return nil }

func newChunk16(int) []int16 { return nil }
