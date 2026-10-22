package nmea_test

import (
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

const anytone = "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"

func withChecksum(body string) string {
	var sum byte
	for i := range len(body) {
		sum ^= body[i]
	}
	const hex = "0123456789ABCDEF"
	return "$" + body + "*" + string([]byte{hex[sum>>4], hex[sum&0xF]})
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestParseRMC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want nmea.RMC
	}{
		{
			name: "anytone",
			in:   anytone,
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Hour: 20, Minute: 0, Second: 37, Valid: true,
				Latitude: 34 + 12.73/60, Longitude: 108 + 50.0/60, Speed: 0, Course: 121,
				Day: 2, Month: 12, Year: 2025, Mode: 'A', HasTime: true, HasPosition: true, HasSpeed: true, HasCourse: true, HasDate: true},
		},
		{
			name: "south west with variation",
			in:   withChecksum("GNRMC,235959.5,A,4807.038,S,01131.000,W,22.4,084.4,230394,3.1,W,D") + "\n",
			want: nmea.RMC{Talker: [2]byte{'G', 'N'}, Hour: 23, Minute: 59, Second: 59, Millisecond: 500, Valid: true,
				Latitude: -(48 + 7.038/60), Longitude: -(11 + 31.0/60), Speed: 22.4, Course: 84.4,
				Day: 23, Month: 3, Year: 2094, MagneticVariation: -3.1, Mode: 'D', HasTime: true, HasPosition: true, HasSpeed: true, HasCourse: true, HasDate: true},
		},
		{
			name: "before 2.3",
			in:   withChecksum("GPRMC,081836,A,3751.65,S,14507.36,E,000.0,360.0,130998,011.3,E"),
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Hour: 8, Minute: 18, Second: 36, Valid: true,
				Latitude: -(37 + 51.65/60), Longitude: 145 + 7.36/60, Course: 360,
				Day: 13, Month: 9, Year: 2098, MagneticVariation: 11.3, HasTime: true, HasPosition: true, HasSpeed: true, HasCourse: true, HasDate: true},
		},
		{
			name: "4.1 with navigation status",
			in:   withChecksum("GNRMC,001031.123456,A,0000.0000,N,18000.0000,W,1.5,,010100,,,A,V"),
			want: nmea.RMC{Talker: [2]byte{'G', 'N'}, Minute: 10, Second: 31, Millisecond: 123, Valid: true,
				Longitude: -180, Speed: 1.5, Day: 1, Month: 1, Year: 2000, Mode: 'A',
				HasTime: true, HasPosition: true, HasSpeed: true, HasDate: true},
		},
		{
			name: "no fix",
			in:   withChecksum("GPRMC,,V,,,,,,,,,,N"),
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Mode: 'N'},
		},
		{
			name: "lowercase checksum",
			in:   "$GPRMC,200139.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*6f",
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Hour: 20, Minute: 1, Second: 39, Valid: true,
				Latitude: 34 + 12.73/60, Longitude: 108 + 50.0/60, Course: 121,
				Day: 2, Month: 12, Year: 2025, Mode: 'A', HasTime: true, HasPosition: true, HasSpeed: true, HasCourse: true, HasDate: true},
		},
		{
			name: "position without speed or course",
			in:   withChecksum("GPRMC,123519,A,4807.038,N,01131.000,E,,,230394,,"),
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Hour: 12, Minute: 35, Second: 19, Valid: true,
				Latitude: 48 + 7.038/60, Longitude: 11 + 31.0/60, Day: 23, Month: 3, Year: 2094,
				HasTime: true, HasPosition: true, HasDate: true},
		},
		{
			name: "date only",
			in:   withChecksum("GPRMC,,V,,,,,,,230394,,"),
			want: nmea.RMC{Talker: [2]byte{'G', 'P'}, Day: 23, Month: 3, Year: 2094, HasDate: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := nmea.ParseRMC([]byte(tt.in))
			if err != nil {
				t.Fatalf("ParseRMC(%q): %v", tt.in, err)
			}
			w := tt.want
			if !near(got.Latitude, w.Latitude) || !near(got.Longitude, w.Longitude) || !near(got.Speed, w.Speed) ||
				!near(got.Course, w.Course) || !near(got.MagneticVariation, w.MagneticVariation) {
				t.Errorf("ParseRMC(%q) = %+v, want %+v", tt.in, got, w)
			}
			got.Latitude, got.Longitude, got.Speed, got.Course, got.MagneticVariation = w.Latitude, w.Longitude, w.Speed, w.Course, w.MagneticVariation
			if got != w {
				t.Errorf("ParseRMC(%q) = %+v, want %+v", tt.in, got, w)
			}
		})
	}
}

