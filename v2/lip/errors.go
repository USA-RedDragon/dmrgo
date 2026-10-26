package lip

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// Errors returned by the package. Each is a static value, so a call that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when a PDU or an information element ends
	// before the bits that it needs.
	ErrShort = errors.New("lip: PDU too short")
	// ErrPDUType is returned for a reserved PDU type, and by a decoder for
	// a PDU of another type or extension.
	ErrPDUType = errors.New("lip: wrong PDU type")
	// ErrShape is returned for a reserved location shape or the location
	// shape extension, whose coding the spec does not define.
	ErrShape = errors.New("lip: location shape not defined")
	// ErrTimeType is returned for the reserved time type 3.
	ErrTimeType = errors.New("lip: time type not defined")
	// ErrElement is returned for a type 5 element with the extended
	// identifier 31 or the reserved length extension 0, whose length the
	// spec does not define, and by AppendElement for a data length out of
	// range.
	ErrElement = errors.New("lip: type 5 element not defined")
)

const (
	typeTime     = "Time"
	typeLocation = "Location"
	typeVelocity = "Velocity"
)

var (
	errTimeElapsed  = &dmrgo.FieldRangeError{Type: typeTime, Field: "Elapsed", Bits: 2}
	errTimeDay      = &dmrgo.FieldRangeError{Type: typeTime, Field: "Day", Bits: 5}
	errTimeHour     = &dmrgo.FieldRangeError{Type: typeTime, Field: "Hour", Bits: 5}
	errTimeMinute   = &dmrgo.FieldRangeError{Type: typeTime, Field: "Minute", Bits: 6}
	errTimeSecond   = &dmrgo.FieldRangeError{Type: typeTime, Field: "Second", Bits: 6}
	errLongitude    = &dmrgo.FieldRangeError{Type: typeLocation, Field: "Longitude", Bits: 25, Signed: true, Min: -180, Max: 180}
	errLatitude     = &dmrgo.FieldRangeError{Type: typeLocation, Field: "Latitude", Bits: 24, Signed: true, Min: -90, Max: 90}
	errUncertainty  = &dmrgo.FieldRangeError{Type: typeLocation, Field: "Uncertainty", Bits: 6}
	errHalfMajor    = &dmrgo.FieldRangeError{Type: typeLocation, Field: "HalfMajor", Bits: 6}
	errHalfMinor    = &dmrgo.FieldRangeError{Type: typeLocation, Field: "HalfMinor", Bits: 6}
	errConfidence   = &dmrgo.FieldRangeError{Type: typeLocation, Field: "Confidence", Bits: 3}
	errAltitude     = &dmrgo.FieldRangeError{Type: typeLocation, Field: "Altitude", Bits: 11}
	errAltitudeUnc  = &dmrgo.FieldRangeError{Type: typeLocation, Field: "AltitudeUncertainty", Bits: 3}
	errPosition     = &dmrgo.FieldRangeError{Type: typeLocation, Field: "PositionError", Bits: 3}
	errHorizontal   = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "Horizontal", Bits: 7}
	errHorizUnc     = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "HorizontalUncertainty", Bits: 3}
	errVertical     = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "Vertical", Bits: 7}
	errVerticalUnc  = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "VerticalUncertainty", Bits: 3}
	errDirectionUnc = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "DirectionUncertainty", Bits: 3}
	errVelocityType = &dmrgo.FieldRangeError{Type: typeVelocity, Field: "Type", Bits: 3}
	errReportType   = &dmrgo.FieldRangeError{Type: "ImmediateRequest", Field: "ReportType", Bits: 2}
)
