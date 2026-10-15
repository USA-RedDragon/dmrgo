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
	tests := []struct {
		name  string
		burst layer2.Burst
		want  string
	}{
		{
			name: "SlotTypeColorCode",
			burst: layer2.Burst{
				SyncPattern: enums.SyncBSSourcedData,
				IsData:      true,
				HasSlotType: true,
				SlotType:    pdu.SlotType{ColorCode: 16, DataType: enums.DataTypeIdle},
			},
			want: "SlotType.ColorCode",
		},
		{
			name: "EmbeddedColorCode",
			burst: layer2.Burst{
				SyncPattern:           enums.SyncEmbeddedSignalling,
				HasEmbeddedSignalling: true,
				EmbeddedSignalling:    pdu.EmbeddedSignalling{ColorCode: 16},
			},
			want: "EmbeddedSignalling.ColorCode",
		},
		{
			name: "EmbeddedReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.SyncEmbeddedSignalling,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
				ReverseChannel:        &pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "reverse channel",
		},
		{
			name: "StandaloneReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.SyncMSSourcedRC,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
				ReverseChannel:        &pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "reverse channel",
		},
		{
			name: "EmbeddedNilReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.SyncEmbeddedSignalling,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
			},
			want: "ReverseChannel is nil",
		},
		{
			name: "StandaloneNilReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.SyncMSSourcedRC,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
			},
			want: "ReverseChannel is nil",
		},
		{
			name: "DataWithoutSlotType",
			burst: layer2.Burst{
				SyncPattern: enums.SyncBSSourcedData,
				IsData:      true,
				SlotType:    pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeIdle},
				Data:        &pdu.PRFill{DataType: enums.DataTypeIdle},
			},
			want: "no slot type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.burst.Encode()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestBurstEncode_DoesNotModifyBurst(t *testing.T) {
	var stale [32]bit.Bit
	stale[3] = 1
	for _, sync := range []enums.SyncPattern{enums.SyncEmbeddedSignalling, enums.SyncMSSourcedRC} {
		t.Run(sync.String(), func(t *testing.T) {
			b := layer2.Burst{
				SyncPattern:            sync,
				HasEmbeddedSignalling:  true,
				EmbeddedSignalling:     pdu.EmbeddedSignalling{ColorCode: 1, PreemptionAndPowerControlIndicator: true},
				EmbeddedSignallingData: stale,
				HasReverseChannel:      true,
				ReverseChannel:         &pdu.ReverseChannel{RCCommand: enums.RCCommand(5)},
			}
			before := b
			frame := must(b.Encode())
			if b.EmbeddedSignallingData != before.EmbeddedSignallingData || b.ReverseChannel != before.ReverseChannel {
				t.Fatal("Encode modified the burst")
			}
			got := must(layer2.NewBurstFromBytes(frame))
			if !got.HasReverseChannel || got.ReverseChannel.RCCommand != 5 {
				t.Fatalf("decoded RC %+v, want command 5", got.ReverseChannel)
			}
			want := must(layer2.EncodeEmbeddedRC(b.ReverseChannel))
			if got.EmbeddedSignallingData != want {
				t.Fatal("embedded data is not the coded reverse channel")
			}
		})
	}
}

func TestBuildLCDataBurst_RejectsOutOfRangeColorCode(t *testing.T) {
	if _, err := layer2.BuildLCDataBurst([12]byte{}, enums.DataTypeVoiceLCHeader, 16); err == nil {
		t.Fatal("color code 16 should be rejected")
	}
	if _, err := layer2.BuildLCDataBurst([12]byte{}, enums.DataType(16), 1); err == nil {
		t.Fatal("data type 16 should be rejected")
	}
}

func TestBurstEncode_RejectsBurstWithoutSync(t *testing.T) {
	tests := []struct {
		name  string
		burst layer2.Burst
	}{
		{"ZeroValue", layer2.Burst{}},
		{"VoiceWithDataSync", layer2.Burst{SyncPattern: enums.SyncBSSourcedData}},
		{"DataWithVoiceSync", layer2.Burst{SyncPattern: enums.SyncMSSourcedVoice, IsData: true}},
		{"EmbeddedPatternWithoutEMB", layer2.Burst{SyncPattern: enums.SyncEmbeddedSignalling}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.burst.Encode(); err == nil {
				t.Fatalf("Encode = %x, want an error", got)
			}
		})
	}
}

// A voice burst encodes its vocoder frames from the SYNC pattern and the
// embedded signalling alone; VoiceBurst does not select the payload.
func TestBurstEncode_VoiceIgnoresVoiceBurst(t *testing.T) {
	var voice [3][72]bit.Bit
	for i := range voice {
		for j := range voice[i] {
			voice[i][j] = bit.Bit((i + j) & 1)
		}
	}
	for _, sync := range []enums.SyncPattern{enums.SyncBSSourcedVoice, enums.SyncReserved, enums.SyncEmbeddedSignalling} {
		t.Run(sync.String(), func(t *testing.T) {
			b := layer2.Burst{
				SyncPattern:           sync,
				HasEmbeddedSignalling: sync == enums.SyncEmbeddedSignalling,
				EmbeddedSignalling:    pdu.EmbeddedSignalling{ColorCode: 3, LCSS: enums.LCSSFirstFragment},
				VoiceSlots:            voice,
			}
			got, err := layer2.NewBurstFromBytes(must(b.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			if got.IsData || got.SyncPattern != sync {
				t.Fatalf("decoded IsData=%v SyncPattern=%s", got.IsData, got.SyncPattern.String())
			}
			if got.VoiceSlots != voice {
				t.Errorf("voice slots: got %v, want %v", got.VoiceSlots, voice)
			}
		})
	}
}
