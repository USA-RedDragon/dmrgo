//go:build !tinygo

package rf

import (
	"errors"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

func TestAudioHighpass(t *testing.T) {
	t.Parallel()
	b := bsBursts(t, "voice")
	run := func(tier Tier, bits, hp int) ([]int16, error) {
		a, err := NewAudioTransmitter(&AudioTxConfig{Tier: tier, Rate: 48000, Bits: bits, Region: RegionUS, Service: ServiceLandMobile, Power: 40, InputHighpassHz: hp})
		if err != nil {
			return nil, err
		}
		return a.AppendFlush(a.AppendSamples(nil, b)), nil
	}
	for _, c := range []struct {
		tier Tier
		hp   int
	}{{Tier0, 10}, {Tier1, -1}, {Tier1, 101}} {
		var ce *dmrgo.ConfigError
		if _, err := run(c.tier, 16, c.hp); !errors.As(err, &ce) || ce.Field != "AudioTxConfig.InputHighpassHz" {
			t.Errorf("tier %d, corner %d: %v, want a ConfigError for InputHighpassHz", c.tier-Tier0, c.hp, err)
		}
	}
	off, err := run(Tier1, 16, 0)
	if err != nil {
		t.Fatal(err)
	}
	on, err := run(Tier1, 16, 10)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(off, on) || len(off) != len(on) {
		t.Error("a stated 10 Hz high pass at 16 bits does not change the signal")
	}
	off12, _ := run(Tier1, 12, 0)
	on12, err := run(Tier1, 12, 10)
	if err != nil || !slices.Equal(off12, on12) {
		t.Errorf("a stated high pass at 12 bits, with no room above the measured level, changed the signal: %v", err)
	}
	for _, v := range on12 {
		if v < -2048 || v > 2047 {
			t.Fatalf("code %d outside 12 bits", v)
		}
	}
}
