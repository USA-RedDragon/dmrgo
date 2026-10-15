package layer3

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestDecodeServiceOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		bits                [8]bit.Bit
		isEmergency         bool
		isPrivacy           bool
		isBroadcast         bool
		isOpenVoiceCallMode bool
		priorityLevel       uint8
	}{
		{
			name:                "AllZeros",
			bits:                [8]bit.Bit{0, 0, 0, 0, 0, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Emergency",
			bits:                [8]bit.Bit{1, 0, 0, 0, 0, 0, 0, 0},
			isEmergency:         true,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Privacy",
			bits:                [8]bit.Bit{0, 1, 0, 0, 0, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           true,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Broadcast",
			bits:                [8]bit.Bit{0, 0, 0, 0, 1, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         true,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "OpenVoiceCallMode",
			bits:                [8]bit.Bit{0, 0, 0, 0, 0, 1, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: true,
			priorityLevel:       0,
		},
		{
			name:                "Priority2",
			bits:                [8]bit.Bit{0, 0, 0, 0, 0, 0, 1, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       2,
		},
		{
			name:                "Priority1",
			bits:                [8]bit.Bit{0, 0, 0, 0, 0, 0, 0, 1},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       1,
		},
		{
			name:                "Priority3",
			bits:                [8]bit.Bit{0, 0, 0, 0, 0, 0, 1, 1},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       3,
		},
		{
			name:                "AllSet",
			bits:                [8]bit.Bit{1, 1, 1, 1, 1, 1, 1, 1},
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

	bits := [8]bit.Bit{0, 0, 1, 1, 0, 0, 0, 0}
	so := DecodeServiceOptions(bits)
	if so.Reserved != 0b11 {
		t.Errorf("Reserved = %#b, want 0b11", so.Reserved)
	}
}

func TestServiceOptions_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	for i := 0; i < 256; i++ {
		b := byte(i)

		var bits [8]bit.Bit
		for j := 0; j < 8; j++ {
			if (b>>(7-j))&1 == 1 {
				bits[j] = 1
			}
		}

		so := DecodeServiceOptions(bits)
		encoded := must(so.Encode())
		if encoded != bits {
			t.Errorf("byte 0x%02X: decode then encode mismatch: got %v, want %v", b, encoded, bits)
		}
	}
}

func TestServiceOptions_String(t *testing.T) {
	t.Parallel()

	bits := [8]bit.Bit{1, 0, 0, 0, 1, 0, 1, 0}
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

func TestServiceOptionsByte(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		so := ServiceOptionsFromByte(byte(v))
		if got := so.Byte(); got != byte(v) {
			t.Errorf("ServiceOptionsFromByte(%#02x).Byte() = %#02x", v, got)
		}
		bits := must(so.Encode())
		if got := byte(bit.Uint(bits[:])); got != byte(v) {
			t.Errorf("%#02x: Encode gives %#02x", v, got)
		}
	}
	if got := DefaultServiceOptions().Byte(); got != 0x20 {
		t.Errorf("DefaultServiceOptions().Byte() = %#02x, want 0x20", got)
	}
	if got := (ServiceOptions{Reserved: 7, PriorityLevel: 6}).Byte(); got != 0x32 {
		t.Errorf("out of range fields give %#02x, want their low bits 0x32", got)
	}
}
