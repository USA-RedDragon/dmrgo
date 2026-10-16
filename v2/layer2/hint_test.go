package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestEndHintString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		hint layer2.EndHint
		want string
	}{
		{layer2.EndNone, "None"},
		{layer2.EndWeak, "Weak"},
		{layer2.EndStrong, "Strong"},
		{layer2.EndHint(7), "EndHint(7)"},
	}
	for _, tt := range tests {
		if got := tt.hint.String(); got != tt.want {
			t.Errorf("EndHint(%d).String() = %q, want %q", uint8(tt.hint), got, tt.want)
		}
	}
}

func TestHintZeroValue(t *testing.T) {
	t.Parallel()

	var h layer2.Hint
	if h.Slot.String() != "Unknown" || h.Kind != layer2.KindUnknown || h.End != layer2.EndNone ||
		h.HasDataType || h.HasSeq || h.HasStream || h.SyncLost {
		t.Errorf("zero Hint = %+v, want nothing known", h)
	}
}
