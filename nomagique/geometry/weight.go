package geometry

import "github.com/theapemachine/symm/nomagique/types"

/*
NewWeight creates a Value closure that scales incoming values by configured strength.
No structs, pure Value closure.
*/
type Weight types.Value[float64, float64]
func NewWeight(strength, direction types.Float) Weight {
	return func(in float64) float64 {
		s := 1.0
		if strength != nil {
			s = strength(in)
		}
		return in * s
	}
}
