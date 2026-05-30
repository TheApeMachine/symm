package market

import (
	"github.com/theapemachine/symm/market/perspectives"
)

var (
	perspectivemap = map[string]perspectives.Perspective{
		"drive": perspectives.NewDrivePerspective(),
		"pump":  perspectives.NewPumpPerspective(),
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

/*
Decide walks every registered perspective against the measurement set and returns
the first actionable verdict — the playbook Action and the perspective that
produced it. Returns (nil, nil) when no perspective is traversable.
*/
func Decide(measurements []perspectives.Measurement) (*perspectives.ActionType, perspectives.Perspective) {
	for _, perspective := range perspectivemap {
		if action := perspective.Decide(measurements); action != nil {
			return action, perspective
		}
	}

	return nil, nil
}
