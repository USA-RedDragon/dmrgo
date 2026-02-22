package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildUSBDBits constructs a [96]bit.Bit array for a USBD PDU.
// serviceType is 4 bits, payload is 76 bits.
// CRC-CCITT is computed and XOR-masked per §B.3.9 (mask 0x3333).
func buildUSBDBits(serviceType byte, payload [76]bit.Bit) [96]bit.Bit {
	var data [96]bit.Bit

	// ServiceType: bits 0-3
	copy(data[0:4], bit.BitsFromUint8(serviceType, 4))

	// Payload: bits 4-79
	copy(data[4:80], payload[:])

	// Pack first 10 bytes to compute CRC
	var dataBytes [10]byte
	for i := range 10 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(data[i*8+j])
		}
	}

	crcVal := crc.CalculateCRCCCITT(dataBytes[:])
	crcHigh := byte(crcVal>>8) ^ 0x33
	crcLow := byte(crcVal) ^ 0x33
	for b := 7; b >= 0; b-- {
		data[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
		data[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
	}

	return data
}

func TestUSBD_DecodeLIP(t *testing.T) {
	var payload [76]bit.Bit
	// Set some payload bits
	payload[0] = 1
	payload[10] = 1
	payload[75] = 1

	infoBits := buildUSBDBits(0b0000, payload) // LIP service type

	usbd, fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeUnifiedSingleBlockData returned uncorrectable FEC")
	}

	if usbd.ServiceType != pdu.ServiceTypeLIP {
		t.Errorf("ServiceType = %d, want ServiceTypeLIP (0)", usbd.ServiceType)
	}

	// Verify payload bits
	if usbd.Payload[0] != 1 {
		t.Error("Payload[0] should be 1")
	}
	if usbd.Payload[10] != 1 {
		t.Error("Payload[10] should be 1")
	}
	if usbd.Payload[75] != 1 {
		t.Error("Payload[75] should be 1")
	}
	if usbd.Payload[1] != 0 {
		t.Error("Payload[1] should be 0")
	}
}

func TestUSBD_DecodeManufacturerSpecific(t *testing.T) {
	var payload [76]bit.Bit
	payload[0] = 1

	infoBits := buildUSBDBits(0b1000, payload) // ManufacturerSpecific1

	usbd, fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeUnifiedSingleBlockData returned uncorrectable FEC")
	}

	if usbd.ServiceType != pdu.ServiceTypeManufacturerSpecific1 {
		t.Errorf("ServiceType = %d, want ServiceTypeManufacturerSpecific1 (8)", usbd.ServiceType)
	}
}

func TestUSBD_EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		serviceType pdu.ServiceType
	}{
		{"LIP", pdu.ServiceTypeLIP},
		{"ManufacturerSpecific1", pdu.ServiceTypeManufacturerSpecific1},
		{"ManufacturerSpecific4", pdu.ServiceTypeManufacturerSpecific4},
		{"ManufacturerSpecific8", pdu.ServiceTypeManufacturerSpecific8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := pdu.UnifiedSingleBlockData{
				DataType:    elements.DataTypeUnifiedSingleBlock,
				ServiceType: tt.serviceType,
			}
			// Set some payload bits
			for i := 0; i < 76; i += 3 {
				orig.Payload[i] = 1
			}

			encoded := pdu.EncodeUnifiedSingleBlockData(&orig)
			decoded, fecResult := pdu.DecodeUnifiedSingleBlockData(encoded)
			if fecResult.Uncorrectable {
				t.Fatal("DecodeUnifiedSingleBlockData round-trip returned uncorrectable FEC")
			}

			if decoded.ServiceType != orig.ServiceType {
				t.Errorf("ServiceType = %d, want %d", decoded.ServiceType, orig.ServiceType)
			}
			for i := 0; i < 76; i++ {
				if decoded.Payload[i] != orig.Payload[i] {
					t.Errorf("Payload[%d] = %d, want %d", i, decoded.Payload[i], orig.Payload[i])
				}
			}
		})
	}
}

