//go:build tinygo

package quadraticresidue

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range qr1676Seeds() {
		checkQR1676(t, s.data, s.mask)
	}
}
