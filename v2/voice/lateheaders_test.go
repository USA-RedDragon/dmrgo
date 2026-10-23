package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func relayStart(t *testing.T, headers int, origin voice.Origin) (n int, due time.Duration) {
	t.Helper()
	tx := newTransmitter(t, &voice.TransmitterConfig{Headers: headers})
	c := rcall()
	c.Origin = origin
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
		t.Fatal(err)
	}
	first := time.Duration(0)
	if origin == voice.OriginHeader {
		first = slot
	}
	for now := time.Duration(0); now <= 20*slot; now += time.Millisecond {
		if now >= first && (now-first)%slot == 0 {
			e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, int((now-first)/slot)%6), Position: enums.VoiceBurstA}
			if err := tx.Relay(now, &e); err != nil {
				t.Fatal(err)
			}
		}
		for _, o := range tx.Tick(now, nil) {
			switch {
			case o.Kind != voice.OutputBurst:
			case o.Hint.Kind == layer2.KindVoice:
				return n, o.Due
			case o.Hint.DataType == enums.DataTypeVoiceLCHeader:
				n++
			}
		}
	}
	t.Fatal("no voice relayed")
	return 0, 0
}

func TestRelayLateEntrySendsOneHeader(t *testing.T) {
	t.Parallel()
	for _, origin := range []voice.Origin{voice.OriginEmbedded, voice.OriginExpected, voice.OriginTerminator} {
		for _, h := range []int{0, 1, 3, voice.MaxHeaders} {
			n, due := relayStart(t, h, origin)
			if n != 1 || due != slot {
				t.Errorf("%v with Headers %d: %d headers, first voice due %v, want 1 and %v", origin, h, n, due, slot)
			}
		}
	}
	for _, h := range []int{0, 1, 3, voice.MaxHeaders} {
		n, due := relayStart(t, h, voice.OriginHeader)
		if want := max(h, 1); n != want || due != time.Duration(want)*slot {
			t.Errorf("header call with Headers %d: %d headers, first voice due %v, want %d and %v", h, n, due, want, time.Duration(want)*slot)
		}
	}
}
