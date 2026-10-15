package lowbit

import (
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestMask(t *testing.T) {
	t.Parallel()

	b := []bit.Bit{0xFF, 2, 3, 0x30, 7, 0xFE, 1, 0}
	Mask(b)
	if want := []bit.Bit{1, 0, 1, 0, 1, 0, 1, 0}; !slices.Equal(b, want) {
		t.Fatalf("Mask = %v, want %v", b, want)
	}
}
