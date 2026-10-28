package nmea

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2"
)

const maxSpeed = 1e13

// AppendRMC appends r to dst as an RMC sentence in the layout that AnyTone
// radios send, and returns dst: the time as hhmmss.sss, the latitude as
// ddmm.mmmm, the longitude as dddmm.mmmm, the speed and the course with two
// decimals, the date as ddmmyy, the magnetic variation with one decimal or
// empty when it rounds to 0, the mode indicator, then the checksum and CR
// LF, as in
// $GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60. A
// field whose Has flag is not set is empty, and an r with Mode 0 gives the
// 11 data fields of NMEA 0183 before version 2.3. A zero Talker is sent as
// GP. ParseRMC reads the sentence back, with the values rounded to those
// decimals. It returns dst unchanged and ErrField for a Talker with a byte
// that is not printable ASCII or is $, * or a comma, a Mode that is not an
// upper case letter, a field out of the range that ParseRMC accepts, a Speed
// of 10^13 knots or more, or a value that is not a number, and
// dmrgo.ErrNilArgument for a nil r. It allocates only to grow dst.
func AppendRMC(dst []byte, r *RMC) ([]byte, error) {
	if r == nil {
		return dst, dmrgo.ErrNilArgument
	}
	if !r.valid() {
		return dst, ErrField
	}
	talker := r.Talker
	if talker == [2]byte{} {
		talker = [2]byte{'G', 'P'}
	}
	n := len(dst)
	dst = append(dst, '$', talker[0], talker[1], 'R', 'M', 'C', ',')
	if r.HasTime {
		dst = appendDigits(dst, uint64(r.Hour), 2)
		dst = appendDigits(dst, uint64(r.Minute), 2)
		dst = appendDigits(dst, uint64(r.Second), 2)
		dst = append(dst, '.')
		dst = appendDigits(dst, uint64(r.Millisecond), 3)
	}
	status := byte('V')
	if r.Valid {
		status = 'A'
	}
	dst = append(dst, ',', status, ',')
	if r.HasPosition {
		dst = appendCoordinate(dst, r.Latitude, 2, 'N', 'S')
		dst = append(dst, ',')
		dst = appendCoordinate(dst, r.Longitude, 3, 'E', 'W')
	} else {
		dst = append(dst, ',', ',', ',')
	}
	dst = append(dst, ',')
	if r.HasSpeed {
		dst = appendFixed(dst, r.Speed, 100, 1)
	}
	dst = append(dst, ',')
	if r.HasCourse {
		dst = appendFixed(dst, r.Course, 100, 1)
	}
	dst = append(dst, ',')
	if r.HasDate {
		dst = appendDigits(dst, uint64(r.Day), 2)
		dst = appendDigits(dst, uint64(r.Month), 2)
		dst = appendDigits(dst, uint64(r.Year-2000), 2)
	}
	dst = append(dst, ',')
	switch v := math.Round(r.MagneticVariation*10) / 10; {
	case v > 0:
		dst = append(appendFixed(dst, v, 10, 3), ',', 'E')
	case v < 0:
		dst = append(appendFixed(dst, -v, 10, 3), ',', 'W')
	default:
		dst = append(dst, ',')
	}
	if r.Mode != 0 {
		dst = append(dst, ',', r.Mode)
	}
	var sum byte
	for _, c := range dst[n+1:] {
		sum ^= c
	}
	const hex = "0123456789ABCDEF"
	return append(dst, '*', hex[sum>>4], hex[sum&0x0F], '\r', '\n'), nil
}

func (r *RMC) valid() bool {
	if r.Talker != [2]byte{} && (!talkerByte(r.Talker[0]) || !talkerByte(r.Talker[1])) {
		return false
	}
	if r.Mode != 0 && !upper(r.Mode) {
		return false
	}
	if r.HasTime && (r.Hour > 23 || r.Minute > 59 || r.Second > 60 || r.Millisecond > 999) {
		return false
	}
	if r.HasPosition && (!(r.Latitude >= -90 && r.Latitude <= 90) || !(r.Longitude >= -180 && r.Longitude <= 180)) {
		return false
	}
	if r.HasSpeed && !(r.Speed >= 0 && r.Speed < maxSpeed) {
		return false
	}
	if r.HasCourse && !(r.Course >= 0 && r.Course <= 360) {
		return false
	}
	if r.HasDate && (r.Day < 1 || r.Day > 31 || r.Month < 1 || r.Month > 12 || r.Year < 2000 || r.Year > 2099) {
		return false
	}
	return r.MagneticVariation >= -180 && r.MagneticVariation <= 180
}

func talkerByte(c byte) bool {
	return c >= 0x20 && c <= 0x7e && c != '$' && c != '*' && c != ','
}

func upper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func appendCoordinate(dst []byte, v float64, degDigits int, pos, neg byte) []byte {
	hemi := pos
	if v < 0 {
		hemi, v = neg, -v
	}
	const perDegree = 60 * 10000
	t := uint64(math.Round(v * perDegree))
	dst = appendDigits(dst, t/perDegree, degDigits)
	t %= perDegree
	dst = appendDigits(dst, t/10000, 2)
	dst = append(dst, '.')
	dst = appendDigits(dst, t%10000, 4)
	return append(dst, ',', hemi)
}

func appendFixed(dst []byte, v float64, scale uint64, wholeDigits int) []byte {
	t := uint64(math.Round(v * float64(scale)))
	dst = appendDigits(dst, t/scale, wholeDigits)
	dst = append(dst, '.')
	frac := 0
	for s := scale; s > 1; s /= 10 {
		frac++
	}
	return appendDigits(dst, t%scale, frac)
}

func appendDigits(dst []byte, v uint64, width int) []byte {
	var buf [20]byte
	i := len(buf)
	for v > 0 || len(buf)-i < width {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return append(dst, buf[i:]...)
}
