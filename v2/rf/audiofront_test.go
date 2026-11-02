package rf

import "testing"

func audioMSVoice(t *testing.T) ([]Burst, []int16) {
	t.Helper()
	b := msBursts(t, "voice")
	atx, err := newAudioTransmitter(usAudio(Tier1, 48000, 16), openGate)
	if err != nil {
		t.Fatal(err)
	}
	return b, atx.AppendFlush(atx.AppendSamples(nil, b))
}

func audioExact(t *testing.T, tier Tier, x []int16, want []Burst) int {
	t.Helper()
	arx, err := NewAudioReceiver(&AudioRxConfig{Tier: tier, Rate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range arx.AppendFlush(arx.AppendBursts(nil, x)) {
		for j := range want {
			if g.Data == want[j].Data {
				n++
				break
			}
		}
	}
	return n
}

func TestAudioFullScaleStart(t *testing.T) {
	t.Parallel()
	b, x := audioMSVoice(t)
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for lead := 80; lead <= 136; lead += 8 {
			y := append(make([]int16, lead), x...)
			y = append(y, make([]int16, 4800)...)
			if n := audioExact(t, tier, y, b); n < len(b)-1 {
				t.Errorf("Tier%d, lead %d samples: %d of %d bursts exact", tier-Tier0, lead, n, len(b))
			}
		}
	}
}

func TestAudioMSIdleNoise(t *testing.T) {
	t.Parallel()
	b, x := audioMSVoice(t)
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for lead := 0; lead <= 56; lead += 8 {
			y := append(make([]int16, lead), x...)
			y = append(y, make([]int16, 4800)...)
			s := uint64(12345)
			for i := range y {
				s = s*6364136223846793005 + 1442695040888963407
				y[i] += int16(int64(s>>33)%301 - 150)
			}
			if n := audioExact(t, tier, y, b); n < len(b)-1 {
				t.Errorf("Tier%d, lead %d samples, noise +-150: %d of %d bursts exact", tier-Tier0, lead, n, len(b))
			}
		}
	}
}
