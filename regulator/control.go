package regulator

import (
	"fmt"
	"math"
	"slices"

	"github.com/theapemachine/symm/system"
)

const (
	controlAllocation = iota
	controlConfidence
	controlGraphThreshold
	controlCausalAlpha
	controlIterations
	controlExploration
	controlCount
)

type controlVector [controlCount]float64

type controlBound struct {
	name    string
	minimum float64
	maximum float64
	integer bool
}

/*
controlSpace owns the configured, dimensionally meaningful bounds of every live
actuator the regulator may change.
*/
type controlSpace struct {
	bounds [controlCount]controlBound
}

func newControlSpace(config *system.Config) (*controlSpace, error) {
	if config == nil || config.Planner == nil {
		return nil, fmt.Errorf("regulator: planner configuration required")
	}

	planner := config.Planner

	if planner.MaxAllocationFraction <= 0 || planner.MaxAllocationFraction > 1 {
		return nil, fmt.Errorf("regulator: allocation ceiling must be in (0,1]")
	}

	if planner.MinimumConfidence < system.UninformativeDirectionConfidence ||
		planner.MinimumConfidence > 1 {
		return nil, fmt.Errorf(
			"regulator: confidence gate must be in [%g,1]",
			system.UninformativeDirectionConfidence,
		)
	}

	if planner.MinimumGraphScore < -1 || planner.MinimumGraphScore > 1 {
		return nil, fmt.Errorf("regulator: graph gate must be in [-1,1]")
	}

	if planner.CausalAlpha < 0 || planner.ExplorationConstant < 0 ||
		planner.MCTSIterations < 1 {
		return nil, fmt.Errorf("regulator: valid MCTS controls required")
	}

	return &controlSpace{bounds: [controlCount]controlBound{
		controlAllocation: {
			name: "allocation", minimum: 0,
			maximum: planner.MaxAllocationFraction,
		},
		controlConfidence: {
			name:    "confidence",
			minimum: system.UninformativeDirectionConfidence, maximum: 1,
		},
		controlGraphThreshold: {
			name: "graph_threshold", minimum: -1, maximum: 1,
		},
		controlCausalAlpha: {
			name: "causal_alpha", minimum: 0, maximum: planner.CausalAlpha,
		},
		controlIterations: {
			name: "mcts_iterations", minimum: 1,
			maximum: float64(planner.MCTSIterations), integer: true,
		},
		controlExploration: {
			name: "mcts_exploration", minimum: 0,
			maximum: planner.ExplorationConstant,
		},
	}}, nil
}

func (space *controlSpace) current(config *system.Config) controlVector {
	return controlVector{
		space.normalize(controlAllocation, config.Planner.MaxAllocationFraction),
		space.normalize(controlConfidence, config.Planner.MinimumConfidence),
		space.normalize(controlGraphThreshold, config.Planner.MinimumGraphScore),
		space.normalize(controlCausalAlpha, config.Planner.CausalAlpha),
		space.normalize(controlIterations, float64(config.Planner.MCTSIterations)),
		space.normalize(controlExploration, config.Planner.ExplorationConstant),
	}
}

func (space *controlSpace) candidates(
	current controlVector,
	resolved int,
) []controlVector {
	movable := space.movable()
	candidates := []controlVector{current}

	if len(movable) == 0 {
		return candidates
	}

	completedCycles := resolved / (len(movable) + len(movable))
	step := 1 / math.Sqrt(float64(len(movable)+completedCycles))

	for _, index := range movable {
		for _, direction := range []float64{-1, 1} {
			candidate := current
			candidate[index] = space.quantize(
				index,
				min(1, max(0, current[index]+direction*step)),
			)

			if candidate != current && !slices.Contains(candidates, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates
}

func (space *controlSpace) exploratory(
	current controlVector,
	resolved int,
	intervention int,
) controlVector {
	candidates := space.candidates(current, resolved)

	if len(candidates) == 1 {
		return current
	}

	return candidates[1+intervention%(len(candidates)-1)]
}

func (space *controlSpace) apply(
	controls controlVector,
	config *system.Config,
) error {
	if config == nil || config.Planner == nil {
		return fmt.Errorf("regulator: mutable planner required")
	}

	for index, value := range controls {
		if value < 0 || value > 1 {
			return fmt.Errorf(
				"regulator: normalized %s control must be in [0,1]",
				space.bounds[index].name,
			)
		}
	}

	config.Planner.MaxAllocationFraction = space.value(controlAllocation, controls)
	config.Planner.MinimumConfidence = space.value(controlConfidence, controls)
	config.Planner.MinimumGraphScore = space.value(controlGraphThreshold, controls)
	config.Planner.CausalAlpha = space.value(controlCausalAlpha, controls)
	config.Planner.MCTSIterations = int(math.Round(
		space.value(controlIterations, controls),
	))
	config.Planner.ExplorationConstant = space.value(controlExploration, controls)

	return nil
}

func (space *controlSpace) value(index int, controls controlVector) float64 {
	bound := space.bounds[index]
	value := bound.minimum + controls[index]*(bound.maximum-bound.minimum)

	if bound.integer {
		return math.Round(value)
	}

	return value
}

func (space *controlSpace) normalize(index int, value float64) float64 {
	bound := space.bounds[index]

	if bound.maximum == bound.minimum {
		return 0
	}

	return (value - bound.minimum) / (bound.maximum - bound.minimum)
}

func (space *controlSpace) quantize(index int, normalized float64) float64 {
	bound := space.bounds[index]

	if !bound.integer {
		return normalized
	}

	value := bound.minimum + normalized*(bound.maximum-bound.minimum)
	return space.normalize(index, math.Round(value))
}

func (space *controlSpace) movable() []int {
	movable := make([]int, 0, controlCount)

	for index, bound := range space.bounds {
		if bound.maximum > bound.minimum {
			movable = append(movable, index)
		}
	}

	return movable
}
