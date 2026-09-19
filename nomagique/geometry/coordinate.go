package geometry

import "github.com/theapemachine/symm/nomagique/types"

/*
NewCoordinate creates a Value closure that holds an addressable 2D position.
When called with [2]int{0, 0}, it returns the current coordinate.
When called with a non-zero [2]int{x, y}, it updates and returns the new coordinate.
No structs, pure Value closure.
*/
type Coordinate types.Value[[2]int, [2]int]
func NewCoordinate(x, y types.Integer) Coordinate {
	initX, initY := 0, 0
	if x != nil {
		initX = x(nil)
	}
	if y != nil {
		initY = y(nil)
	}
	coord := [2]int{initX, initY}
	return func(update [2]int) [2]int {
		if update != [2]int{0, 0} {
			coord = update
		}
		return coord
	}
}
