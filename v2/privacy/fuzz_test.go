package privacy_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func FuzzReceiverEvents(f *testing.F) {
	f.Add([]byte{0x21, 0x10, 0x0F, 0x53, 0x16, 0x99, 0xC4, 0, 0, 1}, []byte{2, 0, 3, 255, 1})
	f.Add([]byte{0x25, 0x10, 0x0E, 5, 0xD3, 0x0B, 0xF2, 0, 0, 1}, []byte{2, 0, 0, 0, 7, 3})
	f.Fuzz(runScript)
}

func runScript(t *testing.T, head, script []byte) {
	aes, err := privacy.NewAESKey(0x0E, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	prx := privacy.NewReceiver()
	for _, k := range []*privacy.Key{aes, privacy.NewARC4Key(0x0F, [5]byte{1}), privacy.NewBasicKey(0x4562)} {
		if err := prx.AddKey(k); err != nil {
			t.Fatal(err)
		}
	}
	ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: aes, Service: privacy.ServiceLandMobile})
	if err != nil {
		t.Fatal(err)
	}
	var h pdu.PIHeader
	var p [10]byte
	copy(p[:], head)
	h.AlgorithmID, h.MFID, h.KeyID = p[0], enums.FID(p[1]), p[2]
	h.MessageIndicator = uint32(p[3])<<24 | uint32(p[4])<<16 | uint32(p[5])<<8 | uint32(p[6])
	h.Destination = uint32(p[7])<<16 | uint32(p[8])<<8 | uint32(p[9])
	pb, err := h.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var hdr layer2.RawBurst
	hdr.SetBPTCPayload(pb)
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	ptx.Start(h.MessageIndicator)
	for i, s := range script {
		var e voice.Event
		raw := layer2.RawBurst{byte(i), s, byte(i * 7)}
		switch s % 5 {
		case 0:
			e = voice.Event{Kind: voice.EventCallStart, Call: voice.Call{Options: layer3.ServiceOptions{IsPrivacy: s&8 != 0}}}
		case 1:
			e = voice.Event{Kind: voice.EventPIHeader, Burst: hdr}
		case 2:
			e = voice.Event{Kind: voice.EventVoice, Burst: raw, Lost: s, Position: enums.VoiceBurstType(s % 8)}
		case 3:
			e = voice.Event{Kind: voice.EventCallEnd}
		default:
			e = voice.Event{Kind: voice.EventKind(s), Burst: raw}
		}
		prx.Event(&e, &frames)
		o := voice.Output{Kind: voice.OutputBurst, Burst: raw, Hint: layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstType(s % 8)}}
		_ = ptx.Output(&o)
	}
}
