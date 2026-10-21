package layer3

import (
	"errors"
	"testing"

	dmrgo "github.com/USA-RedDragon/dmrgo/v2"
)

func TestDecodeServiceOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		bits                [1]byte
		isEmergency         bool
		isPrivacy           bool
		isBroadcast         bool
		isOpenVoiceCallMode bool
		priorityLevel       uint8
	}{
		{
			name:                "AllZeros",
			bits:                [1]byte{0b00000000},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Emergency",
			bits:                [1]byte{0b10000000},
			isEmergency:         true,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Privacy",
			bits:                [1]byte{0b01000000},
			isEmergency:         false,
			isPrivacy:           true,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Broadcast",
			bits:                [1]byte{0b00001000},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         true,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "OpenVoiceCallMode",
			bits:                [1]byte{0b00000100},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: true,
			priorityLevel:       0,
		},
		{
			name:                "Priority2",
			bits:                [1]byte{0b00000010},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       2,
		},
		{
			name:                "Priority1",
			bits:                [1]byte{0b00000001},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       1,
		},
		{
			name:                "Priority3",
			bits:                [1]byte{0b00000011},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       3,
		},
		{
			name:                "AllSet",
			bits:                [1]byte{0b11111111},
			isEmergency:         true,
			isPrivacy:           true,
			isBroadcast:         true,
			isOpenVoiceCallMode: true,
			priorityLevel:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			so := DecodeServiceOptions(tt.bits)
			if so.IsEmergency != tt.isEmergency {
				t.Errorf("IsEmergency = %v, want %v", so.IsEmergency, tt.isEmergency)
			}
			if so.IsPrivacy != tt.isPrivacy {
				t.Errorf("IsPrivacy = %v, want %v", so.IsPrivacy, tt.isPrivacy)
			}
			if so.IsBroadcast != tt.isBroadcast {
				t.Errorf("IsBroadcast = %v, want %v", so.IsBroadcast, tt.isBroadcast)
			}
			if so.IsOpenVoiceCallMode != tt.isOpenVoiceCallMode {
				t.Errorf("IsOpenVoiceCallMode = %v, want %v", so.IsOpenVoiceCallMode, tt.isOpenVoiceCallMode)
			}
			if so.PriorityLevel != tt.priorityLevel {
				t.Errorf("PriorityLevel = %d, want %d", so.PriorityLevel, tt.priorityLevel)
			}
		})
	}
}

func TestDecodeServiceOptions_ReservedBits(t *testing.T) {
	t.Parallel()

	bits := [1]byte{0b00110000}
	so := DecodeServiceOptions(bits)
	if so.Reserved != 0b11 {
		t.Errorf("Reserved = %#b, want 0b11", so.Reserved)
	}
}

func TestServiceOptions_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	for i := 0; i < 256; i++ {
		b := byte(i)

		bits := [1]byte{b}

		so := DecodeServiceOptions(bits)
		encoded := must(so.Encode())
		if encoded != bits {
			t.Errorf("byte 0x%02X: decode then encode mismatch: got %v, want %v", b, encoded, bits)
		}
	}
}

func TestServiceOptions_String(t *testing.T) {
	t.Parallel()

	bits := [1]byte{0b10001010}
	so := DecodeServiceOptions(bits)
	str := so.String()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	if !containsStr(str, "IsEmergency: true") {
		t.Errorf("String() missing IsEmergency: true, got %q", str)
	}
	if !containsStr(str, "IsBroadcast: true") {
		t.Errorf("String() missing IsBroadcast: true, got %q", str)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestServiceOptionsField(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		so := DecodeServiceOptions([1]byte{byte(v)})
		if got := must(so.Field()); got != byte(v) {
			t.Errorf("DecodeServiceOptions(%#02x).Field() = %#02x", v, got)
		}
		bits := must(so.Encode())
		if got := bits[0]; got != byte(v) {
			t.Errorf("%#02x: Encode gives %#02x", v, got)
		}
	}
	if got := must(MotorolaServiceOptions().Field()); got != 0x20 {
		t.Errorf("MotorolaServiceOptions().Field() = %#02x, want 0x20", got)
	}
	var rangeErr *dmrgo.FieldRangeError
	if _, err := (ServiceOptions{Reserved: 4}).Field(); !errors.As(err, &rangeErr) || rangeErr.Field != "Reserved" {
		t.Errorf("Reserved 4: error %v, want the FieldRangeError of Reserved", err)
	}
	if _, err := (ServiceOptions{PriorityLevel: 4}).Field(); !errors.As(err, &rangeErr) || rangeErr.Field != "PriorityLevel" {
		t.Errorf("PriorityLevel 4: error %v, want the FieldRangeError of PriorityLevel", err)
	}
}
