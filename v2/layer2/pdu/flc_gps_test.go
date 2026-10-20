package pdu_test

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	lonStep = 360.0 / (1 << 25)
	latStep = 180.0 / (1 << 24)
)

func TestFLCGPSInfo_CoordinatesRoundTrip(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			in := pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat}
			enc := must(in.Encode())
			out := pdu.DecodeFLCGPSInfo(enc)
			if math.Abs(out.Longitude-tt.lon) > lonStep/2 {
				t.Errorf("Longitude = %v, want %v", out.Longitude, tt.lon)
			}
			if math.Abs(out.Latitude-tt.lat) > latStep/2 {
				t.Errorf("Latitude = %v, want %v", out.Latitude, tt.lat)
			}
			if again := must(out.Encode()); again != enc {
				t.Error("re-encoding the decoded value changed the bits")
			}
		})
	}
}

func TestFLCGPSInfo_DecodeTwosComplement(t *testing.T) {
	t.Parallel()

	var data [7]byte
	bits.SetBit(data[:], 7, 1)
	bits.SetField(data[:], 32, 24, 1<<24-1)
	out := pdu.DecodeFLCGPSInfo(data)
	if out.Longitude != -180 {
		t.Errorf("Longitude = %v, want -180", out.Longitude)
	}
	if out.Latitude != -latStep {
		t.Errorf("Latitude = %v, want %v", out.Latitude, -latStep)
	}
}

func TestFLCGPSInfo_EncodeTwosComplement(t *testing.T) {
	t.Parallel()

	in := pdu.FLCGPSInfo{Longitude: -lonStep, Latitude: -90}
	enc := must(in.Encode())
	for i := 7; i < 32; i++ {
		if b := bits.Bit(enc[:], i); b != 1 {
			t.Fatalf("longitude bit %d = %d, want all ones for -1 step", i, b)
		}
	}
	if bits.Bit(enc[:], 32) != 1 {
		t.Error("latitude MSB should be set for -90")
	}
	for i := 33; i < 56; i++ {
		if b := bits.Bit(enc[:], i); b != 0 {
			t.Fatalf("latitude bit %d = %d, want 0 for -90", i, b)
		}
	}
}

func TestFLCGPSInfo_EncodeRangeEdges(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			out := pdu.DecodeFLCGPSInfo(must((&pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat}).Encode()))
			if out.Longitude != tt.wantLon || out.Latitude != tt.wantLat {
				t.Errorf("decoded %v, %v, want %v, %v", out.Longitude, out.Latitude, tt.wantLon, tt.wantLat)
			}
		})
	}
}
