package rf_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func TestConstructorsNilConfig(t *testing.T) {
	t.Parallel()

	if _, err := rf.NewReceiver(nil); err != nil {
		t.Errorf("NewReceiver(nil): %v", err)
	}
	if _, err := rf.NewAudioReceiver(nil); err != nil {
		t.Errorf("NewAudioReceiver(nil): %v", err)
	}
	var ce *dmrgo.ConfigError
	if _, err := rf.NewReceiver(&rf.RxConfig{Rate: 1000}); !errors.As(err, &ce) || ce.Field != "RxConfig.Rate" {
		t.Errorf("NewReceiver at 1 kHz: %v", err)
	}
	if _, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Bits: 17}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.Bits" {
		t.Errorf("NewAudioReceiver with 17 bits: %v", err)
	}
	r, err := rf.NewReceiver(&rf.RxConfig{Tier: rf.Tier0, Rate: 9600, OnBursts: func([]rf.Burst) {}})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.Write(make([]byte, 4096)); n != 4096 || err != nil {
		t.Errorf("Write: %d, %v", n, err)
	}
	if err := r.Flush(); err != nil {
		t.Errorf("Flush: %v", err)
	}
}
