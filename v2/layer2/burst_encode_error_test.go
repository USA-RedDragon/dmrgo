package layer2_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBurstEncode_ReturnsErrorForOutOfRangeFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		burst layer2.Burst
		want  string
	}{
		{
			name: "SlotTypeColorCode",
			burst: layer2.Burst{
				SyncPattern: enums.SyncBSSourcedData,
				Kind:        layer2.KindData,
				SlotType:    pdu.SlotType{ColorCode: 16, DataType: enums.DataTypeIdle},
			},
			want: "SlotType.ColorCode",
		},
		{
			name: "EmbeddedColorCode",
			burst: layer2.Burst{
				Kind:               layer2.KindVoice,
				SyncPattern:        enums.SyncNone,
				EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 16},
			},
			want: "EmbeddedSignalling.ColorCode",
		},
		{
			name: "EmbeddedReverseChannel",
			burst: layer2.Burst{
				Kind:               layer2.KindVoice,
				SyncPattern:        enums.SyncNone,
				EmbeddedSignalling: pdu.EmbeddedSignalling{PreemptionAndPowerControlIndicator: true},
				ReverseChannel:     pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "ReverseChannel.RCCommand",
		},
		{
			name: "StandaloneReverseChannel",
			burst: layer2.Burst{
				Kind:           layer2.KindReverseChannel,
				SyncPattern:    enums.SyncMSSourcedRC,
				ReverseChannel: pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "ReverseChannel.RCCommand",
		},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.burst.Encode()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestBurstEncode_DoesNotModifyBurst(t *testing.T) {
	t.Parallel()

	stale := [4]byte{0x10}
	for _, sync := range []enums.SyncPattern{enums.SyncNone, enums.SyncMSSourcedRC} {
		t.Run(sync.String(), func(t *testing.T) {
			t.Parallel()

			kind := layer2.KindVoice
			if sync == enums.SyncMSSourcedRC {
				kind = layer2.KindReverseChannel
			}
			b := layer2.Burst{
				Kind:                   kind,
				SyncPattern:            sync,
				EmbeddedSignalling:     pdu.EmbeddedSignalling{ColorCode: 1, PreemptionAndPowerControlIndicator: true},
				EmbeddedSignallingData: stale,
				ReverseChannel:         pdu.ReverseChannel{RCCommand: enums.RCCommand(5)},
			}
			data, rc := b.EmbeddedSignallingData, b.ReverseChannel
			frame := must(b.Encode())
			if b.EmbeddedSignallingData != data || b.ReverseChannel != rc {
				t.Fatal("Encode modified the burst")
			}
			got := must(layer2.NewBurstFromBytes(frame))
			if !got.HasReverseChannel() || got.ReverseChannel.RCCommand != 5 {
				t.Fatalf("decoded RC %+v, want command 5", got.ReverseChannel)
			}
			want := must(layer2.EncodeEmbeddedRC(&b.ReverseChannel))
			if got.EmbeddedSignallingData != want {
				t.Fatal("embedded data is not the coded reverse channel")
			}
		})
	}
}

func TestBurstEncode_RejectsBurstWithoutSync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		burst layer2.Burst
	}{
		{"ZeroValue", layer2.Burst{}},
		{"VoiceWithoutKind", layer2.Burst{SyncPattern: enums.SyncMSSourcedVoice}},
		{"VoiceWithDataSync", layer2.Burst{Kind: layer2.KindVoice, SyncPattern: enums.SyncBSSourcedData}},
		{"DataWithVoiceSync", layer2.Burst{Kind: layer2.KindData, SyncPattern: enums.SyncMSSourcedVoice}},
		{"DataWithRCSync", layer2.Burst{Kind: layer2.KindData, SyncPattern: enums.SyncMSSourcedRC}},
		{"DataWithoutSync", layer2.Burst{Kind: layer2.KindData, SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeIdle}, Data: &pdu.PRFill{}}},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, err := tt.burst.Encode(); err == nil {
				t.Fatalf("Encode = %x, want an error", got)
			}
		})
	}
}

// A voice burst encodes its vocoder frames from the SYNC pattern and the
// embedded signalling alone; VoiceBurst does not select the payload.
func TestBurstEncode_VoiceIgnoresVoiceBurst(t *testing.T) {
	t.Parallel()

	var voice [3][72]bit.Bit
	for i := range voice {
		for j := range voice[i] {
			voice[i][j] = bit.Bit((i + j) & 1)
		}
	}
	for _, sync := range []enums.SyncPattern{enums.SyncBSSourcedVoice, enums.SyncReserved, enums.SyncNone} {
		t.Run(sync.String(), func(t *testing.T) {
			t.Parallel()

			b := layer2.Burst{
				Kind:               layer2.KindVoice,
				SyncPattern:        sync,
				EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 3, LCSS: enums.LCSSFirstFragment},
				VoiceSlots:         voice,
			}
			got, err := layer2.NewBurstFromBytes(must(b.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != layer2.KindVoice || got.SyncPattern != sync {
				t.Fatalf("decoded Kind=%s SyncPattern=%s", got.Kind, got.SyncPattern.String())
			}
			if got.VoiceSlots != voice {
				t.Errorf("voice slots: got %v, want %v", got.VoiceSlots, voice)
			}
		})
	}
}
