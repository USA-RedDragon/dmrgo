// Package nofuzz is a machine like package machine, but no fuzz test checks
// its MaxOut bound.
package nofuzz

// MaxOut is the most outputs that Run appends.
const MaxOut = 2

// Out is one output.
type Out struct{ N int }

// Machine is a machine.
type Machine struct{}

// Run appends one output to dst.
func (m *Machine) Run(dst []Out) []Out {
	return append(dst, Out{N: 1})
}
