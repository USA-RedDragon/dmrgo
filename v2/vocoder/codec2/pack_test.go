package codec2

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func bitsFromString(t *testing.T, s string) [72]bit.Bit {
	t.Helper()
	var out [72]bit.Bit
	n := 0
	for _, r := range s {
		switch r {
		case '0', '1':
			out[n] = bit.Bit(r - '0')
			n++
		case ' ':
		default:
			t.Fatalf("bad bit %q", r)
		}
	}
	if n != 72 {
		t.Fatalf("got %d bits, want 72", n)
	}
	return out
}

func TestPackSlotLayout(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		want  string
	}{
		{
			name:  "2400",
			frame: []byte{0xA5, 0x01, 0x80, 0xFF, 0x00, 0x3C},
			want: "10100101 00000001 10000000 11111111 00000000 00111100 " +
				"00000000 00000000 00000000",
		},
		{
			name:  "3200",
			frame: []byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF1},
			want: "00010010 00110100 01010110 01111000 10011010 10111100 " +
				"11011110 11110001 00000000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := bitsFromString(t, tt.want)
			got := packSlot(tt.frame)
			if got != want {
				t.Fatalf("packSlot = %v, want %v", got, want)
			}
			back := unpackSlot(got, len(tt.frame))
			if string(back) != string(tt.frame) {
				t.Fatalf("unpackSlot = %x, want %x", back, tt.frame)
			}
		})
	}
}

func TestUnpackSlotIgnoresPadBits(t *testing.T) {
	frame := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02}
	slot := packSlot(frame)
	for i := 48; i < 72; i++ {
		slot[i] = 1
	}
	if got := unpackSlot(slot, 6); string(got) != string(frame) {
		t.Fatalf("unpackSlot = %x, want %x", got, frame)
	}
}

// TestQRadioLinkBurstLayout rebuilds a voice burst the way QRadioLink does
// (DMRControl::getTxAudio and DMRFrame::setVoice) and checks that the three
// 9-byte frames land in the burst's voice bits in order.
func TestQRadioLinkBurstLayout(t *testing.T) {
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

	bits := bit.UnpackBytesToBits264(burst)
	var voice [216]bit.Bit
	copy(voice[:108], bits[:108])
	copy(voice[108:], bits[156:])
	for i, f := range codecFrames {
		var got [72]bit.Bit
		copy(got[:], voice[i*72:(i+1)*72])
		if want := packSlot(f); got != want {
			t.Errorf("frame %d: burst bits %v, want %v", i, got, want)
		}
		if back := unpackSlot(got, 6); string(back) != string(f) {
			t.Errorf("frame %d: unpacked %x, want %x", i, back, f)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
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
