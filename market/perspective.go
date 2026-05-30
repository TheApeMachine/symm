package market

import (
	"github.com/theapemachine/symm/market/perspectives"
)

var (
	perspectivemap = map[string]perspectives.Perspective{
		"pump": perspectives.NewPumpPerspective(),
	}
)

/*
NewPerspective creates a perspective shell with room for measurements.
Tree is nil until a playbook is attached (see NewPumpPerspective in market).
*/
func NewPerspective(measurements []perspectives.Measurement) perspectives.Perspective {
	capacity := len(measurements)

	if capacity == 0 {
		capacity = 16
	}

	for _, perspective := range perspectivemap {
		if found := perspective.Walk(measurements); found != nil {
			return perspective
		}
	}

	return nil
}
