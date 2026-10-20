// Package engine passes slices of its fields to the growers of other
// machines. The Grow methods pass a field that NewEngine makes with the Max
// capacity of a machine whose fuzz test checks that bound, which the dst
// growth rule must accept. The Bad methods break one condition, so the rule
// must reject them.
package engine

import (
	"strconv"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/testdata/scratch/machine"
	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/testdata/scratch/nofuzz"
)

// Engine holds the machines and their scratch slices.
type Engine struct {
	m      machine.Machine
	n      nofuzz.Machine
	buf    []machine.Out
	nbuf   []nofuzz.Out
	small  []machine.Out
	stored []machine.Out
	set    []machine.Out
	full   []machine.Out
}

// NewEngine returns an Engine with its scratch slices.
func NewEngine() *Engine {
	return &Engine{
		buf:    make([]machine.Out, 0, machine.MaxOut),
		nbuf:   make([]nofuzz.Out, 0, nofuzz.MaxOut),
		small:  make([]machine.Out, 0, 8),
		stored: make([]machine.Out, 0, machine.MaxOut),
		set:    make([]machine.Out, 0, machine.MaxOut),
		full:   make([]machine.Out, 0, machine.MaxOut),
	}
}

// GrowScratch passes buf, made with machine.MaxOut, to machine.Run.
func (e *Engine) GrowScratch(dst []int) []int {
	out := e.m.Run(e.buf[:0])
	return append(dst, len(out))
}

// BadNotMax passes small, made with a literal capacity.
func (e *Engine) BadNotMax(dst []int) []int {
	out := e.m.Run(e.small[:0])
	return append(dst, len(out))
}

// BadNoFuzz passes nbuf to a machine without a fuzz test of its bound.
func (e *Engine) BadNoFuzz(dst []int) []int {
	out := e.n.Run(e.nbuf[:0])
	return append(dst, len(out))
}

// BadStored stores what Run returns back in the field.
func (e *Engine) BadStored(dst []int) []int {
	e.stored = e.m.Run(e.stored[:0])
	return append(dst, len(e.stored))
}

// BadSetLater sets the field outside NewEngine too.
func (e *Engine) BadSetLater(dst []int) []int {
	e.set = nil
	out := e.m.Run(e.set[:0])
	return append(dst, len(out))
}

// BadNotEmpty passes the field with its length.
func (e *Engine) BadNotEmpty(dst []int) []int {
	out := e.m.Run(e.full)
	return append(dst, len(out))
}

// Name returns n as text, so that the program has the runtime functions
// that the gate expects.
func Name(n int) string {
	return strconv.Itoa(n)
}
