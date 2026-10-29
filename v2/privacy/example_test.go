package privacy_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// ExampleTransmitter sends a group call with Basic Privacy key 100 in the
// land mobile service and receives it with the same key. A transmitter in
// the amateur service gets ErrAmateur.
func ExampleTransmitter() {
	key := privacy.NewBasicKeyNumber(100)
	if _, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: key, Service: privacy.ServiceAmateur}); errors.Is(err, privacy.ErrAmateur) {
		fmt.Println("amateur:", err)
	}
	ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: key, Service: privacy.ServiceLandMobile})
	if err != nil {
		fmt.Println(err)
		return
	}
	prx := privacy.NewReceiver()
	if err := prx.AddKey(key); err != nil {
		fmt.Println(err)
		return
	}
	tx, _ := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS,
		Silence: vocoder.AMBESilenceSlot(), Access: enums.AccessImpolite, Queue: voice.MaxQueue})
	rx, _ := voice.NewReceiver(nil)

	call := voice.Call{Group: true, Target: 9, Source: 3191868, Options: layer3.ServiceOptions{IsPrivacy: true}, FID: enums.FIDMotorola}
	ptx.Start(0)
	if err := tx.Start(0, call, nil); err != nil {
		fmt.Println(err)
		return
	}
	sent := make([]vocoder.AMBEFrame, 18)
	for i := range sent {
		sent[i] = vocoder.AMBEFrame{byte(i), 0x5A, 0xA5, byte(3 * i), 0x0F, 0xF0, 0x80}
		_ = tx.Frame(sent[i])
	}
	tx.Stop(0)

	out := make([]voice.Output, 0, voice.MaxOutputs)
	events := make([]voice.Event, 0, voice.MaxEvents)
	var onAir, received []vocoder.AMBEFrame
	var info privacy.Info
	for now := time.Duration(0); tx.State() != voice.TransmitterIdle; now += 60 * time.Millisecond {
		for _, o := range tx.Tick(now, out[:0]) {
			if o.Kind != voice.OutputBurst {
				continue
			}
			if err := ptx.Output(&o); err != nil {
				fmt.Println(err)
				return
			}
			if o.Hint.Kind == layer2.KindVoice {
				frames, _ := o.Burst.AMBEFrames()
				onAir = append(onAir, frames[:]...)
			}
			for _, e := range rx.Burst(o.Due, &o.Burst, o.Hint, events[:0]) {
				var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
				info = prx.Event(&e, &frames)
				if e.Kind == voice.EventVoice {
					received = append(received, frames[:]...)
				}
			}
		}
	}
	fmt.Println("on air:", same(onAir, sent), "of", len(sent), "frames in the clear")
	fmt.Println("received:", same(received, sent), "of", len(sent), "frames in the clear")
	fmt.Println("encrypted", info.Encrypted, "PI header", info.Header, "decrypting", info.Decrypting)
	// Output:
	// amateur: privacy: the amateur service forbids encryption
	// on air: 0 of 18 frames in the clear
	// received: 18 of 18 frames in the clear
	// encrypted true PI header false decrypting true
}

func same(got, want []vocoder.AMBEFrame) int {
	n := 0
	for i := range min(len(got), len(want)) {
		if got[i] == want[i] {
			n++
		}
	}
	return n
}
