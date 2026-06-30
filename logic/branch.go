package logic

import (
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/statutil"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches" json:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group" json:"condition_group"`
	Action         *Action         `yaml:"action" json:"action"`

	// stage tracks cross-tick sequential matches for this branch.
	// Initialized lazily on first evaluation of a branch with children.
	mu           sync.Mutex
	stage        *stageMemory
	confirmation *confirmationMemory
}

/*
Evaluate returns every candidate action this branch subtree yields. The playbook
proposes; it does not choose, so a branch whose children each match contributes
all their actions. A nil/empty slice means the branch did not fire. Sequential
parents arm a cadence-derived stage and only let children fire on a strictly
later batch — "stage A, THEN stage B".
*/
func (branch *Branch) Evaluate(
	measurements []*datura.Artifact, holdings *datura.Artifact,
) ([]*Action, error) {
	now := tickTime(measurements)
	symbol := symbolFromMeasurements(measurements)

	if branch.ConditionGroup != nil {
		matched, evaluateErr := branch.conditionMatched(
			measurements,
			holdings,
			now,
			symbol,
		)

		if evaluateErr != nil {
			return nil, evaluateErr
		}

		// Sequential parent: has children but no direct action. When the
		// condition matches, record it per-symbol with a cadence-derived
		// validity window so children can evaluate on a LATER tick's
		// measurements (cross-tick sequencing).
		if len(branch.Branches) > 0 && branch.Action == nil {
			if matched {
				branch.ensureStage().record(symbol, now, stageWindow(measurements))
			}

			// Children fire only when the parent matched on an EARLIER batch
			// (strictly older timestamp): the sequence is "stage A, THEN stage
			// B", never both on the same instant. A parent that only just
			// matched this tick parks and waits for the next batch.
			if !branch.ensureStage().matchedBefore(symbol, now) {
				return nil, nil
			}
		} else if !matched {
			return nil, nil
		}
	}

	if branch.Action != nil {
		action, actionErr := actionForMatch(
			branch.Action,
			symbol,
			measurements,
			branch.ConditionGroup,
		)

		if actionErr != nil {
			return nil, actionErr
		}

		return []*Action{action}, nil
	}

	actions := make([]*Action, 0, len(branch.Branches))

	for _, child := range branch.Branches {
		childActions, err := child.Evaluate(measurements, holdings)

		if err != nil {
			return nil, err
		}

		actions = append(actions, childActions...)
	}

	// A completed sequence clears this branch's stage so it must re-arm from the
	// first stage rather than re-firing next tick from a stale match.
	if len(actions) > 0 {
		branch.ensureStage().clear(symbol)
	}

	return actions, nil
}

func (branch *Branch) ensureStage() *stageMemory {
	branch.mu.Lock()
	defer branch.mu.Unlock()

	if branch.stage == nil {
		branch.stage = newStageMemory()
	}

	return branch.stage
}

func (branch *Branch) ensureConfirmation() *confirmationMemory {
	branch.mu.Lock()
	defer branch.mu.Unlock()

	if branch.confirmation == nil {
		branch.confirmation = newConfirmationMemory()
	}

	return branch.confirmation
}

func (branch *Branch) conditionMatched(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	now time.Time,
	symbol string,
) (bool, error) {
	matched, evaluateErr := branch.ConditionGroup.Evaluate(
		measurements,
		holdings,
	)

	if evaluateErr != nil {
		return false, evaluateErr
	}

	minObservations := branch.ConditionGroup.MinObservations
	if minObservations <= 1 {
		return matched, nil
	}

	return branch.ensureConfirmation().observe(
		symbol,
		now,
		matched,
		minObservations,
	), nil
}

/*
tickTime extracts a representative timestamp from the current measurement batch.
The batch timestamp anchors all sequential interval math, so sequencing is driven
by when the data was observed, never by wall-clock at evaluation time.
*/
func tickTime(measurements []*datura.Artifact) time.Time {
	latest := int64(0)

	for _, measurement := range measurements {
		if stamp := measurement.Timestamp(); stamp > latest {
			latest = stamp
		}
	}

	if latest == 0 {
		return time.Now()
	}

	return time.Unix(0, latest)
}

/*
stageWindow derives how long a matched sequential stage stays armed, from the
cadence of this batch's measurement timestamps. The window is a budget of median
inter-measurement intervals (statutil.WindowDepth × the median gap) so a stage on
a fast pair expires sooner in wall-clock than the same stage on a slow one — the
sequence tolerance scales with how often the symbol actually speaks. With too few
distinct stamps to form a cadence the window is zero: the match is valid only on
its own instant, so a sequence cannot fire from a single lonely observation.
*/
func stageWindow(measurements []*datura.Artifact) time.Duration {
	stamps := make([]float64, 0, len(measurements))

	for _, measurement := range measurements {
		if stamp := measurement.Timestamp(); stamp > 0 {
			stamps = append(stamps, float64(stamp))
		}
	}

	cadence := statutil.MedianCadence(stamps)

	if cadence <= 0 {
		return 0
	}

	depth := statutil.WindowDepth(stamps)

	if depth < 1 {
		depth = 1
	}

	return time.Duration(cadence * float64(depth))
}
