package trunk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestNewTSCC_ConfigErrors(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	for name, edit := range map[string]func(c *trunk.TSCCConfig){
		"Channel CHNULL":       func(c *trunk.TSCCConfig) { c.Channel = trunk.ChannelNull },
		"Channel over 12 bits": func(c *trunk.TSCCConfig) { c.Channel = 5000 },
		"Channel absolute":     func(c *trunk.TSCCConfig) { c.Channel = trunk.ChannelAbsolute },
		"payload is Channel":   func(c *trunk.TSCCConfig) { c.Payload[1] = c.Channel },
		"payload repeats":      func(c *trunk.TSCCConfig) { c.Payload[1] = c.Payload[0] },
		"GrantRepeats":         func(c *trunk.TSCCConfig) { c.GrantRepeats = -1 },
		"ClearRepeats":         func(c *trunk.TSCCConfig) { c.ClearRepeats = -1 },
		"Lead":                 func(c *trunk.TSCCConfig) { c.Lead = -1 },
	} {
		cfg := tsccConfig()
		edit(cfg)
		if _, err := trunk.NewTSCC(cfg); !errors.As(err, &ce) {
			t.Errorf("%s: NewTSCC = %v, want a *dmrgo.ConfigError", name, err)
		}
	}
	if _, err := trunk.NewTSCC(tsccConfig()); err != nil {
		t.Errorf("NewTSCC = %v", err)
	}
	zero := tsccConfig()
	zero.PayloadLen = 0
	for _, c := range []*trunk.TSCCConfig{nil, zero} {
		if _, err := trunk.NewTSCC(c); !errors.As(err, &ce) || ce.Field != "PayloadLen" {
			t.Errorf("NewTSCC(%v) = %v, want the PayloadLen error", c == nil, err)
		}
	}
	if _, err := trunk.NewMS(nil); !errors.As(err, &ce) || ce.Field != "Address" {
		t.Errorf("NewMS(nil) = %v, want the Address error", err)
	}
}

func TestNewFollowerMS_ConfigErrors(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	for name, cfg := range map[string]*trunk.FollowerConfig{
		"talkgroup over 24 bits": {Talkgroups: [trunk.MaxTalkgroups]uint32{1, 0x1000000}, TalkgroupsLen: 2},
		"SysErrors":              {SysErrors: -1},
	} {
		if _, err := trunk.NewFollower(cfg); !errors.As(err, &ce) {
			t.Errorf("%s: NewFollower = %v, want a *dmrgo.ConfigError", name, err)
		}
	}
	for name, cfg := range map[string]*trunk.MSConfig{
		"Slot":                   {Address: 1, Slot: 7},
		"talkgroup over 24 bits": {Address: 1, Talkgroups: [trunk.MaxTalkgroups]uint32{0x1000000}, TalkgroupsLen: 1},
	} {
		if _, err := trunk.NewMS(cfg); !errors.As(err, &ce) {
			t.Errorf("%s: NewMS = %v, want a *dmrgo.ConfigError", name, err)
		}
	}
}
