// Package machine is a machine whose Run appends at most MaxOut outputs,
// which its fuzz test checks.
package machine

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
