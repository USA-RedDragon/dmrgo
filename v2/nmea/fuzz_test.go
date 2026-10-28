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

func FuzzAppendRMC(f *testing.F) {
	rmcSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzAppendRMC)
}

func rmcSeeds(add func(data []byte)) {
	add([]byte(anytone))
	add([]byte("$GNRMC,235959.5,A,4807.038,S,01131.000,W,22.4,084.4,230394,3.1,W,D*1B\n"))
	add([]byte("$GPRMC,081836,A,3751.65,S,14507.36,E,000.0,360.0,130998,011.3,E*62"))
	add([]byte("$GPRMC,,V,,,,,,,,,,N*53"))
	add([]byte("$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*61"))
	add([]byte("\x00\x01\x00\x00\x00'~'~$GPRMC"))
	add([]byte("$GPRMC,081833,A,3751.65,S,14507.36,E,6.0,360.0,130998,0.033,E*62"))
	add([]byte("$GaRMC,235959.5,A,4807.038,S,01131.0,W,.4,084.4,230394,3.1,W,D*1B"))
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

func fuzzAppendRMC(t *testing.T, data []byte) {
	r, err := nmea.ParseRMC(data)
	if err != nil {
		return
	}
	out, err := nmea.AppendRMC(nil, &r)
	if err != nil {
		if !errors.Is(err, nmea.ErrField) || r.Speed < 1e13 {
			t.Fatalf("AppendRMC(%+v) from %q: %v", r, data, err)
		}
		return
	}
	g, err := nmea.ParseRMC(out)
	if err != nil {
		t.Fatalf("ParseRMC(AppendRMC(%+v)) = ParseRMC(%q): %v", r, out, err)
	}
	if r.Talker == [2]byte{} {
		r.Talker = [2]byte{'G', 'P'}
	}
	if g.Talker != r.Talker || g.HasTime != r.HasTime || g.Hour != r.Hour || g.Minute != r.Minute || g.Second != r.Second ||
		g.Millisecond != r.Millisecond || g.Valid != r.Valid || g.HasPosition != r.HasPosition || g.HasSpeed != r.HasSpeed ||
		g.HasCourse != r.HasCourse || g.HasDate != r.HasDate || g.Day != r.Day || g.Month != r.Month || g.Year != r.Year ||
		g.Mode != r.Mode || !within(g.Latitude, r.Latitude, 1e-6) || !within(g.Longitude, r.Longitude, 1e-6) ||
		!within(g.Speed, r.Speed, 0.005+r.Speed*1e-15) || !within(g.Course, r.Course, 0.005) ||
		!within(g.MagneticVariation, r.MagneticVariation, 0.05) {
		t.Fatalf("ParseRMC(AppendRMC(%+v)) = %+v from %q", r, g, out)
	}
	again, err := nmea.AppendRMC(nil, &g)
	if err != nil || string(again) != string(out) {
		t.Fatalf("AppendRMC(%+v) = %q, %v, want %q", g, again, err, out)
	}
}

func within(a, b, tol float64) bool {
	d := a - b
	return d <= tol && d >= -tol
}
