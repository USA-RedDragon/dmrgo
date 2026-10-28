package data_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
)

func TestSenderZeroSlot(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]*data.SenderConfig{"nil": nil, "zero": {}} {
		var ce *dmrgo.ConfigError
		if s, err := data.NewSender(cfg); !errors.As(err, &ce) || ce.Field != "Slot" || s != nil {
			t.Errorf("%s config: %v %v, want a ConfigError for Slot", name, s != nil, err)
		}
	}
}
