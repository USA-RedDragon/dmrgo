package voice_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestZeroSlot(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"Interrupter nil":  func() error { _, err := voice.NewInterrupter(nil); return err }(),
		"Interrupter zero": func() error { _, err := voice.NewInterrupter(&voice.InterrupterConfig{}); return err }(),
		"Transmitter nil":  func() error { _, err := voice.NewTransmitter(nil); return err }(),
		"Transmitter zero": func() error { _, err := voice.NewTransmitter(&voice.TransmitterConfig{}); return err }(),
	} {
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != "Slot" {
			t.Errorf("%s: %v, want a ConfigError for Slot", name, err)
		}
	}
}
