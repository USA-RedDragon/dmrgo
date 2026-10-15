package layer2_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
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
				SyncPattern: enums.BsSourcedData,
				IsData:      true,
				HasSlotType: true,
				SlotType:    pdu.SlotType{ColorCode: 16, DataType: elements.DataTypeIdle},
			},
			want: "SlotType.ColorCode",
		},
		{
			name: "EmbeddedColorCode",
			burst: layer2.Burst{
				SyncPattern:           enums.EmbeddedSignallingPattern,
				HasEmbeddedSignalling: true,
				EmbeddedSignalling:    pdu.EmbeddedSignalling{ColorCode: 16},
			},
			want: "EmbeddedSignalling.ColorCode",
		},
		{
			name: "EmbeddedReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.EmbeddedSignallingPattern,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
				ReverseChannel:        &pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "reverse channel",
		},
		{
			name: "StandaloneReverseChannel",
			burst: layer2.Burst{
				SyncPattern:           enums.MsSourcedRcSync,
				HasEmbeddedSignalling: true,
				HasReverseChannel:     true,
				ReverseChannel:        &pdu.ReverseChannel{RCCommand: enums.RCCommand(16)},
			},
			want: "reverse channel",
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

func TestBuildLCDataBurst_RejectsOutOfRangeColorCode(t *testing.T) {
	if _, err := layer2.BuildLCDataBurst([12]byte{}, elements.DataTypeVoiceLCHeader, 16); err == nil {
		t.Fatal("color code 16 should be rejected")
	}
	if _, err := layer2.BuildLCDataBurst([12]byte{}, elements.DataType(16), 1); err == nil {
		t.Fatal("data type 16 should be rejected")
	}
}
