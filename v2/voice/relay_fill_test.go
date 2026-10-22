package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRegression_RelayFillOtherCodec(t *testing.T) {
	t.Parallel()

	sil := vocoder.AMBESilenceSlot()
	for _, fid := range []enums.FID{enums.FIDStandardized, enums.FIDMotorola, 0xC2} {
		tx := newTransmitter(t, &voice.TransmitterConfig{Queue: voice.MaxQueue})
		c := rcall()
		lc := c.LC
		lc.SetFID(fid)
		c.LC, c.FID = lc, fid
		if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
			t.Fatal(err)
		}
		var out []voice.Output
		now := time.Duration(0)
		for i := 1; i <= 8; i++ {
			now = time.Duration(i) * slot
			if i != 3 && i != 6 {
				e := voice.Event{Kind: voice.EventVoice, Call: c, Burst: vburst(t, (i-1)%6), Position: enums.VoiceBurstA}
				if i == 7 {
					e.Lost = 3
				}
				if err := tx.Relay(now, &e); err != nil {
					t.Fatal(err)
				}
			}
			out = append(out, tx.Tick(now, nil)...)
		}
		_ = tx.Relay(now, &voice.Event{Kind: voice.EventCallEnd, Call: c, Reason: voice.EndTerminator})
		for range 40 {
			at, ok := tx.Deadline()
			if !ok {
				break
			}
			out = append(out, tx.Tick(at, nil)...)
		}
		ambe := 0
		for _, o := range out {
			if o.Kind != voice.OutputBurst || o.Hint.Kind != layer2.KindVoice {
				continue
			}
			for _, s := range o.Burst.VoiceSlots() {
				if s == sil {
					ambe++
				}
			}
		}
		std := fid == enums.FIDStandardized || fid == enums.FIDMotorola
		if std && ambe == 0 {
			t.Errorf("FID %v: no AMBE+2 silence in the fills", fid)
		}
		if !std && ambe != 0 {
			t.Errorf("FID %v: %d AMBE+2 silence slots in the fills of a call of another codec", fid, ambe)
		}
	}
}
