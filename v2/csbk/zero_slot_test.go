package csbk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
)

func TestZeroSlot(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"Responder":      func() error { _, err := csbk.NewResponder(&csbk.ResponderConfig{Address: 1}); return err }(),
		"Wakeup nil":     func() error { _, err := csbk.NewWakeup(nil); return err }(),
		"Wakeup zero":    func() error { _, err := csbk.NewWakeup(&csbk.WakeupConfig{}); return err }(),
		"Requester nil":  func() error { _, err := csbk.NewRequester(nil); return err }(),
		"Requester zero": func() error { _, err := csbk.NewRequester(&csbk.RequesterConfig{}); return err }(),
	} {
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != "Slot" {
			t.Errorf("%s: %v, want a ConfigError for Slot", name, err)
		}
	}
}
