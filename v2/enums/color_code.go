package enums

import "fmt"

// ColorCode is the 4-bit color code of a slot type or EMB (ETSI TS 102 361-1
// clause 9.3.1). A ColorCode from NewColorCode is 0 to 15. The setters and
// builders that take a ColorCode write its low 4 bits and do not return an
// error for it.
type ColorCode uint8

// NewColorCode returns v as a ColorCode. It returns a *ColorCodeError when v
// is above 15.
func NewColorCode(v uint8) (ColorCode, error) {
	if v > 15 {
		return 0, &colorCodeErrors[v-16]
	}
	return ColorCode(v), nil
}

// colorCodeErrors holds the *ColorCodeError of each value above 15, so that
// NewColorCode does not allocate.
var colorCodeErrors = func() (t [256 - 16]ColorCodeError) {
	for i := range t {
		t[i].Value = uint8(i + 16)
	}
	return t
}()

// ColorCodeError is the error of NewColorCode for a value that does not fit
// in 4 bits.
type ColorCodeError struct {
	// Value is the value given to NewColorCode.
	Value uint8
}

// Error returns the message of e. It allocates the message.
func (e *ColorCodeError) Error() string {
	return fmt.Sprintf("color code %d does not fit in 4 bits", e.Value)
}
