package geometry

import "github.com/theapemachine/symm/nomagique/types"

type Intersection types.Value[[2][2]int64, bool]
/*
NewIntersection returns true if two 1D intervals overlap.
Intervals are passed as primitive arrays [from, to] to avoid bespoke DTOs.
*/
func NewIntersection() Intersection {
	return func(intervals [2][2]int64) bool {
		left, right := intervals[0], intervals[1]
		
		// overlap condition: leftFrom < rightTo && rightFrom < leftTo
		return left[0] < right[1] && right[0] < left[1]
	}
}
