package trunk_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestChannelPlan(t *testing.T) {
	t.Parallel()
	var p trunk.ChannelPlan
	if err := p.AddRule(trunk.ChannelRule{First: 1, Last: 10, RXHz: 450_000_000, StepHz: 25_000, TXOffsetHz: -5_000_000}); err != nil {
		t.Fatal(err)
	}
	if err := p.Add(trunk.Channel{Number: 3, RXHz: 460_000_000}); err != nil {
		t.Fatal(err)
	}
	if c, ok := p.Lookup(3); !ok || c.RXHz != 460_000_000 {
		t.Errorf("Lookup(3) = %+v, %v: an entry wins over a rule", c, ok)
	}
	if c, ok := p.Lookup(4); !ok || c.RXHz != 450_075_000 || c.TXHz != 445_075_000 {
		t.Errorf("Lookup(4) = %+v, %v", c, ok)
	}
	if _, ok := p.Lookup(11); ok {
		t.Error("Lookup(11): ok outside the rule")
	}
	cdef := layer3.CdefParms{Channel: 3, TXMHz: 455, TXKHz: 8, RXMHz: 450, RXKHz: 8}
	if err := p.AddCdef(&cdef); err != nil || p.Len() != 1 {
		t.Fatalf("AddCdef: %v, Len %d", err, p.Len())
	}
	if c, _ := p.Lookup(3); c.RXHz != 450_001_000 || c.TXHz != 455_001_000 {
		t.Errorf("Lookup(3) after AddCdef = %+v", c)
	}
	for _, n := range []uint16{trunk.ChannelNull, trunk.ChannelAbsolute, 0x1000} {
		if err := p.Add(trunk.Channel{Number: n}); !errors.Is(err, trunk.ErrChannelNumber) {
			t.Errorf("Add(%d) = %v, want ErrChannelNumber", n, err)
		}
	}
	if err := p.AddRule(trunk.ChannelRule{First: 5, Last: 4}); !errors.Is(err, trunk.ErrChannelNumber) {
		t.Errorf("AddRule(5..4) = %v", err)
	}
	for i := range trunk.MaxChannels - 1 {
		if err := p.Add(trunk.Channel{Number: uint16(100 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Add(trunk.Channel{Number: 4000}); !errors.Is(err, trunk.ErrPlanFull) {
		t.Errorf("Add to a full plan = %v", err)
	}
	for range trunk.MaxRules - 1 {
		_ = p.AddRule(trunk.ChannelRule{First: 1, Last: 1})
	}
	if err := p.AddRule(trunk.ChannelRule{First: 1, Last: 1}); !errors.Is(err, trunk.ErrPlanFull) {
		t.Errorf("AddRule to a full plan = %v", err)
	}
}

func TestHuntList(t *testing.T) {
	t.Parallel()
	var h trunk.HuntList
	for _, ch := range []uint16{5, 6, 7} {
		if err := h.Add(trunk.HuntEntry{Channel: ch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Add(trunk.HuntEntry{Channel: 6, ColorCode: 3, HasColorCode: true}); err != nil || h.Len() != 3 || h.At(1).ColorCode != 3 {
		t.Errorf("Add of a channel in the list: %v, %+v", err, h.At(1))
	}
	if !h.Withdraw(5) || h.Withdraw(5) || h.Len() != 2 || h.At(0).Channel != 6 || h.At(1).Channel != 7 {
		t.Errorf("Withdraw: len %d, %+v %+v", h.Len(), h.At(0), h.At(1))
	}
	if h.At(2) != (trunk.HuntEntry{}) || h.At(-1) != (trunk.HuntEntry{}) {
		t.Error("At out of range is not zero")
	}
	if err := h.Add(trunk.HuntEntry{}); !errors.Is(err, trunk.ErrChannelNumber) {
		t.Errorf("Add(CHNULL) = %v", err)
	}
}

func TestAccess_Applies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a         trunk.Access
		ms        uint32
		emergency bool
		want      bool
	}{
		{trunk.Access{}, 1234, false, true},
		{trunk.Access{Mask: 1, Address: 1}, 1235, false, true},
		{trunk.Access{Mask: 1, Address: 1}, 1234, false, false},
		{trunk.Access{Mask: 1, Address: 1}, 1234, true, true},
		{trunk.Access{Mask: 4, Address: 0x5}, 0x125, false, true},
		{trunk.Access{Mask: 24, Address: 1234}, 1234, false, true},
		{trunk.Access{Mask: 24, Address: 1234}, 1235, true, false},
		{trunk.Access{Mask: 24, Address: 0}, 0, true, false},
	}
	for _, tt := range tests {
		if got := tt.a.Applies(tt.ms, tt.emergency); got != tt.want {
			t.Errorf("%+v.Applies(%d, %v) = %v, want %v", tt.a, tt.ms, tt.emergency, got, tt.want)
		}
	}
}
