package nmea_test

import (
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func TestAppendRMC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   nmea.RMC
		want string
	}{
		{"anytone", nmea.RMC{Talker: [2]byte{'G', 'P'}, Hour: 20, Minute: 0, Second: 37, HasTime: true, Valid: true,
			Latitude: 34 + 12.73/60, Longitude: 108 + 50.0/60, HasPosition: true, HasSpeed: true, Course: 121, HasCourse: true,
			Day: 2, Month: 12, Year: 2025, HasDate: true, Mode: 'A'},
			"$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"},
		{"south west", nmea.RMC{Talker: [2]byte{'G', 'N'}, Hour: 23, Minute: 59, Second: 59, Millisecond: 500, HasTime: true, Valid: true,
			Latitude: -(48 + 7.038/60), Longitude: -(11 + 31.0/60), HasPosition: true, Speed: 22.4, HasSpeed: true, Course: 84.4, HasCourse: true,
			Day: 23, Month: 3, Year: 2094, HasDate: true, MagneticVariation: -3.1, Mode: 'D'},
			"$GNRMC,235959.500,A,4807.0380,S,01131.0000,W,22.40,84.40,230394,003.1,W,D*04\r\n"},
		{"east variation, no mode", nmea.RMC{Valid: true, Latitude: 37 + 51.65/60, Longitude: -(145 + 7.36/60), HasPosition: true,
			MagneticVariation: 11.3},
			"$GPRMC,,A,3751.6500,N,14507.3600,W,,,,011.3,E*66\r\n"},
		{"empty", nmea.RMC{Mode: 'N'}, "$GPRMC,,V,,,,,,,,,,N*53\r\n"},
		{"carry", nmea.RMC{Latitude: 59.999999999, Longitude: -179.99999999, HasPosition: true, Speed: 0.004, HasSpeed: true, Course: 359.999, HasCourse: true},
			"$GPRMC,,V,6000.0000,N,18000.0000,W,0.00,360.00,,,*12\r\n"},
	}
	for _, tt := range tests {
		out, err := nmea.AppendRMC([]byte("x"), &tt.in)
		if err != nil || string(out) != "x"+tt.want {
			t.Errorf("%s: AppendRMC = %q, %v, want %q", tt.name, out, err, tt.want)
			continue
		}
		if _, err := nmea.ParseRMC(out[1:]); err != nil {
			t.Errorf("%s: ParseRMC(%q): %v", tt.name, out[1:], err)
		}
	}
}

func TestAppendRMCRejects(t *testing.T) {
	t.Parallel()

	ok := nmea.RMC{HasTime: true, HasPosition: true, HasSpeed: true, HasCourse: true, HasDate: true, Day: 1, Month: 1, Year: 2000}
	tests := []struct {
		name string
		edit func(r *nmea.RMC)
	}{
		{"talker comma", func(r *nmea.RMC) { r.Talker = [2]byte{'G', ','} }},
		{"talker star", func(r *nmea.RMC) { r.Talker = [2]byte{'*', 'P'} }},
		{"talker nul", func(r *nmea.RMC) { r.Talker = [2]byte{'G', 0} }},
		{"mode", func(r *nmea.RMC) { r.Mode = '*' }},
		{"hour", func(r *nmea.RMC) { r.Hour = 24 }},
		{"minute", func(r *nmea.RMC) { r.Minute = 60 }},
		{"second", func(r *nmea.RMC) { r.Second = 61 }},
		{"millisecond", func(r *nmea.RMC) { r.Millisecond = 1000 }},
		{"latitude", func(r *nmea.RMC) { r.Latitude = 90.5 }},
		{"latitude NaN", func(r *nmea.RMC) { r.Latitude = math.NaN() }},
		{"longitude", func(r *nmea.RMC) { r.Longitude = -181 }},
		{"longitude Inf", func(r *nmea.RMC) { r.Longitude = math.Inf(1) }},
		{"speed", func(r *nmea.RMC) { r.Speed = -1 }},
		{"speed large", func(r *nmea.RMC) { r.Speed = 1e13 }},
		{"course", func(r *nmea.RMC) { r.Course = 360.5 }},
		{"day", func(r *nmea.RMC) { r.Day = 0 }},
		{"month", func(r *nmea.RMC) { r.Month = 13 }},
		{"year", func(r *nmea.RMC) { r.Year = 2100 }},
		{"variation", func(r *nmea.RMC) { r.MagneticVariation = 181 }},
		{"variation NaN", func(r *nmea.RMC) { r.MagneticVariation = math.NaN() }},
	}
	for _, tt := range tests {
		r := ok
		tt.edit(&r)
		out, err := nmea.AppendRMC([]byte("x"), &r)
		if !errors.Is(err, nmea.ErrField) || string(out) != "x" {
			t.Errorf("%s: AppendRMC = %q, %v, want %v", tt.name, out, err, nmea.ErrField)
		}
	}
	if out, err := nmea.AppendRMC([]byte("x"), &ok); err != nil {
		t.Errorf("AppendRMC(%+v) = %q, %v", ok, out, err)
	}
	r := ok
	r.HasTime, r.Hour, r.HasPosition, r.Latitude, r.HasSpeed, r.Speed, r.HasCourse, r.Course, r.HasDate, r.Day = false, 99, false, 99, false, -1, false, 999, false, 0
	if out, err := nmea.AppendRMC(nil, &r); err != nil || string(out) != "$GPRMC,,V,,,,,,,,,*31\r\n" {
		t.Errorf("fields without their Has flags: %q, %v", out, err)
	}
	if out, err := nmea.AppendRMC([]byte("x"), nil); !errors.Is(err, dmrgo.ErrNilArgument) || string(out) != "x" {
		t.Errorf("nil RMC: %q, %v", out, err)
	}
}
