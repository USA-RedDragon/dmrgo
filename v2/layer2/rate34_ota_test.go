package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// rate34Sent is a Rate 3/4 data burst that an XPR 8400 repeater sent over the
// air on 2026-10-15.
const rate34Sent = "2c3c2a3cf259f24919d542edde1dff57d75df5d7d42efea77a2f9de704bf0f2c4e"

// TestRate34OverTheAir decodes two receptions of rate34Sent with symbol
// errors: one with +1 received as -3 (2 bits), one with +3 received as +1.
func TestRate34OverTheAir(t *testing.T) {
	t.Parallel()

	want, res := layer2.RawBurst(hexBurst(t, rate34Sent)).Rate34Payload()
	if res.ErrorsCorrected != 0 || res.Uncorrectable {
		t.Fatalf("sent burst: %+v", res)
	}
	for _, c := range []struct {
		rx     string
		errors int
	}{
		{"2c3c2a3ff259f24919d542edde1dff57d75df5d7d42efea77a2f9de704bf0f2c4e", 2},
		{"2c3c2a3cf259f24918d542edde1dff57d75df5d7d42efea77a2f9de704bf0f2c4e", 1},
	} {
		got, res := layer2.RawBurst(hexBurst(t, c.rx)).Rate34Payload()
		if got != want || res.ErrorsCorrected != c.errors || res.Uncorrectable {
			t.Errorf("%s: payload %x %+v, want %x with %d errors corrected", c.rx, got, res, want, c.errors)
		}
	}
}

// TestRate34SingleDibitErrors sets each information dibit of rate34Sent to
// each other value. A 1-step error always decodes to the sent payload. A 2
// or 3-step error can make the burst at least as close to another block,
// because two blocks can differ in only 2 dibits (see the trellis tests).
func TestRate34SingleDibitErrors(t *testing.T) {
	t.Parallel()

	sent := layer2.RawBurst(hexBurst(t, rate34Sent))
	want, _ := sent.Rate34Payload()
	level := [4]int{1, 3, -1, -3}
	var ok, flagged, other int
	for i := range 33 {
		if i >= 13 && i <= 19 {
			continue
		}
		for sh := 0; sh < 8; sh += 2 {
			for v := byte(1); v < 4; v++ {
				rx := sent
				rx[i] ^= v << sh
				got, res := rx.Rate34Payload()
				steps := level[sent[i]>>sh&3] - level[rx[i]>>sh&3]
				switch {
				case got == want && !res.Uncorrectable:
					ok++
				case steps == 2 || steps == -2:
					t.Errorf("byte %d mask %02x, a 1-step error: payload %x %+v, want %x", i, v<<sh, got, res, want)
				case res.Uncorrectable:
					flagged++
				default:
					other++
				}
			}
		}
	}
	t.Logf("of %d single dibit errors: %d corrected, %d flagged Uncorrectable, %d decoded to a block with a lower path metric", ok+flagged+other, ok, flagged, other)
}
