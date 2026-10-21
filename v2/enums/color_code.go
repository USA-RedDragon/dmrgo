package enums

import "github.com/USA-RedDragon/dmrgo/v2"

// ColorCode is the 4-bit color code of a slot type or EMB. A ColorCode from
// NewColorCode is 0 to 15. The encoders, setters and builders that take a
// ColorCode return a *dmrgo.FieldRangeError for one above 15.
// (ETSI TS 102 361-1 clause 9.3.1)
type ColorCode uint8

// NewColorCode returns v as a ColorCode. It returns a *dmrgo.FieldRangeError
// when v is above 15. It does not allocate.
func NewColorCode(v uint8) (ColorCode, error) {
	if v > 15 {
		return 0, errColorCodeRange
	}
	return ColorCode(v), nil
}

var errColorCodeRange = &dmrgo.FieldRangeError{Type: "enums", Field: "ColorCode", Bits: 4}
