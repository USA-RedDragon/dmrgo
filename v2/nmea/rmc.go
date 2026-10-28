// Package nmea decodes and encodes the NMEA 0183 RMC sentence that some
// radios send as a position report in a DMR data packet, such as the $GPRMC
// sentence of an AnyTone radio in a UDP datagram. It does no I/O and does
// not allocate except to grow a slice of the caller.
package nmea

// RMC is a Recommended Minimum Specific GNSS Data sentence ($--RMC). An
// empty field gives 0, and the Has fields tell an empty field from 0.
type RMC struct {
	// Talker is the talker ID, such as "GP" for GPS or "GN" for more than
	// one satellite system.
	Talker [2]byte
	// Hour is the hour of the UTC time of the fix.
	Hour uint8
	// Minute is the minute of the UTC time of the fix.
	Minute uint8
	// Second is the second of the UTC time of the fix, 60 for a leap second.
	Second uint8
	// Millisecond is the fraction of Second, from the first three digits
	// after the decimal point.
	Millisecond uint16
	// HasTime reports a UTC time field that is not empty.
	HasTime bool
	// Valid reports status A. Status V, a warning of the receiver, gives
	// false.
	Valid bool
	// Latitude is in degrees, north positive.
	Latitude float64
	// Longitude is in degrees, east positive.
	Longitude float64
	// HasPosition reports a latitude and a longitude that are not empty.
	HasPosition bool
	// Speed is the speed over ground in knots.
	Speed float64
	// HasSpeed reports a speed field that is not empty.
	HasSpeed bool
	// Course is the course over ground in degrees true.
	Course float64
	// HasCourse reports a course field that is not empty.
	HasCourse bool
	// Day is the day of the UTC date, 1 to 31.
	Day uint8
	// Month is the month of the UTC date, 1 to 12.
	Month uint8
	// Year is 2000 plus the two-digit year of the UTC date.
	Year uint16
	// HasDate reports a UTC date field that is not empty.
	HasDate bool
	// MagneticVariation is in degrees, east positive.
	MagneticVariation float64
	// Mode is the mode indicator of NMEA 0183 2.3 and later, such as 'A'
	// for autonomous, or 0 when the sentence has none.
	Mode byte
}

const maxFields = 13

// ParseRMC decodes one RMC sentence from s, from the $ to the two hex
// digits of the checksum, with an optional CR LF or LF after them. It
// accepts any talker ID and the 11, 12 or 13 data fields of NMEA 0183
// versions before 2.3, 2.3 and 4.1. It returns ErrSentence, ErrChecksum or
// ErrField.
func ParseRMC(s []byte) (RMC, error) {
	var r RMC
	n := len(s)
	if n > 0 && s[n-1] == '\n' {
		n--
	}
	if n > 0 && s[n-1] == '\r' {
		n--
	}
	if n == 0 || s[0] != '$' {
		return r, ErrSentence
	}
	var sep spans
	fields := 0
	star := -1
	var sum byte
	for i := 1; i < n; i++ {
		c := s[i]
		if c == '*' {
			star = i
			break
		}
		if c < 0x20 || c > 0x7e || c == '$' {
			return r, ErrSentence
		}
		sum ^= c
		if c == ',' {
			if fields == maxFields {
				return r, ErrSentence
			}
			sep[fields] = i
			fields++
		}
	}
	if star < 0 || star+3 != n {
		return r, ErrChecksum
	}
	hi, okHi := hexDigit(s[star+1])
	lo, okLo := hexDigit(s[star+2])
	if !okHi || !okLo || hi<<4|lo != sum {
		return r, ErrChecksum
	}
	if fields < 11 || sep[0] != 6 || s[3] != 'R' || s[4] != 'M' || s[5] != 'C' {
		return r, ErrSentence
	}
	sep[fields] = star
	r.Talker = [2]byte{s[1], s[2]}
	if err := r.parse(s, &sep, fields); err != nil {
		return RMC{}, err
	}
	return r, nil
}

type spans [maxFields + 1]int

func (p *spans) at(k int) (lo, hi int) {
	return p[k-1] + 1, p[k]
}

func (r *RMC) parse(s []byte, sep *spans, fields int) error {
	lo, hi := sep.at(1)
	if !r.parseTime(s, lo, hi) {
		return ErrField
	}
	lo, hi = sep.at(2)
	switch {
	case hi-lo == 1 && s[lo] == 'A':
		r.Valid = true
	case hi-lo == 1 && s[lo] == 'V':
	default:
		return ErrField
	}
	var ok bool
	lo, hi = sep.at(3)
	hemiLo, hemiHi := sep.at(4)
	if r.Latitude, ok = coordinate(s, lo, hi, hemiLo, hemiHi, 2, 'N', 'S'); !ok {
		return ErrField
	}
	latEmpty := lo == hi
	lo, hi = sep.at(5)
	hemiLo, hemiHi = sep.at(6)
	if r.Longitude, ok = coordinate(s, lo, hi, hemiLo, hemiHi, 3, 'E', 'W'); !ok {
		return ErrField
	}
	r.HasPosition = !latEmpty && lo != hi
	lo, hi = sep.at(7)
	if r.Speed, ok = decimal(s, lo, hi); !ok {
		return ErrField
	}
	r.HasSpeed = lo != hi
	lo, hi = sep.at(8)
	if r.Course, ok = decimal(s, lo, hi); !ok || r.Course > 360 {
		return ErrField
	}
	r.HasCourse = lo != hi
	lo, hi = sep.at(9)
	if !r.parseDate(s, lo, hi) {
		return ErrField
	}
	lo, hi = sep.at(10)
	hemiLo, hemiHi = sep.at(11)
	if r.MagneticVariation, ok = variation(s, lo, hi, hemiLo, hemiHi); !ok {
		return ErrField
	}
	if fields >= 12 {
		lo, hi = sep.at(12)
		if r.Mode, ok = letter(s, lo, hi); !ok {
			return ErrField
		}
	}
	if fields == maxFields {
		lo, hi = sep.at(13)
		if _, ok = letter(s, lo, hi); !ok {
			return ErrField
		}
	}
	return nil
}

