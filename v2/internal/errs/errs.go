// Package errs builds the errors of the Encode methods that dmrgen
// generates. Each error is built in one of these constructors, which are
// not inlined, so the methods themselves have no heap allocation.
package errs

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// NewFieldRange returns a *bit.FieldRangeError for an integer field of
// type typ whose value does not fit in its bits.
//
//go:noinline
func NewFieldRange[T any](typ, field string, value T, bits int, signed bool) error {
	return &bit.FieldRangeError{Type: typ, Field: field, Value: value, Bits: bits, Signed: signed}
}

// NewCoordinateRange returns a *bit.FieldRangeError for a coordinate field
// of type typ whose value is outside [lo, hi].
//
//go:noinline
func NewCoordinateRange[T any](typ, field string, value T, bits int, lo, hi float64) error {
	return &bit.FieldRangeError{Type: typ, Field: field, Value: value, Bits: bits, Signed: true, Min: lo, Max: hi}
}

// NewWrapped returns err wrapped with the prefix "name: ".
//
//go:noinline
func NewWrapped(name string, err error) error {
	return fmt.Errorf("%s: %w", name, err)
}

// NewWrappedIndex returns err wrapped with the prefix "name[i]: ".
//
//go:noinline
func NewWrappedIndex(name string, i int, err error) error {
	return fmt.Errorf("%s[%d]: %w", name, i, err)
}

// NewBothSet returns the error of a PDU of type typ that has two payload
// fields set.
//
//go:noinline
func NewBothSet(typ, first, second string) error {
	return fmt.Errorf("%s: %s and %s are both set", typ, first, second)
}

// NewWrongPayload returns the error of a PDU of type typ whose payload field
// set is not the one that its selector fields select. selects is the text
// that names the selectors, such as "Opcode selects ".
//
//go:noinline
func NewWrongPayload(typ, set, selects, want string) error {
	return fmt.Errorf("%s.%s is set, but %s%s", typ, set, selects, want)
}

// NewNoFECMask returns the error of a field of type typ whose value has no
// FEC mask.
//
//go:noinline
func NewNoFECMask(typ, field string, value uint64) error {
	return fmt.Errorf("%s.%s: no FEC mask for value %d", typ, field, value)
}
