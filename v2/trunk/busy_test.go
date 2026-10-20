package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestTSCCManyBusyCarriers(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.PayloadLen = trunk.MaxPayloadChannels
	cfg.Hangtime, cfg.Inactive = time.Minute, time.Minute
	for i := range cfg.Payload {
		cfg.Payload[i] = uint16(10 + i)
	}
	ms := make([]*trunk.MSConfig, 0, 28)
	for i := range 28 {
		tg := [trunk.MaxTalkgroups]uint32{uint32(100 + i)}
		ms = append(ms, &trunk.MSConfig{Address: uint32(3120001 + i), Talkgroups: tg, TalkgroupsLen: 1, Seed: uint64(i + 1)})
	}
	s := newSim(t, cfg, ms...)
	s.until(30*time.Second, func() bool {
		for _, m := range s.ms {
			if !m.ms.Registered() {
				return false
			}
		}
		return true
	})
	most := 0
	for i, m := range s.ms {
		if err := m.ms.Call(s.now, true, uint32(100+i), layer3.VoiceServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		for end := s.now + time.Second; s.now < end; {
			s.step()
			most = max(most, len(s.tout))
		}
	}
	for end := s.now + 3*time.Second; s.now < end; {
		s.step()
		most = max(most, len(s.tout))
	}
	if most <= trunk.MaxOutputs {
		t.Errorf("at most %d outputs per Tick; the test needs more than trunk.MaxOutputs, %d", most, trunk.MaxOutputs)
	}
	t.Logf("most outputs of one Tick: %d of MaxOutputs() %d", most, s.tscc.MaxOutputs())
}
