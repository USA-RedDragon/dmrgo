package pdu_test

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	lonStep = 360.0 / (1 << 25)
	latStep = 180.0 / (1 << 24)
)

func TestFLCGPSInfo_CoordinatesRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		lon, lat float64
	}{
		{"Origin", 0, 0},
		{"SouthWest", -90, -45},
		{"Texas", -97.5, -33.0},
		{"NorthEast", 151.2093, 33.8688},
		{"MinimumLongitude", -180, 0},
		{"MaximumLongitude", 180 - lonStep, 0},
		{"MinimumLatitude", 0, -90},
		{"MaximumLatitude", 0, 90 - latStep},
		{"SmallestNegative", -lonStep, -latStep},
		{"SmallestPositive", lonStep, latStep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat}
			bits := must(pdu.EncodeFLCGPSInfo(&in))
			out, _ := pdu.DecodeFLCGPSInfo(bits)
			if math.Abs(out.Longitude-tt.lon) > lonStep/2 {
				t.Errorf("Longitude = %v, want %v", out.Longitude, tt.lon)
			}
			if math.Abs(out.Latitude-tt.lat) > latStep/2 {
				t.Errorf("Latitude = %v, want %v", out.Latitude, tt.lat)
			}
			if again := must(pdu.EncodeFLCGPSInfo(&out)); again != bits {
				t.Error("re-encoding the decoded value changed the bits")
			}
		})
	}
}

func TestFLCGPSInfo_DecodeTwosComplement(t *testing.T) {
	var bits [56]bit.Bit
	// Longitude bits 7-31: 1 followed by zeros = -2^24 steps = -180 degrees.
	bits[7] = 1
	// Latitude bits 32-55: all ones = -1 step.
	for i := 32; i < 56; i++ {
		bits[i] = 1
	}
	out, _ := pdu.DecodeFLCGPSInfo(bits)
	if out.Longitude != -180 {
		t.Errorf("Longitude = %v, want -180", out.Longitude)
	}
	if out.Latitude != -latStep {
		t.Errorf("Latitude = %v, want %v", out.Latitude, -latStep)
	}
}

func TestFLCGPSInfo_EncodeTwosComplement(t *testing.T) {
	in := pdu.FLCGPSInfo{Longitude: -lonStep, Latitude: -90}
	bits := must(pdu.EncodeFLCGPSInfo(&in))
	for i := 7; i < 32; i++ {
		if bits[i] != 1 {
			t.Fatalf("longitude bit %d = %d, want all ones for -1 step", i, bits[i])
		}
	}
	if bits[32] != 1 {
		t.Error("latitude MSB should be set for -90")
	}
	for i := 33; i < 56; i++ {
		if bits[i] != 0 {
			t.Fatalf("latitude bit %d = %d, want 0 for -90", i, bits[i])
		}
	}
}