func TestParseRMCChecksum(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"wrong":          "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*61\r\n",
		"one bit flips":  "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*E0",
		"payload change": "$GPRMC,200037.000,A,3412.7300,N,10850.0001,E,0.00,121.00,021225,,,A*60",
		"missing":        "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A\r\n",
		"one digit":      "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*6",
		"three digits":   "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*600",
		"not hex":        "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*6G",
		"trailing bytes": "$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n\x00",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := nmea.ParseRMC([]byte(in)); !errors.Is(err, nmea.ErrChecksum) {
				t.Errorf("ParseRMC(%q) = %v, want %v", in, err, nmea.ErrChecksum)
			}
		})
	}
}

func TestParseRMCRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", nmea.ErrSentence},
		{"no dollar", "GPRMC,,V,,,,,,,,,,N*53", nmea.ErrSentence},
		{"other sentence", withChecksum("GPGGA,200037.000,3412.7300,N,10850.0000,E,1,08,1.0,100.0,M,0.0,M,,"), nmea.ErrSentence},
		{"long address", withChecksum("GPRMCX,,V,,,,,,,,,,N"), nmea.ErrSentence},
		{"too few fields", withChecksum("GPRMC,,V,,,,,,,,"), nmea.ErrSentence},
		{"too many fields", withChecksum("GPRMC,,V,,,,,,,,,,N,V,X"), nmea.ErrSentence},
		{"control byte", withChecksum("GPRMC,,V,,,,,,,,,,\x01"), nmea.ErrSentence},
		{"second dollar", withChecksum("GPRMC,,V,,,,,,,,$,,N"), nmea.ErrSentence},
		{"hour 24", withChecksum("GPRMC,240000,V,,,,,,,,,,N"), nmea.ErrField},
		{"short time", withChecksum("GPRMC,2000,V,,,,,,,,,,N"), nmea.ErrField},
		{"time dot only", withChecksum("GPRMC,200000.,V,,,,,,,,,,N"), nmea.ErrField},
		{"status X", withChecksum("GPRMC,,X,,,,,,,,,,N"), nmea.ErrField},
		{"minutes 60", withChecksum("GPRMC,,A,3460.0000,N,10850.0000,E,,,,,,A"), nmea.ErrField},
		{"latitude 91", withChecksum("GPRMC,,A,9100.0000,N,10850.0000,E,,,,,,A"), nmea.ErrField},
		{"longitude 181", withChecksum("GPRMC,,A,3412.7300,N,18100.0000,E,,,,,,A"), nmea.ErrField},
		{"three latitude degrees", withChecksum("GPRMC,,A,03412.7300,N,10850.0000,E,,,,,,A"), nmea.ErrField},
		{"no hemisphere", withChecksum("GPRMC,,A,3412.7300,,10850.0000,E,,,,,,A"), nmea.ErrField},
		{"hemisphere E for latitude", withChecksum("GPRMC,,A,3412.7300,E,10850.0000,E,,,,,,A"), nmea.ErrField},
		{"hemisphere without value", withChecksum("GPRMC,,A,,N,,,,,,,,A"), nmea.ErrField},
		{"speed two dots", withChecksum("GPRMC,,A,,,,,1.2.3,,,,,A"), nmea.ErrField},
		{"speed dot only", withChecksum("GPRMC,,A,,,,,.,,,,,A"), nmea.ErrField},
		{"negative speed", withChecksum("GPRMC,,A,,,,,-1.0,,,,,A"), nmea.ErrField},
		{"speed too many digits", withChecksum("GPRMC,,A,,,,,1234567890123456789,,,,,A"), nmea.ErrField},
		{"course 361", withChecksum("GPRMC,,A,,,,,,361.0,,,,A"), nmea.ErrField},
		{"month 13", withChecksum("GPRMC,,A,,,,,,,011325,,,A"), nmea.ErrField},
		{"day 0", withChecksum("GPRMC,,A,,,,,,,001225,,,A"), nmea.ErrField},
		{"long date", withChecksum("GPRMC,,A,,,,,,,0212250,,,A"), nmea.ErrField},
		{"variation without side", withChecksum("GPRMC,,A,,,,,,,,3.1,,A"), nmea.ErrField},
		{"variation side N", withChecksum("GPRMC,,A,,,,,,,,3.1,N,A"), nmea.ErrField},
		{"lowercase mode", withChecksum("GPRMC,,A,,,,,,,,,,a"), nmea.ErrField},
		{"long navigation status", withChecksum("GPRMC,,A,,,,,,,,,,A,VV"), nmea.ErrField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := nmea.ParseRMC([]byte(tt.in)); !errors.Is(err, tt.want) {
				t.Errorf("ParseRMC(%q) = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}
