package logic

import (
	"time"

	"github.com/theapemachine/datura"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches" json:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group" json:"condition_group"`
	Action         *Action         `yaml:"action" json:"action"`

	// stage tracks cross-tick sequential matches for this branch.
	// Initialized lazily on first evaluation of a branch with children.
	stage *stageMemory
}

func (branch *Branch) Evaluate(
	measurements []*datura.Artifact,
	holdings *Balances,
) (*Action, error) {
	now := tickTime(measurements)
	symbol := symbolFromMeasurements(measurements)

	if branch.ConditionGroup != nil {
		matched, evaluateErr := branch.ConditionGroup.Evaluate(
			measurements,
			holdings,
		)

		if evaluateErr != nil {
			return nil, evaluateErr
		}

		// Sequential parent: has children but no direct action. When the
		// condition matches, record it per-symbol so children can evaluate
		// on a later tick's measurements (cross-tick sequencing).
		if len(branch.Branches) > 0 && branch.Action == nil {
			if matched {
				branch.ensureStage().record(symbol, now)
			}

			// Even if the parent just matched THIS tick, evaluate children
			// against the current measurements — the child conditions are
			// different sources/categories, so same-tick is valid.
			if !matched && !branch.ensureStage().active(symbol, now) {
				return nil, nil
			}
		} else if !matched {
			return nil, nil
		}
	}

	if branch.Action != nil {
		return branch.Action, nil
	}

	for _, child := range branch.Branches {
		action, err := child.Evaluate(measurements, holdings)

		if err != nil {
			return nil, err
		}

		if action != nil {
			// Clear the parent stage match — the sequence completed.
			branch.ensureStage().clear(symbol)
			return action, nil
		}
	}

	return nil, nil
}

func (branch *Branch) ensureStage() *stageMemory {
	if branch.stage == nil {
		branch.stage = newStageMemory()
	}

	return branch.stage
}

/*
tickTime extracts a representative timestamp from the current measurement batch.
*/
func tickTime(measurements []*datura.Artifact) time.Time {
	for _, measurement := range measurements {
		if stamp := measurement.Timestamp(); stamp > 0 {
			return time.Unix(0, stamp)
		}
	}

	return time.Now()
}

