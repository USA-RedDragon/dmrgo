//go:build !tinygo

package privacy_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestAllocs(t *testing.T) {
	for _, c := range captures(t) {
		ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: c.key, Service: privacy.ServiceLandMobile})
		if err != nil {
			t.Fatal(err)
		}
		prx := privacy.NewReceiver()
		if err := prx.AddKey(c.key); err != nil {
			t.Fatal(err)
		}
		raw, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{c.pt[0], c.pt[1], c.pt[2]}, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		pi, _ := ptx.PIHeader(1)
		pib, _ := pi.Encode()
		var hdr layer2.RawBurst
		hdr.SetBPTCPayload(pib)
		start := voice.Event{Kind: voice.EventCallStart, Call: encryptedCall}
		head := voice.Event{Kind: voice.EventPIHeader, Burst: hdr}
		e := voice.Event{Kind: voice.EventVoice, Burst: raw}
		end := voice.Event{Kind: voice.EventCallEnd}
		o := voice.Output{Kind: voice.OutputBurst, Burst: raw, Hint: layer2.Hint{Kind: layer2.KindVoice}}
		var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
		mi := uint32(0)
		n := testing.AllocsPerRun(200, func() {
			mi++
			ptx.Start(mi)
			for range 40 {
				_ = ptx.Output(&o)
			}
			prx.Event(&start, nil)
			prx.Event(&head, nil)
			for range 40 {
				prx.Event(&e, &frames)
			}
			prx.Event(&end, nil)
		})
		if n != 0 {
			t.Errorf("%s: %v allocations", c.name, n)
		}
	}
}
