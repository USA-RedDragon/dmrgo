package hytera_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	if _, err := (&hytera.Frame{Slot: enums.Slot(3)}).AppendBinary(nil); !errors.Is(err, hytera.ErrCode) {
		t.Errorf("Frame.Slot 3: %v, want ErrCode", err)
	}
	var fre *dmrgo.FieldRangeError
	if _, err := (&hytera.Frame{Slot: enums.Slot1, ColorCode: 16}).AppendBinary(nil); !errors.As(err, &fre) {
		t.Errorf("Frame.ColorCode 16: %v, want a *dmrgo.FieldRangeError", err)
	}
	if err := (&hytera.Frame{}).SetHint(layer2.Hint{Kind: layer2.KindUnknown}); !errors.Is(err, layer2.ErrKind) {
		t.Errorf("SetHint KindUnknown: %v, want layer2.ErrKind", err)
	}
}
