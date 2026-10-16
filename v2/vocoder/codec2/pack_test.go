package codec2

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestQRadioLinkBurstLayout rebuilds a voice burst the way QRadioLink does
// (DMRControl::getTxAudio and DMRFrame::setVoice) and checks that the three
// 9-byte frames land in the burst's voice bits in order.
func TestQRadioLinkBurstLayout(t *testing.T) {
	t.Parallel()

	codecFrames := [3][]byte{
		{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
		{0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC},
		{0xDD, 0xEE, 0xFF, 0x01, 0x23, 0x45},
	}
	var audio [27]byte
	for i, f := range codecFrames {
		copy(audio[i*9:], f)
	}
	var burst [33]byte
	copy(burst[:14], audio[:14])
	burst[13] &= 0xF0
	burst[19] = audio[13] & 0x0F
	copy(burst[20:], audio[14:])

	for i, got := range layer2.RawBurst(burst).VoiceSlots() {
		var want [9]byte
		copy(want[:], codecFrames[i])
		if got != want {
			t.Errorf("frame %d: burst slot % X, want % X", i, got, want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      Config
		want    Config
		wantErr error
	}{
		{Config{}, Config{Mode: Mode2400, FID: DefaultFID}, nil},
		{Config{Mode: Mode3200}, Config{Mode: Mode3200, FID: DefaultFID}, nil},
		{Config{Mode: Mode2400, FID: 0x7E}, Config{Mode: Mode2400, FID: 0x7E}, nil},
		{Config{Mode: 1300}, Config{}, ErrUnsupportedMode},
		{Config{Mode: -1}, Config{}, ErrUnsupportedMode},
	}
	for _, tt := range tests {
		got, err := tt.in.withDefaults()
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("%+v: error = %v, want %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("%+v: got %+v, want %+v", tt.in, got, tt.want)
		}
	}
	if Mode2400.frameBytes() != 6 || Mode3200.frameBytes() != 8 {
		t.Errorf("frameBytes = %d, %d, want 6, 8", Mode2400.frameBytes(), Mode3200.frameBytes())
	}
}
