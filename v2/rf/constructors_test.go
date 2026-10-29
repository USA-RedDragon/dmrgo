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

func TestLookbackBound(t *testing.T) {
	t.Parallel()

	for _, tier := range []rf.Tier{rf.TierDefault, rf.Tier1, rf.Tier2, rf.Tier3} {
		for _, n := range []int{rf.MaxLookbackSlots + 1, 1 << 30, int(^uint(0) >> 1)} {
			var ce *dmrgo.ConfigError
			if _, err := rf.NewReceiver(&rf.RxConfig{Tier: tier, LookbackSlots: n}); !errors.As(err, &ce) || ce.Field != "RxConfig.LookbackSlots" {
				t.Errorf("tier %d, LookbackSlots %d: %v", tier, n, err)
			}
		}
		if _, err := rf.NewReceiver(&rf.RxConfig{Tier: tier, LookbackSlots: rf.MaxLookbackSlots}); err != nil {
			t.Errorf("tier %d, LookbackSlots %d: %v", tier, rf.MaxLookbackSlots, err)
		}
	}
	var r rf.Receiver
	var ce *dmrgo.ConfigError
	if err := r.Init(rf.RxConfig{Rate: 9600, LookbackSlots: 1 << 30}); !errors.As(err, &ce) || ce.Field != "RxConfig.LookbackSlots" {
		t.Errorf("Init with LookbackSlots 1<<30: %v", err)
	}
}
