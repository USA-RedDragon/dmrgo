package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type relayTiming struct {
	emitted, due time.Duration
}

func markedBurst(t *testing.T, i int) layer2.RawBurst {
	t.Helper()
	b := vburst(t, 0)
	b.SetVoiceSlots([layer2.VoiceSlotsPerBurst][9]byte{{0xa0, byte(i)}, {0xa1, byte(i)}, {0xa2, byte(i)}})
	return b
}

func relayTimes(t *testing.T, cfg *voice.TransmitterConfig, arrive map[int]time.Duration, lost map[int]uint8, n int) map[int]relayTiming {
	t.Helper()
	tx := newTransmitter(t, cfg)
	c := rcall()
	lc := c.LC
	lc.SetFID(0xC2)
	c.LC, c.FID = lc, 0xC2
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
		t.Fatal(err)
	}
	got := map[int]relayTiming{}
	ms := time.Millisecond
	for now := time.Duration(0); now <= time.Duration(n+5)*slot; now += ms {
		for i := 1; i <= n; i++ {
			if at, ok := arrive[i]; ok && at == now {
				e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: markedBurst(t, i), Position: enums.VoiceBurstA, Lost: lost[i]}
				if err := tx.Relay(now, &e); err != nil {
					t.Fatal(err)
				}
			}
		}
		for {
			at, ok := tx.Deadline()
			if !ok || at > now {
				break
			}
			for _, o := range tx.Tick(now, nil) {
				if o.Kind != voice.OutputBurst || o.Hint.Kind != layer2.KindVoice {
					continue
				}
				if s := o.Burst.VoiceSlots(); s[0][0] == 0xa0 {
					got[int(s[0][1])] = relayTiming{emitted: now, due: o.Due}
				}
			}
		}
	}
	return got
}

func TestRelayWinsBackALateBurstAtTheNextLoss(t *testing.T) {
	t.Parallel()
	arrive, lost := map[int]time.Duration{}, map[int]uint8{13: 1}
	for i := 1; i <= 20; i++ {
		if i != 12 {
			arrive[i] = time.Duration(i) * slot
		}
	}
	arrive[5] += 2 * time.Millisecond
	got := relayTimes(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue}, arrive, lost, 20)
	for i := 1; i <= 20; i++ {
		r, ok := got[i]
		if i == 12 {
			continue
		}
		if !ok {
			t.Fatalf("burst %d not sent", i)
		}
		want := time.Duration(0)
		switch {
		case i == 5:
			want = 58 * time.Millisecond
		case i > 5 && i < 12:
			want = slot
		}
		if hold := r.emitted - arrive[i]; hold != want {
			t.Errorf("burst %d held %v, want %v", i, hold, want)
		}
	}
}

func TestRelayKeepsLead(t *testing.T) {
	t.Parallel()
	lead := 20 * time.Millisecond
	arrive := map[int]time.Duration{}
	for i := 1; i <= 10; i++ {
		arrive[i] = time.Duration(i) * slot
	}
	got := relayTimes(t, &voice.TransmitterConfig{Slot: enums.Slot1, Queue: voice.MaxQueue, Lead: lead}, arrive, nil, 10)
	for i := 1; i <= 10; i++ {
		r, ok := got[i]
		if !ok {
			t.Fatalf("burst %d not sent", i)
		}
		if r.emitted != arrive[i] || r.due != arrive[i]+lead {
			t.Errorf("burst %d: emitted %v and due %v after it came, want 0 and %v", i, r.emitted-arrive[i], r.due-arrive[i], lead)
		}
	}
}
