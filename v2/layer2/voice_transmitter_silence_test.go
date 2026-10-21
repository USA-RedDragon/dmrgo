package layer2_test

import (
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestVoiceTransmitter_ZeroSilenceIsAMBESilence(t *testing.T) {
	t.Parallel()

	zero := &layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1}
	set := &layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	got := transmit(t, must(layer2.NewVoiceTransmitter(zero)), 4)
	if want := transmit(t, must(layer2.NewVoiceTransmitter(set)), 4); !slices.Equal(got, want) {
		t.Error("a zero Silence pads with other bursts than vocoder.AMBESilenceSlot()")
	}
}
