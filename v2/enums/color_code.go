package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// ColorCode is the 4-bit color code of a slot type or EMB (ETSI TS 102 361-1
// clause 9.3.1). A ColorCode from NewColorCode is 0 to 15. The encoders,
// setters and builders that take a ColorCode return a
// *dmrgo.FieldRangeError for one above 15.
type ColorCode uint8

// NewColorCode returns v as a ColorCode. It returns a *ColorCodeError when v
// is above 15. It does not allocate.
func NewColorCode(v uint8) (ColorCode, error) {
	if v > 15 {
		return 0, &colorCodeErrors[v-16]
	}
	return ColorCode(v), nil
}

// ColorCodeError is the error of NewColorCode for a value that does not fit
// in 4 bits.
type ColorCodeError struct {
	// Value is the value given to NewColorCode.
	Value uint8
}

// Error returns the message of e. It allocates the message.
func (e *ColorCodeError) Error() string {
	return "color code " + text.Int(int64(e.Value)) + " does not fit in 4 bits"
}
