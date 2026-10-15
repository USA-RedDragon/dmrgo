package enums

import "fmt"

// ColorCode is the 4-bit Colour Code of a slot type or EMB (ETSI TS 102
// 361-1 §9.3.1). NewColorCode checks the range, so a ColorCode from it is 0
// to 15. The setters and builders that take a ColorCode write its low 4
// bits and do not return an error for it.
type ColorCode uint8

// NewColorCode returns v as a ColorCode. It returns a *ColorCodeError when v
// is above 15.
func NewColorCode(v uint8) (ColorCode, error) {
	if v > 15 {
		return 0, &ColorCodeError{Value: v}
	}
	return ColorCode(v), nil
}

// ColorCodeError is the error of NewColorCode for a value that does not fit
// in 4 bits.
type ColorCodeError struct {
	// Value is the value given to NewColorCode.
	Value uint8
}

func (e *ColorCodeError) Error() string {
	return fmt.Sprintf("color code %d does not fit in 4 bits", e.Value)
}
