package impulse

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/types"
)

// Solver owns the live impulse map. The workload calls it after signal and
// logic producers and before the agent. Its lock protects read-only UI snapshots.
type Solver struct {
	*grid.Space
	Mutex  sync.Mutex
	Latest map[string]grid.Impulse
	err    error
}

func NewSolver() *Solver {
	return &Solver{Space: grid.NewSpace(), Latest: make(map[string]grid.Impulse)}
}

func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	solver.Mutex.Lock()
	defer solver.Mutex.Unlock()
	envelope.Impulses = envelope.Impulses[:0]
	groups := make(map[string][]*data.Measurement[float64])
	for _, measurement := range envelope.Measurements() {
		if measurement != nil {
			groups[measurement.Label] = append(groups[measurement.Label], measurement)
		}
	}
	for label, measurements := range groups {
		at, from := measurements[0].At, measurements[0].From
		for _, measurement := range measurements {
			if measurement.At.After(at) {
				at = measurement.At
			}

			if !measurement.From.IsZero() && (from.IsZero() || measurement.From.Before(from)) {
				from = measurement.From
			}
		}

		if err := solver.Space.Step(measurements); err != nil {
			solver.err = errnie.Error(err)
			return envelope
		}
		impulse, err := solver.Space.Impulse(label, at, from)

		if err != nil {
			solver.err = errnie.Error(err)
			return envelope
		}
		solver.Latest[label] = impulse
		envelope.Impulses = append(envelope.Impulses, impulse)
	}
	return envelope
}

func (solver *Solver) Error() error {
	solver.Mutex.Lock()
	defer solver.Mutex.Unlock()
	return solver.err
}
