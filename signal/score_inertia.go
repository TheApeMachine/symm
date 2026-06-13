package signal

import (
	"math"

	"github.com/theapemachine/symm/logic"
)

/*
DirectionalScoreInertia resists reversing a published score until consecutive
raw readings push in the new direction for effortThreshold observations. Once
moving in a direction, same-direction updates apply freely.
*/
type DirectionalScoreInertia struct {
	published       float64
	momentum        int
	effort          int
	desireDirection int
	initialized     bool
}

type symbolScoreInertia struct {
	confidence DirectionalScoreInertia
	surprise   DirectionalScoreInertia
}

/*
Apply returns the inertia-smoothed score. effortThreshold is the number of
consecutive same-direction raw readings required to start or reverse movement.
*/
func (inertia *DirectionalScoreInertia) Apply(raw float64, effortThreshold int) float64 {
	if effortThreshold <= 0 {
		return raw
	}

	if !scoreInertiaFinitePositive(raw) {
		if inertia.initialized {
			return inertia.published
		}

		return raw
	}

	if !inertia.initialized {
		inertia.published = raw
		inertia.initialized = true

		return raw
	}

	if raw == inertia.published {
		inertia.effort = 0
		inertia.desireDirection = 0

		return inertia.published
	}

	if raw > inertia.published {
		return inertia.applyUp(raw, effortThreshold)
	}

	return inertia.applyDown(raw, effortThreshold)
}

func (inertia *DirectionalScoreInertia) applyUp(raw float64, effortThreshold int) float64 {
	if inertia.momentum == 1 {
		inertia.published = raw
		inertia.effort = 0
		inertia.desireDirection = 0

		return inertia.published
	}

	if inertia.desireDirection != 1 {
		inertia.effort = 0
		inertia.desireDirection = 1
	}

	inertia.effort++

	if inertia.effort >= effortThreshold {
		inertia.momentum = 1
		inertia.published = raw
		inertia.effort = 0
		inertia.desireDirection = 0
	}

	return inertia.published
}

func (inertia *DirectionalScoreInertia) applyDown(raw float64, effortThreshold int) float64 {
	if inertia.momentum == -1 {
		inertia.published = raw
		inertia.effort = 0
		inertia.desireDirection = 0

		return inertia.published
	}

	if inertia.desireDirection != -1 {
		inertia.effort = 0
		inertia.desireDirection = -1
	}

	inertia.effort++

	if inertia.effort >= effortThreshold {
		inertia.momentum = -1
		inertia.published = raw
		inertia.effort = 0
		inertia.desireDirection = 0
	}

	return inertia.published
}

func resolveScoreInertiaEffort() int {
	return ScoreInertiaEffort()
}

func scoreInertiaFinitePositive(value float64) bool {
	if value <= 0 {
		return false
	}

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}

	return true
}

func (system *System) scoreInertiaFor(symbol string) *symbolScoreInertia {
	if raw, ok := system.scoreInertia.Load(symbol); ok {
		state, stateOK := raw.(*symbolScoreInertia)

		if stateOK {
			return state
		}
	}

	created := &symbolScoreInertia{}
	actual, _ := system.scoreInertia.LoadOrStore(symbol, created)

	state, stateOK := actual.(*symbolScoreInertia)

	if !stateOK {
		return created
	}

	return state
}

func (system *System) applyScoreInertia(
	measurement logic.Measurement,
	state *symbolScoreInertia,
) logic.Measurement {
	effortThreshold := system.scoreInertiaEffort

	measurement.Confidence = state.confidence.Apply(measurement.Confidence, effortThreshold)
	measurement.Surprise = state.surprise.Apply(measurement.Surprise, effortThreshold)

	return measurement
}
