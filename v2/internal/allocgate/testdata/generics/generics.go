// Package generics holds generic code that the gate cannot follow, in the
// Hole functions, and generic code that it can, in the Fine functions.
package generics

// Box holds a value.
type Box[T any] struct{ v T }

// HoleGet is an exported method of a generic type.
func (b *Box[T]) HoleGet() T { return b.v }

// HoleIdentity is an exported generic function.
func HoleIdentity[T any](v T) T { return v }

type stringer interface{ String() string }

func holeString[T stringer](v T) string { return v.String() }

func holeApply[T any](v T, f func(T) T) T { return f(v) }

func fineSum[T int | int64](s []T) T {
	var sum T
	for _, v := range s {
		sum += v
	}
	return sum
}

// FineUse instantiates the unexported generic functions.
func FineUse(s []int, v named) (int, string, named) {
	return fineSum(s), holeString(v), holeApply(v, func(n named) named { return n })
}

type named int

func (n named) String() string { return "n" }