func TestUSBD_EncodeStability(t *testing.T) {
	orig := pdu.UnifiedSingleBlockData{
		ServiceType: pdu.ServiceTypeLIP,
	}
	for i := 0; i < 76; i += 2 {
		orig.Payload[i] = 1
	}

	enc1 := pdu.EncodeUnifiedSingleBlockData(&orig)
	dec1, _ := pdu.DecodeUnifiedSingleBlockData(enc1)
	enc2 := pdu.EncodeUnifiedSingleBlockData(&dec1)

	for i := 0; i < 96; i++ {
		if enc1[i] != enc2[i] {
			t.Errorf("Encode stability failed at bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestUSBD_CRCFailure(t *testing.T) {
	var infoBits [96]bit.Bit
	// All zeros will fail CRC check
	_, fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits)
	if !fecResult.Uncorrectable {
		t.Error("DecodeUnifiedSingleBlockData should return uncorrectable for invalid CRC")
	}
}

func TestUSBD_CRCBitFlip(t *testing.T) {
	var payload [76]bit.Bit
	payload[5] = 1
	infoBits := buildUSBDBits(0b0000, payload)

	// Flip a data bit to corrupt CRC
	infoBits[5] ^= 1

	_, fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits)
	if !fecResult.Uncorrectable {
		t.Error("DecodeUnifiedSingleBlockData should return uncorrectable after bit flip")
	}
}

func TestUSBD_GetDataType(t *testing.T) {
	usbd := &pdu.UnifiedSingleBlockData{
		DataType: elements.DataTypeUnifiedSingleBlock,
	}
	if usbd.GetDataType() != elements.DataTypeUnifiedSingleBlock {
		t.Errorf("GetDataType() = %d, want DataTypeUnifiedSingleBlock", usbd.GetDataType())
	}
}

func TestUSBD_ToString(t *testing.T) {
	var payload [76]bit.Bit
	payload[0] = 1

	infoBits := buildUSBDBits(0b0000, payload)

	usbd, _ := pdu.DecodeUnifiedSingleBlockData(infoBits)
	usbd.DataType = elements.DataTypeUnifiedSingleBlock

	str := usbd.ToString()
	if str == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestServiceTypeToName(t *testing.T) {
	tests := []struct {
		st   pdu.ServiceType
		want string
	}{
		{pdu.ServiceTypeLIP, "Location Information Protocol"},
		{pdu.ServiceTypeManufacturerSpecific1, "Manufacturer Specific Service 1"},
		{pdu.ServiceTypeManufacturerSpecific2, "Manufacturer Specific Service 2"},
		{pdu.ServiceTypeManufacturerSpecific3, "Manufacturer Specific Service 3"},
		{pdu.ServiceTypeManufacturerSpecific4, "Manufacturer Specific Service 4"},
		{pdu.ServiceTypeManufacturerSpecific5, "Manufacturer Specific Service 5"},
		{pdu.ServiceTypeManufacturerSpecific6, "Manufacturer Specific Service 6"},
		{pdu.ServiceTypeManufacturerSpecific7, "Manufacturer Specific Service 7"},
		{pdu.ServiceTypeManufacturerSpecific8, "Manufacturer Specific Service 8"},
	}

	for _, tt := range tests {
		if got := pdu.ServiceTypeToName(tt.st); got != tt.want {
			t.Errorf("ServiceTypeToName(%d) = %q, want %q", tt.st, got, tt.want)
		}
	}

	// Reserved values should contain "Reserved"
	reserved := pdu.ServiceType(0b0001)
	name := pdu.ServiceTypeToName(reserved)
	if name == "" {
		t.Error("ServiceTypeToName for reserved should not be empty")
	}
}

func TestUSBD_AllServiceTypes(t *testing.T) {
	// Test that all 16 possible service type values decode without panic
	for st := 0; st < 16; st++ {
		var payload [76]bit.Bit
		infoBits := buildUSBDBits(byte(st), payload)
		usbd, fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits)
		if fecResult.Uncorrectable {
			t.Errorf("ServiceType %d: decode returned uncorrectable", st)
		}
		if uint8(usbd.ServiceType) != uint8(st) {
			t.Errorf("ServiceType %d: decoded as %d", st, usbd.ServiceType)
		}
	}
}
