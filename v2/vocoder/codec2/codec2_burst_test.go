//go:build cgo && !nocodec2 && !tinygo

package codec2

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	burstTestGroup  = 91
	burstTestSource = 3191868
)

func transmit(t *testing.T, c *Codec, pcm []int16) []layer2.RawBurst {
	t.Helper()
	payload, err := (&pdu.FLCGroupVoice{GroupAddress: burstTestGroup, SourceAddress: burstTestSource}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	lc := pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           c.FID(),
		RawPayload:    payload,
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: c.Silence()})
	if err != nil {
		t.Fatal(err)
	}
	bursts := []layer2.RawBurst{tx.Header()}
	for _, slot := range encodeAll(t, c, pcm) {
		out, err := tx.AddSlots(slot)
		if err != nil {
			t.Fatal(err)
		}
		bursts = append(bursts, out...)
	}
	out, err := tx.Terminate()
	if err != nil {
		t.Fatal(err)
	}
	return append(bursts, out...)
}

func checkLC(t *testing.T, where string, lc *pdu.FullLinkControl, fid enums.FeatureSetID) {
	t.Helper()
	if lc.FID != fid {
		t.Errorf("%s: FID %#02x, want %#02x", where, uint8(lc.FID), uint8(fid))
	}
	target, source, ok := lc.Addresses()
	if !ok || target != burstTestGroup || source != burstTestSource {
		t.Errorf("%s: addresses %d/%d ok=%v, want %d/%d", where, target, source, ok, burstTestGroup, burstTestSource)
	}
}

func TestBurstRoundTrip(t *testing.T) {
	t.Parallel()

	pcm := voiced(50)
	for _, mode := range allModes() {
		want := encodeAll(t, newCodec(t, Config{Mode: mode}), pcm)
		c := newCodec(t, Config{Mode: mode})
		bursts := transmit(t, c, pcm)

		var rx layer2.VoiceReceiver
		var slots [][72]bit.Bit
		var headers, terminators, embedded int
		for i, raw := range bursts {
			b, err := layer2.NewBurstFromBytes(raw)
			if err != nil {
				t.Fatalf("mode %d burst %d: %v", mode, i, err)
			}
			if got, err := b.Encode(); err != nil || got != raw {
				t.Fatalf("mode %d burst %d: decode then Encode changed the burst (err %v)", mode, i, err)
			}
			if b.Kind == layer2.KindData {
				lc, ok := b.Data.(*pdu.FullLinkControl)
				if !ok || lc.FEC.Uncorrectable {
					t.Fatalf("mode %d burst %d: data burst is not a clean Full LC", mode, i)
				}
				if dt := b.SlotType.DataType; dt == enums.DataTypeVoiceLCHeader {
					headers++
				} else if dt == enums.DataTypeTerminatorWithLC {
					terminators++
				} else {
					t.Fatalf("mode %d burst %d: data type %v", mode, i, dt)
				}
				checkLC(t, "data burst", lc, c.FID())
				rx.Reset()
				continue
			}
			lc, ok, err := rx.Receive(b)
			if err != nil {
				t.Fatalf("mode %d burst %d: %v", mode, i, err)
			}
			if ok {
				embedded++
				checkLC(t, "embedded LC", &lc, c.FID())
			}
			for _, s := range b.VoiceSlots {
				slots = append(slots, s)
			}
		}

		if headers != 1 || terminators != 1 {
			t.Errorf("mode %d: %d headers and %d terminators, want 1 and 1", mode, headers, terminators)
		}
		if superframes := (len(slots) + 17) / 18; embedded != superframes {
			t.Errorf("mode %d: %d embedded LCs, want %d", mode, embedded, superframes)
		}
		if len(slots)%18 != 0 || len(slots) < len(want) {
			t.Fatalf("mode %d: %d slots for %d frames, want whole superframes", mode, len(slots), len(want))
		}
		for i, s := range slots {
			exp := c.Silence()
			if i < len(want) {
				exp = want[i]
			}
			if s != exp {
				t.Fatalf("mode %d: slot %d differs after the burst round trip", mode, i)
			}
		}

		out := decodeAll(t, c, slots[:len(want)])[800:]
		if ratio := rms(out) / rms(pcm[800:]); ratio < 0.5 || ratio > 2 {
			t.Errorf("mode %d: output/input RMS ratio %.2f", mode, ratio)
		}
		if lag := pitchLag(out); lag < 50 || lag > 57 {
			t.Errorf("mode %d: pitch lag %d samples, want about 53 (150 Hz)", mode, lag)
		}
	}
}
