package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestTSCCInactiveClearsRunningCall(t *testing.T) {
	t.Parallel()

	s := newServiceSite(&trunk.TSCCConfig{Inactive: 2 * time.Second}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	var ends []trunk.Reason
	var endAt time.Duration
	s.onTSCC = func(o *trunk.TSCCOutput) {
		if o.Kind == trunk.TSCCCallEnd {
			ends, endAt = append(ends, o.Reason), s.now
		}
	}
	s.run(3 * time.Second)
	if err := s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.run(time.Second)
	lc, _ := pdu.NewVoiceLC(true, 9, 3120001, layer3.ServiceOptions{})
	vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 3, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	raw := vt.Header()
	var out []trunk.TSCCOutput
	var last time.Duration
	for stop := s.now + 4*time.Second; s.now < stop; s.now += 60 * time.Millisecond {
		last = s.now
		out = s.tscc.Inbound(s.now, 10, &raw, layer2.Hint{Slot: s.grantSlot}, out[:0])
		for i := range out {
			s.onTSCC(&out[i])
		}
		out = s.tscc.Tick(s.now, out[:0])
		for i := range out {
			s.onTSCC(&out[i])
		}
	}
	if len(ends) != 0 {
		t.Fatalf("call with inbound bursts cleared: %v", ends)
	}
	s.run(3 * time.Second)
	if len(ends) != 1 || ends[0] != trunk.ReasonInactive {
		t.Fatalf("ends %v, want one Inactive", ends)
	}
	if d := endAt - last; d < 2*time.Second || d > 2500*time.Millisecond {
		t.Fatalf("cleared %v after the last inbound burst, want TV_Inactive and the P_CLEARs", d)
	}
}
