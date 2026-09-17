package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Coordinate represents an addressable point in two dimensions.
// Once used as an ordered store key, it must remain unchanged.
type Coordinate struct {
	*core.PrimitiveError
	X, Y int
}

func NewCoordinate(x, y int) *Coordinate {
	return &Coordinate{PrimitiveError: core.NewPrimitiveError(), X: x, Y: y}
}

func (coordinate *Coordinate) Identity() *Coordinate { return coordinate }

func (coordinate *Coordinate) Identify(address *Coordinate) core.Identifiable[*Coordinate] {
	coordinate.X, coordinate.Y = address.X, address.Y
	return coordinate
}

func (coordinate *Coordinate) Less(other *Coordinate) bool {
	if coordinate.X != other.X {
		return coordinate.X < other.X
	}

	return coordinate.Y < other.Y
}

func (coordinate *Coordinate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if !yield(unsafe.Pointer(&coordinate.X)) {
			return
		}

		yield(unsafe.Pointer(&coordinate.Y))
	}
}
