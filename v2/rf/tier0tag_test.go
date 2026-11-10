//go:build rftier0

package rf

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

func TestTier0TagRejectsHigherTiers(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	for _, tier := range []Tier{Tier1, Tier2, Tier3} {
		if _, err := NewReceiver(&RxConfig{Rate: 48000, Tier: tier}); !errors.As(err, &ce) {
			t.Errorf("NewReceiver tier %d: %v, want a ConfigError", tier, err)
		}
		if _, err := NewAudioReceiver(&AudioRxConfig{Tier: tier}); !errors.As(err, &ce) {
			t.Errorf("NewAudioReceiver tier %d: %v, want a ConfigError", tier, err)
		}
	}
	rx, err := NewReceiver(&RxConfig{Rate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	if rx.core().hasExt() {
		t.Error("the default tier attached the soft stages")
	}
	if _, err := NewAudioReceiver(nil); err != nil {
		t.Fatal(err)
	}
}
