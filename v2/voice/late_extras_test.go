package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRegression_LateEntryExtrasAfterCallStart(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"N0CALL", "N0CALL Jacob McSwain"} {
		tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Queue: voice.MaxQueue, Access: enums.AccessImpolite})
		gps := pdu.FLCGPSInfo{Latitude: 32.75, Longitude: -97.33}
		if err := tx.Start(0, voice.Call{Group: true, Target: 91, Source: 3191868}, &voice.CallOptions{TalkerAlias: alias, GPS: &gps}); err != nil {
			t.Fatal(err)
		}
		for range 3 * 6 * 10 {
			if err := tx.Frame(vocoder.AMBESilence()); err != nil {
				t.Fatal(err)
			}
		}
		tx.Stop()
		var voiceBursts []layer2.RawBurst
		for _, o := range drain(t, tx, 200) {
			if o.Kind == voice.OutputBurst && o.Hint.Kind == layer2.KindVoice {
				voiceBursts = append(voiceBursts, o.Burst)
			}
		}
		for skip := 1; skip < 30; skip++ {
			rx := newReceiver(t, nil)
			events := feed(t, rx, timed(voiceBursts[skip:], 0, layer2.Hint{}), -1)
			started := false
			nEmb, nTA := 0, 0
			for i, e := range events {
				switch e.Kind {
				case voice.EventCallStart:
					started = true
				case voice.EventEmbedded, voice.EventTalkerAlias:
					if !started || e.Call.Origin == voice.OriginUnknown || e.Call.Target != 91 {
						t.Fatalf("alias %q skip %d: event %d %v before EventCallStart, call %+v: %s", alias, skip, i, e.Kind, e.Call, kinds(events))
					}
					if e.Kind == voice.EventEmbedded {
						nEmb++
					} else {
						nTA++
						if string(e.Text) != alias {
							t.Errorf("alias %q skip %d: talker alias %q", alias, skip, e.Text)
						}
					}
				case voice.EventVoice, voice.EventPIHeader, voice.EventCallEnd, voice.EventSessionStart, voice.EventSessionEnd:
				}
			}
			if !started || nTA == 0 && skip <= 6 {
				t.Errorf("alias %q skip %d: start %v, %d talker aliases: %s", alias, skip, started, nTA, kinds(events))
			}
			if want := (len(voiceBursts) - skip) / 6 / 2; nEmb < want {
				t.Errorf("alias %q skip %d: %d embedded LCs, want at least %d: %s", alias, skip, nEmb, want, kinds(events))
			}
		}
	}
}
