package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestVoiceServiceOptions_Table7_52(t *testing.T) {
	t.Parallel()
	tests := []struct {
		o    layer3.VoiceServiceOptions
		want uint8
	}{
		{layer3.VoiceServiceOptions{IsEmergency: true}, 0b100_0000},
		{layer3.VoiceServiceOptions{IsBroadcast: true}, 0b000_1000},
		{layer3.VoiceServiceOptions{PriorityLevel: 3}, 0b000_0011},
	}
	for _, tt := range tests {
		got, err := tt.o.Field()
		if err != nil || got != tt.want {
			t.Errorf("%+v.Field() = %07b, %v, want %07b", tt.o, got, err, tt.want)
		}
		if back := layer3.DecodeVoiceServiceOptions([1]byte{got << 1}); back != tt.o {
			t.Errorf("DecodeVoiceServiceOptions(%07b) = %+v, want %+v", got, back, tt.o)
		}
	}
	if _, err := (layer3.VoiceServiceOptions{PriorityLevel: 4}).Field(); err == nil {
		t.Error("Field: no error for PriorityLevel 4")
	}
}

func TestDataServiceOptions_Table7_53(t *testing.T) {
	t.Parallel()
	got, err := layer3.DataServiceOptions{HiRate: true, PriorityLevel: 2}.Field()
	if err != nil || got != 0b000_1010 {
		t.Errorf("Field() = %07b, %v, want 0001010", got, err)
	}
	if o := layer3.DecodeDataServiceOptions([1]byte{0b1000_1000}); !o.IsEmergency || !o.MultiItem || o.HiRate {
		t.Errorf("DecodeDataServiceOptions = %+v", o)
	}
}

func TestRegistrationServiceOptions_Table7_55(t *testing.T) {
	t.Parallel()
	got, err := layer3.RegistrationServiceOptions{Register: true}.Field()
	if err != nil || got != 0b000_0001 {
		t.Errorf("Field() = %07b, %v, want 0000001", got, err)
	}
	if o := layer3.DecodeRegistrationServiceOptions([1]byte{0b0000_0110}); o.PowerSave != 1 || !o.Register || o.IPInform {
		t.Errorf("DecodeRegistrationServiceOptions = %+v", o)
	}
}
