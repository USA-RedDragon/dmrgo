package enums

import (
	"errors"
	"testing"
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
		var ccErr *ColorCodeError
		if !errors.As(err, &ccErr) || ccErr.Value != uint8(v) {
			t.Errorf("NewColorCode(%d) error = %v, want a *ColorCodeError", v, err)
		}
	}
	if got := (&ColorCodeError{Value: 16}).Error(); got != "color code 16 does not fit in 4 bits" {
		t.Errorf("Error() = %q", got)
	}
}
