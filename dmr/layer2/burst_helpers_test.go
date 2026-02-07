package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
	layer3Elements "github.com/USA-RedDragon/dmrgo/dmr/layer3/elements"
)

func TestBurst_PackUnpackEmbeddedSignallingData_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data [32]byte
	}{
		{"AllZeros", [32]byte{}},
		{"AllOnes", func() [32]byte {
			var d [32]byte
			for i := range d {
				d[i] = 1
			}
			return d
		}()},
		{"Alternating", func() [32]byte {
			var d [32]byte
			for i := range d {
				d[i] = byte(i % 2)
			}
			return d
		}()},
		{"FirstHalfSet", func() [32]byte {
			var d [32]byte
			for i := 0; i < 16; i++ {
				d[i] = 1
			}
			return d
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			burst := &layer2.Burst{}
			burst.EmbeddedSignallingData = tt.data

			packed := burst.PackEmbeddedSignallingData()
			burst2 := &layer2.Burst{}
			burst2.UnpackEmbeddedSignallingData(packed[:])

			for i := 0; i < 32; i++ {
				if burst2.EmbeddedSignallingData[i] != tt.data[i] {
					t.Errorf("bit %d: got %d, want %d", i, burst2.EmbeddedSignallingData[i], tt.data[i])
				}
			}
		})
	}
}

func TestBurst_UnpackEmbeddedSignallingData_EmptySlice(t *testing.T) {
	burst := &layer2.Burst{}
	burst.EmbeddedSignallingData[0] = 1
	burst.EmbeddedSignallingData[5] = 1

	burst.UnpackEmbeddedSignallingData([]byte{})
	for i := 0; i < 32; i++ {
		if burst.EmbeddedSignallingData[i] != 0 {
			t.Errorf("bit %d should be 0 after unpacking empty data, got %d", i, burst.EmbeddedSignallingData[i])
		}
	}
}

func TestBurst_UnpackEmbeddedSignallingData_SingleByte(t *testing.T) {
	burst := &layer2.Burst{}
	burst.UnpackEmbeddedSignallingData([]byte{0xA5}) // 10100101

	expected := [8]byte{1, 0, 1, 0, 0, 1, 0, 1}
	for i := 0; i < 8; i++ {
		if burst.EmbeddedSignallingData[i] != expected[i] {
			t.Errorf("bit %d: got %d, want %d", i, burst.EmbeddedSignallingData[i], expected[i])
		}
	}
	for i := 8; i < 32; i++ {
		if burst.EmbeddedSignallingData[i] != 0 {
			t.Errorf("bit %d should be 0, got %d", i, burst.EmbeddedSignallingData[i])
		}
	}
}

func TestBurst_PackEmbeddedSignallingData_KnownValue(t *testing.T) {
	burst := &layer2.Burst{}
	for i := 0; i < 8; i++ {
		burst.EmbeddedSignallingData[i] = 1
	}

	packed := burst.PackEmbeddedSignallingData()
	if packed[0] != 0xFF {
		t.Errorf("packed[0] = 0x%02X, want 0xFF", packed[0])
	}
	for i := 1; i < 4; i++ {
		if packed[i] != 0x00 {
			t.Errorf("packed[%d] = 0x%02X, want 0x00", i, packed[i])
		}
	}
}

func TestBuildLCDataBurst_ProducesCorrectSize(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		ServiceOptions: layer3Elements.ServiceOptions{
			PriorityLevel: 0,
		},
		GroupAddress:  9990,
		SourceAddress: 3120101,
	}

	encoded, err := flc.Encode()
	if err != nil {
		t.Fatalf("FLC Encode failed: %v", err)
	}

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	result := layer2.BuildLCDataBurst(lcBytes, elements.DataTypeVoiceLCHeader, 1)
	if len(result) != 33 {
		t.Errorf("BuildLCDataBurst returned %d bytes, want 33", len(result))
	}

	burst := layer2.NewBurstFromBytes(result)
	if burst.SyncPattern != enums.BsSourcedData {
		t.Errorf("SyncPattern = %v, want BsSourcedData", burst.SyncPattern)
	}
}

func TestBuildLCDataBurst_Terminator(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.StandardizedFID,
		GroupAddress:  1,
		SourceAddress: 2,
	}

	encoded, err := flc.Encode()
	if err != nil {
		t.Fatalf("FLC Encode failed: %v", err)
	}

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	result := layer2.BuildLCDataBurst(lcBytes, elements.DataTypeTerminatorWithLC, 0)
	burst := layer2.NewBurstFromBytes(result)
	if burst.SyncPattern != enums.BsSourcedData {
		t.Errorf("SyncPattern = %v, want BsSourcedData", burst.SyncPattern)
	}
}

func TestBuildLCDataBurst_ColorCodeRange(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.StandardizedFID,
		GroupAddress:  100,
		SourceAddress: 200,
	}

	encoded, err := flc.Encode()
	if err != nil {
		t.Fatalf("FLC Encode failed: %v", err)
	}

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	for cc := uint8(0); cc < 16; cc++ {
		result := layer2.BuildLCDataBurst(lcBytes, elements.DataTypeVoiceLCHeader, cc)
		burst := layer2.NewBurstFromBytes(result)
		if !burst.HasSlotType {
			t.Errorf("cc=%d: burst has no slot type", cc)
			continue
		}
		if burst.SlotType.ColorCode != int(cc) {
			t.Errorf("cc=%d: SlotType.ColorCode = %d", cc, burst.SlotType.ColorCode)
		}
	}
}
