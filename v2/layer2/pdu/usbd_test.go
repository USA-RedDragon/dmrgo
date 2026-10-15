package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildUSBDBits constructs a [96]bit.Bit array for a USBD PDU.
// serviceType is 4 bits, payload is 76 bits.
// CRC-CCITT is computed and XOR-masked with mask 0x3333 (ETSI TS 102 361-1 clause B.3.8).
func buildUSBDBits(serviceType byte, payload [76]bit.Bit) [96]bit.Bit {
	var data [96]bit.Bit

	// ServiceType: bits 0-3
	bit.PutUint(data[0:4], uint64(serviceType))

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

	crcVal := crc.CCITT(dataBytes[:])
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

	usbd := pdu.DecodeUnifiedSingleBlockData(infoBits)
	fecResult := usbd.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeUnifiedSingleBlockData returned uncorrectable FEC")
	}

	if usbd.ServiceType != pdu.ServiceTypeLIP {
		t.Errorf("ServiceType = %d, want ServiceTypeLIP (0)", usbd.ServiceType)
	}

	// Verify payload bits
	if rawBit(usbd.Payload[:], 0) != 1 {
		t.Error("Payload[0] should be 1")
	}
	if rawBit(usbd.Payload[:], 10) != 1 {
		t.Error("Payload[10] should be 1")
	}
	if rawBit(usbd.Payload[:], 75) != 1 {
		t.Error("Payload[75] should be 1")
	}
	if rawBit(usbd.Payload[:], 1) != 0 {
		t.Error("Payload[1] should be 0")
	}
}

func TestUSBD_DecodeManufacturerSpecific(t *testing.T) {
	var payload [76]bit.Bit
	payload[0] = 1

	infoBits := buildUSBDBits(0b1000, payload) // ManufacturerSpecific1

	usbd := pdu.DecodeUnifiedSingleBlockData(infoBits)
	fecResult := usbd.FEC
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
				ServiceType: tt.serviceType,
			}
			// Set some payload bits
			for i := 0; i < 76; i += 3 {
				setRawBit(orig.Payload[:], i)
			}

			encoded := must(orig.Encode())
			decoded := pdu.DecodeUnifiedSingleBlockData(encoded)
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeUnifiedSingleBlockData round-trip returned uncorrectable FEC")
			}

			if decoded.ServiceType != orig.ServiceType {
				t.Errorf("ServiceType = %d, want %d", decoded.ServiceType, orig.ServiceType)
			}
			if decoded.Payload != orig.Payload {
				t.Errorf("Payload = %x, want %x", decoded.Payload, orig.Payload)
			}
		})
	}
}

func TestUSBD_EncodeStability(t *testing.T) {
	orig := pdu.UnifiedSingleBlockData{
		ServiceType: pdu.ServiceTypeLIP,
	}
	for i := 0; i < 76; i += 2 {
		setRawBit(orig.Payload[:], i)
	}

	enc1 := must(orig.Encode())
	dec1 := pdu.DecodeUnifiedSingleBlockData(enc1)
	enc2 := must(dec1.Encode())

	for i := 0; i < 96; i++ {
		if enc1[i] != enc2[i] {
			t.Errorf("Encode stability failed at bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestUSBD_CRCFailure(t *testing.T) {
	var infoBits [96]bit.Bit
	// All zeros will fail CRC check
	fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits).FEC
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

	fecResult := pdu.DecodeUnifiedSingleBlockData(infoBits).FEC
	if !fecResult.Uncorrectable {
		t.Error("DecodeUnifiedSingleBlockData should return uncorrectable after bit flip")
	}
}

func TestUSBD_DataType(t *testing.T) {
	usbd := &pdu.UnifiedSingleBlockData{}
	if usbd.DataType() != enums.DataTypeUnifiedSingleBlock {
		t.Errorf("DataType() = %d, want DataTypeUnifiedSingleBlock", usbd.DataType())
	}
}

func TestUSBD_String(t *testing.T) {
	var payload [76]bit.Bit
	payload[0] = 1

	infoBits := buildUSBDBits(0b0000, payload)

	usbd := pdu.DecodeUnifiedSingleBlockData(infoBits)

	str := usbd.String()
	if str == "" {
		t.Error("String() should not be empty")
	}
}

func TestServiceTypeString(t *testing.T) {
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
		if got := tt.st.String(); got != tt.want {
			t.Errorf("(%d).String() = %q, want %q", tt.st, got, tt.want)
		}
	}

	// Reserved values should contain "Reserved"
	reserved := pdu.ServiceType(0b0001)
	name := reserved.String()
	if name == "" {
		t.Error("ServiceTypeToName for reserved should not be empty")
	}
}

func TestUSBD_AllServiceTypes(t *testing.T) {
	// Test that all 16 possible service type values decode without panic
	for st := 0; st < 16; st++ {
		var payload [76]bit.Bit
		infoBits := buildUSBDBits(byte(st), payload)
		usbd := pdu.DecodeUnifiedSingleBlockData(infoBits)
		fecResult := usbd.FEC
		if fecResult.Uncorrectable {
			t.Errorf("ServiceType %d: decode returned uncorrectable", st)
		}
		if uint8(usbd.ServiceType) != uint8(st) {
			t.Errorf("ServiceType %d: decoded as %d", st, usbd.ServiceType)
		}
	}
}

func TestUSBD_PayloadLayout(t *testing.T) {
	var payload [76]bit.Bit
	payload[0], payload[75] = 1, 1
	usbd := pdu.DecodeUnifiedSingleBlockData(buildUSBDBits(0, payload))
	if want := [10]byte{0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0x10}; usbd.Payload != want {
		t.Fatalf("Payload = %x, want %x: bit 4 in the top bit of byte 0, bit 79 in bit 4 of byte 9", usbd.Payload, want)
	}
	usbd.Payload[9] |= 0x01
	_, err := usbd.Encode()
	var rangeErr *bit.FieldRangeError
	if !errors.As(err, &rangeErr) || rangeErr.Field != "Payload" || rangeErr.Bits != 76 {
		t.Errorf("Encode with a padding bit set: %v, want a FieldRangeError for Payload", err)
	}
}
