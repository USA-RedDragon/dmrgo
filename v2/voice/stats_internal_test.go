package voice

import "testing"

func TestAddSat32(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		a    uint32
		n    int
		want uint32
	}{
		{65535, 1, 65536},
		{0xFFFFFFFE, 1, 0xFFFFFFFF},
		{0xFFFFFFFF, 1, 0xFFFFFFFF},
		{5, -10, 0},
	} {
		if got := addSat32(c.a, c.n); got != c.want {
			t.Errorf("addSat32(%d, %d) = %d, want %d", c.a, c.n, got, c.want)
		}
	}
}
