package geometry

import "github.com/theapemachine/symm/nomagique/types"

/*
NewWeight creates a Value closure that scales incoming values by configured strength.
No structs, pure Value closure.
*/
type Weight types.Value[float64, float64]
func NewWeight(strength, direction float64) Weight {
	return func(in float64) float64 {
		return in * strength
	}
}
