package nmea_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func FuzzParseRMC(f *testing.F) {
	rmcSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzParseRMC)
}

func rmcSeeds(add func(data []byte)) {
	add([]byte(anytone))
	add([]byte("$GNRMC,235959.5,A,4807.038,S,01131.000,W,22.4,084.4,230394,3.1,W,D*1B\n"))
	add([]byte("$GPRMC,081836,A,3751.65,S,14507.36,E,000.0,360.0,130998,011.3,E*62"))
	add([]byte("$GPRMC,,V,,,,,,,,,,N*53"))
	add([]byte("$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*61"))
	add([]byte("\x00\x01\x00\x00\x00'~'~$GPRMC"))
}

func fuzzParseRMC(t *testing.T, data []byte) {
	r, err := nmea.ParseRMC(data)
	if err != nil {
		if !errors.Is(err, nmea.ErrSentence) && !errors.Is(err, nmea.ErrChecksum) && !errors.Is(err, nmea.ErrField) {
			t.Fatalf("ParseRMC(%q): unexpected error %v", data, err)
		}
		if r != (nmea.RMC{}) {
			t.Fatalf("ParseRMC(%q) = %+v with error %v, want the zero RMC", data, r, err)
		}
		return
	}
	if r.Latitude < -90 || r.Latitude > 90 || r.Longitude < -180 || r.Longitude > 180 ||
		r.Speed < 0 || r.Course < 0 || r.Course > 360 || r.MagneticVariation < -180 || r.MagneticVariation > 180 ||
		r.Hour > 23 || r.Minute > 59 || r.Second > 60 || r.Millisecond > 999 ||
		(r.Day != 0 && (r.Month < 1 || r.Month > 12 || r.Year < 2000 || r.Year > 2099)) || r.Day > 31 {
		t.Fatalf("ParseRMC(%q) = %+v, out of range", data, r)
	}
	if data[0] != '$' {
		t.Fatalf("ParseRMC(%q) accepted a sentence without $", data)
	}
	star := len(data) - 1
	for star >= 0 && data[star] != '*' {
		star--
	}
	bad := append([]byte(nil), data...)
	bad[star+1] ^= 0x01
	if _, err := nmea.ParseRMC(bad); !errors.Is(err, nmea.ErrChecksum) {
		t.Fatalf("ParseRMC(%q) = %v, want %v", bad, err, nmea.ErrChecksum)
	}
}
