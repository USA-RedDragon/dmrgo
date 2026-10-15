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
			bits := must(in.Encode())
			out, _ := pdu.DecodeFLCGPSInfo(bits)
			if math.Abs(out.Longitude-tt.lon) > lonStep/2 {
				t.Errorf("Longitude = %v, want %v", out.Longitude, tt.lon)
			}
			if math.Abs(out.Latitude-tt.lat) > latStep/2 {
				t.Errorf("Latitude = %v, want %v", out.Latitude, tt.lat)
			}
			if again := must(out.Encode()); again != bits {
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
	bits := must(in.Encode())
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

// Values at the top of each range have no code of their own. Longitude
// wraps to -180, the same meridian, and latitude takes the largest code.
func TestFLCGPSInfo_EncodeRangeEdges(t *testing.T) {
	tests := []struct {
		name             string
		lon, lat         float64
		wantLon, wantLat float64
	}{
		{"Longitude180", 180, 0, -180, 0},
		{"LongitudeRoundsTo180", 180 - lonStep/4, 0, -180, 0},
		{"LongitudeBelowHalfStep", 180 - lonStep*3/4, 0, 180 - lonStep, 0},
		{"Latitude90", 0, 90, 0, 90 - latStep},
		{"LatitudeRoundsTo90", 0, 90 - latStep/4, 0, 90 - latStep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := pdu.DecodeFLCGPSInfo(must((&pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat}).Encode()))
			if out.Longitude != tt.wantLon || out.Latitude != tt.wantLat {
				t.Errorf("decoded %v, %v, want %v, %v", out.Longitude, out.Latitude, tt.wantLon, tt.wantLat)
			}
		})
	}
}
