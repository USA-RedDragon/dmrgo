package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// lcBurst builds a BS-sourced data burst that carries the BPTC(196,96)
// payload p with a slot type of color code cc and data type dt, which may be
// reserved.
func lcBurst(p [12]byte, dt enums.DataType, cc enums.ColorCode) ([33]byte, error) {
	var r layer2.RawBurst
	if err := r.SetSync(enums.SyncBSSourcedData); err != nil {
		return r, err
	}
	r.SetSlotType(pdu.SlotType{ColorCode: cc, DataType: dt})
	r.SetBPTCPayload(p)
	return r, nil
}

func TestBuildDataBurst_ProducesCorrectSize(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: layer3.ServiceOptions{
				PriorityLevel: 0,
			},
			GroupAddress:  9990,
			SourceAddress: 3120101,
		},
	}

	encBits := must(flc.Encode())
	encoded := bit.Pack(encBits[:])

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	result := [33]byte(must(layer2.BuildDataBurst(lcBytes, enums.DataTypeVoiceLCHeader, 1, layer2.SyncSourceBS)))
	if len(result) != 33 {
		t.Errorf("BuildDataBurst returned %d bytes, want 33", len(result))
	}

	burst, err := layer2.NewBurstFromBytes(result)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}
	if burst.SyncPattern != enums.SyncBSSourcedData {
		t.Errorf("SyncPattern = %v, want BsSourcedData", burst.SyncPattern)
	}
}

func TestBuildDataBurst_Terminator(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeTerminatorWithLC,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  1,
			SourceAddress: 2,
		},
	}

	encBits := must(flc.Encode())
	encoded := bit.Pack(encBits[:])

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	result := [33]byte(must(layer2.BuildDataBurst(lcBytes, enums.DataTypeTerminatorWithLC, 0, layer2.SyncSourceBS)))
	burst, err := layer2.NewBurstFromBytes(result)
	if err != nil {
		t.Fatalf("NewBurstFromBytes failed: %v", err)
	}
	if burst.SyncPattern != enums.SyncBSSourcedData {
		t.Errorf("SyncPattern = %v, want BsSourcedData", burst.SyncPattern)
	}
}

func TestBuildDataBurst_ColorCodeRange(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  100,
			SourceAddress: 200,
		},
	}

	encBits := must(flc.Encode())
	encoded := bit.Pack(encBits[:])

	var lcBytes [12]byte
	copy(lcBytes[:], encoded)

	for cc := enums.ColorCode(0); cc < 16; cc++ {
		result := [33]byte(must(layer2.BuildDataBurst(lcBytes, enums.DataTypeVoiceLCHeader, cc, layer2.SyncSourceBS)))
		burst, err := layer2.NewBurstFromBytes(result)
		if err != nil {
			t.Fatalf("NewBurstFromBytes failed for color code %d: %v", cc, err)
			continue
		}
		if burst.Kind != layer2.KindData {
			t.Errorf("cc=%d: burst has no slot type", cc)
			continue
		}
		if burst.SlotType.ColorCode != cc {
			t.Errorf("cc=%d: SlotType.ColorCode = %d", cc, burst.SlotType.ColorCode)
		}
	}
}
