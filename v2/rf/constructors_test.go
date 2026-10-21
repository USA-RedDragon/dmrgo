package rf_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func TestConstructorsNilConfig(t *testing.T) {
	t.Parallel()

	if _, err := rf.New(nil); err != nil {
		t.Errorf("New(nil): %v", err)
	}
	if _, err := rf.NewDisc(nil); err != nil {
		t.Errorf("NewDisc(nil): %v", err)
	}
	var ce *dmrgo.ConfigError
	if _, err := rf.NewLowIQ(nil); !errors.As(err, &ce) || ce.Field != "LowIQConfig.OnBurst" {
		t.Errorf("NewLowIQ(nil): %v", err)
	}
	if _, err := rf.NewLowIQ(&rf.LowIQConfig{Hz: 1000, OnBurst: func(*rf.Burst) {}}); !errors.As(err, &ce) || ce.Field != "LowIQConfig.Hz" {
		t.Errorf("NewLowIQ at 1 kHz: %v", err)
	}
	r, err := rf.NewLowIQ(&rf.LowIQConfig{OnBurst: func(*rf.Burst) {}})
	if err != nil {
		t.Fatal(err)
	}
	r.WriteIQ16(make([]uint32, 4096))
	r.Flush()
}
