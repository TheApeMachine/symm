package strategy

import "github.com/theapemachine/symm/system"

/*
Ignition is the volume-clock floor an opportunity must clear before it
enters the rank queue. Flat and weakly accelerating clocks stay out.
*/
type Ignition struct{}

func NewIgnition() *Ignition {
	return &Ignition{}
}

/*
Clears reports whether clock acceleration meets the configured ignition
floor (and is strictly positive).
*/
func (ignition *Ignition) Clears(accel float64) bool {
	if ignition == nil {
		return false
	}

	floor := 0.0

	if system.Cfg != nil && system.Cfg.Learning != nil {
		floor = system.Cfg.Learning.GateIgnitionFloor
	}

	if floor < 0 {
		floor = 0
	}

	return accel > 0 && accel >= floor
}
