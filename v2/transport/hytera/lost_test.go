package hytera_test

import (
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestHintLostAfterHyteraHeaders(t *testing.T) {
	t.Parallel()

	types := []hytera.SlotType{hytera.SlotTypeVoiceLCHeader, hytera.SlotTypeVoiceLCHeader, hytera.SlotTypeVoiceLCHeader,
		hytera.SlotTypeVoiceA, hytera.SlotTypeVoiceB, hytera.SlotTypeVoiceC}
	for _, c := range []struct {
		name string
		keep []int
		want string
	}{
		{"three headers", []int{0, 1, 2, 3, 4, 5}, "A0B0C0"},
		{"two headers lost", []int{0, 3, 4, 5}, "A0B0C0"},
		{"two headers and A lost", []int{0, 4, 5}, "B1C0"},
	} {
		var tr layer2.PositionTracker
		got := ""
		for _, k := range c.keep {
			f := hytera.Frame{Seq: uint8(250 + k), Slot: enums.Slot1, SlotType: types[k]}
			h := f.Hint(0)
			lost, _ := tr.Place(&f.Burst, &h)
			if i, ok := h.VoiceBurst.Index(); ok {
				got += string("ABCDEF"[i]) + strconv.Itoa(lost)
			}
		}
		if got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
