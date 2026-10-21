package enums

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

func TestNewColorCode(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		cc, err := NewColorCode(uint8(v))
		if v <= 15 {
			if err != nil || cc != ColorCode(v) {
				t.Errorf("NewColorCode(%d) = %d, %v", v, cc, err)
			}
			continue
		}
		var rangeErr *dmrgo.FieldRangeError
		if !errors.As(err, &rangeErr) || rangeErr.Type != "enums" || rangeErr.Field != "ColorCode" || rangeErr.Bits != 4 {
			t.Errorf("NewColorCode(%d) error = %v, want a *dmrgo.FieldRangeError", v, err)
		}
	}
	if _, err := NewColorCode(16); err.Error() != "enums.ColorCode: value does not fit in 4 bits" {
		t.Errorf("Error() = %q", err)
	}
}