func (r *RMC) parseTime(s []byte, lo, hi int) bool {
	if lo == hi {
		return true
	}
	if hi-lo < 6 {
		return false
	}
	h, okH := twoDigits(s, lo)
	m, okM := twoDigits(s, lo+2)
	sec, okS := twoDigits(s, lo+4)
	if !okH || !okM || !okS || h > 23 || m > 59 || sec > 60 {
		return false
	}
	r.Hour, r.Minute, r.Second, r.HasTime = h, m, sec, true
	if hi-lo == 6 {
		return true
	}
	if s[lo+6] != '.' || hi-lo == 7 {
		return false
	}
	ms, scale := 0, 100
	for i := lo + 7; i < hi; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		ms += int(c-'0') * scale
		scale /= 10
	}
	r.Millisecond = uint16(ms)
	return true
}

func (r *RMC) parseDate(s []byte, lo, hi int) bool {
	if lo == hi {
		return true
	}
	d, okD := twoDigits(s, lo)
	m, okM := twoDigits(s, lo+2)
	y, okY := twoDigits(s, lo+4)
	if hi-lo != 6 || !okD || !okM || !okY || d < 1 || d > 31 || m < 1 || m > 12 {
		return false
	}
	r.Day, r.Month, r.Year, r.HasDate = d, m, 2000+uint16(y), true
	return true
}

func coordinate(s []byte, lo, hi, hemiLo, hemiHi, degDigits int, pos, neg byte) (float64, bool) {
	if lo == hi {
		return 0, hemiLo == hemiHi
	}
	if hemiHi-hemiLo != 1 || (s[hemiLo] != pos && s[hemiLo] != neg) {
		return 0, false
	}
	whole := lo
	for whole < hi && s[whole] != '.' {
		whole++
	}
	if whole-lo != degDigits+2 {
		return 0, false
	}
	deg, ok := digits(s, lo, degDigits)
	if !ok {
		return 0, false
	}
	minutes, ok := decimal(s, lo+degDigits, hi)
	if !ok || minutes >= 60 {
		return 0, false
	}
	v := float64(deg) + minutes/60
	if v > float64(90*(degDigits-1)) {
		return 0, false
	}
	if s[hemiLo] == neg {
		v = -v
	}
	return v, true
}

func variation(s []byte, lo, hi, hemiLo, hemiHi int) (float64, bool) {
	if lo == hi {
		return 0, hemiLo == hemiHi
	}
	v, ok := decimal(s, lo, hi)
	if !ok || v > 180 || hemiHi-hemiLo != 1 {
		return 0, false
	}
	switch s[hemiLo] {
	case 'E':
		return v, true
	case 'W':
		return -v, true
	}
	return 0, false
}

func decimal(s []byte, lo, hi int) (float64, bool) {
	if lo == hi {
		return 0, true
	}
	var mant uint64
	n, frac := 0, -1
	for i := lo; i < hi; i++ {
		c := s[i]
		switch {
		case c == '.' && frac < 0:
			frac = 0
		case c >= '0' && c <= '9':
			if n == 18 {
				return 0, false
			}
			mant = mant*10 + uint64(c-'0')
			n++
			if frac >= 0 {
				frac++
			}
		default:
			return 0, false
		}
	}
	if n == 0 {
		return 0, false
	}
	scale := 1.0
	for range max(frac, 0) {
		scale *= 10
	}
	return float64(mant) / scale, true
}

func digits(s []byte, lo, n int) (int, bool) {
	if lo+n > len(s) {
		return 0, false
	}
	v := 0
	for i := lo; i < lo+n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, true
}

func twoDigits(s []byte, lo int) (uint8, bool) {
	if lo+2 > len(s) || s[lo] < '0' || s[lo] > '9' || s[lo+1] < '0' || s[lo+1] > '9' {
		return 0, false
	}
	return (s[lo]-'0')*10 + s[lo+1] - '0', true
}

func letter(s []byte, lo, hi int) (byte, bool) {
	switch {
	case lo == hi:
		return 0, true
	case hi-lo == 1 && s[lo] >= 'A' && s[lo] <= 'Z':
		return s[lo], true
	}
	return 0, false
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}
