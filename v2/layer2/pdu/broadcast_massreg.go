package pdu

import "time"

// RegWindowTime returns TReg_Window, the time in which an MS makes its
// registration for the Reg_Window w, and false for 0, which cancels the
// mass registration, or a w over 15 (ETSI TS 102 361-4 clause 6.4.5.1,
// Table 6.12).
func RegWindowTime(w uint8) (time.Duration, bool) {
	switch w {
	case 1:
		return 500 * time.Millisecond, true
	case 2:
		return time.Second, true
	case 3:
		return 2 * time.Second, true
	case 4:
		return 5 * time.Second, true
	case 5:
		return 10 * time.Second, true
	case 6:
		return 20 * time.Second, true
	case 7:
		return 30 * time.Second, true
	case 8:
		return 100 * time.Second, true
	case 9:
		return 300 * time.Second, true
	case 10:
		return 1000 * time.Second, true
	case 11:
		return 3000 * time.Second, true
	case 12:
		return 10000 * time.Second, true
	case 13:
		return 30000 * time.Second, true
	case 14:
		return 100000 * time.Second, true
	case 15:
		return 200000 * time.Second, true
	}
	return 0, false
}
