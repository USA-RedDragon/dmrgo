package elements

import (
	"testing"
)

func TestNewServiceOptionsFromBits(t *testing.T) {
	tests := []struct {
		name                string
		bits                [8]byte
		isEmergency         bool
		isPrivacy           bool
		isBroadcast         bool
		isOpenVoiceCallMode bool
		priorityLevel       uint8
	}{
		{
			name:                "AllZeros",
			bits:                [8]byte{0, 0, 0, 0, 0, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Emergency",
			bits:                [8]byte{1, 0, 0, 0, 0, 0, 0, 0},
			isEmergency:         true,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Privacy",
			bits:                [8]byte{0, 1, 0, 0, 0, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           true,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "Broadcast",
			bits:                [8]byte{0, 0, 0, 0, 1, 0, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         true,
			isOpenVoiceCallMode: false,
			priorityLevel:       0,
		},
		{
			name:                "OpenVoiceCallMode",
			bits:                [8]byte{0, 0, 0, 0, 0, 1, 0, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: true,
			priorityLevel:       0,
		},
		{
			name:                "Priority1",
			bits:                [8]byte{0, 0, 0, 0, 0, 0, 1, 0},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       1,
		},
		{
			name:                "Priority2",
			bits:                [8]byte{0, 0, 0, 0, 0, 0, 0, 1},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       2,
		},
		{
			name:                "Priority3",
			bits:                [8]byte{0, 0, 0, 0, 0, 0, 1, 1},
			isEmergency:         false,
			isPrivacy:           false,
			isBroadcast:         false,
			isOpenVoiceCallMode: false,
			priorityLevel:       3,
		},
		{
			name:                "AllSet",
			bits:                [8]byte{1, 1, 1, 1, 1, 1, 1, 1},
			isEmergency:         true,
			isPrivacy:           true,
			isBroadcast:         true,
			isOpenVoiceCallMode: true,
			priorityLevel:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := NewServiceOptionsFromBits(tt.bits)
			if so == nil {
				t.Fatal("NewServiceOptionsFromBits returned nil")
			}
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

func TestServiceOptions_ReservedBits(t *testing.T) {
	bits := [8]byte{0, 0, 1, 1, 0, 0, 0, 0}
	so := NewServiceOptionsFromBits(bits)
	if so.Reserved[0] != 1 {
		t.Errorf("Reserved[0] = %d, want 1", so.Reserved[0])
	}
	if so.Reserved[1] != 1 {
		t.Errorf("Reserved[1] = %d, want 1", so.Reserved[1])
	}
}

func TestServiceOptions_ToByte_RoundTrip(t *testing.T) {
	// Test that NewServiceOptionsFromBits -> ToByte produces the correct packed byte
	tests := []struct {
		name         string
		bits         [8]byte
		expectedByte byte
	}{
		{"AllZeros", [8]byte{0, 0, 0, 0, 0, 0, 0, 0}, 0x00},
		{"Emergency", [8]byte{1, 0, 0, 0, 0, 0, 0, 0}, 0x80},
		{"Privacy", [8]byte{0, 1, 0, 0, 0, 0, 0, 0}, 0x40},
		{"Reserved0", [8]byte{0, 0, 1, 0, 0, 0, 0, 0}, 0x20},
		{"Reserved1", [8]byte{0, 0, 0, 1, 0, 0, 0, 0}, 0x10},
		{"Broadcast", [8]byte{0, 0, 0, 0, 1, 0, 0, 0}, 0x08},
		{"OpenVoice", [8]byte{0, 0, 0, 0, 0, 1, 0, 0}, 0x04},
		{"Priority1", [8]byte{0, 0, 0, 0, 0, 0, 1, 0}, 0x02},
		{"Priority2", [8]byte{0, 0, 0, 0, 0, 0, 0, 1}, 0x01},
		{"AllSet", [8]byte{1, 1, 1, 1, 1, 1, 1, 1}, 0xFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := NewServiceOptionsFromBits(tt.bits)
			b := so.ToByte()
			if b != tt.expectedByte {
				t.Errorf("ToByte() = 0x%02X, want 0x%02X", b, tt.expectedByte)
			}
		})
	}
}

func TestServiceOptions_ToByte_FromByte_RoundTrip(t *testing.T) {
	// Test all 256 possible byte values
	for i := 0; i < 256; i++ {
		b := byte(i)

		// Unpack byte to bits
		var bits [8]byte
		for j := 0; j < 8; j++ {
			if (b>>(7-j))&1 == 1 {
				bits[j] = 1
			}
		}

		so := NewServiceOptionsFromBits(bits)
		result := so.ToByte()
		if result != b {
			t.Errorf("byte 0x%02X: ToByte() = 0x%02X, want 0x%02X", b, result, b)
		}
	}
}

func TestServiceOptions_ToString(t *testing.T) {
	bits := [8]byte{1, 0, 0, 0, 1, 0, 1, 0}
	so := NewServiceOptionsFromBits(bits)
	str := so.ToString()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	// Should contain "IsEmergency: true"
	if !containsStr(str, "IsEmergency: true") {
		t.Errorf("ToString() missing IsEmergency: true, got %q", str)
	}
	if !containsStr(str, "IsBroadcast: true") {
		t.Errorf("ToString() missing IsBroadcast: true, got %q", str)
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
