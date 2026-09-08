package strategy

import (
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
MaxTemporalContextDepth bounds how many distinct grid states one decision is
conditioned on. A market develops through stages — a precursor state, an
ignition, an exhaustion — and the agent needs enough of that progression to
recognize which stage it is in. It does not need all of it: an unbounded window
produces a token sequence that occurs exactly once, so the model trie interns a
branch no later observation can ever match and no record reaches a second
sample. Six retained transitions keep A to B to C addressable while leaving the
sequence short enough to recur.
*/
const MaxTemporalContextDepth = 6

/*
Context retains changes of grid condition within the input producers' actual
observation horizon. Repeated identical conditions do not add model depth.
*/
type Context struct {
	At         time.Time
	Conditions []uint64
}

type Development struct {
	Symbol    string
	At        time.Time
	From      time.Time
	Updates   uint64
	Decisions uint64
	Regions   []grid.Region
	History   []Context
}

func (development *Development) Context(
	at time.Time, measurements []*data.Measurement[float64],
) []uint64 {
	from := at

	for _, measurement := range measurements {
		if !measurement.From.IsZero() && measurement.From.Before(from) {
			from = measurement.From
		}
	}

	development.From = from
	first := 0

	for first < len(development.History) && development.History[first].At.Before(from) {
		first++
	}

	development.History = slices.Delete(development.History, 0, first)

	// The producer's horizon is the outer bound; the retained depth is the
	// inner one. Advance already collapses repeated identical states, so a
	// stagnant market spends no depth and cannot push a precursor out of view.
	if excess := len(development.History) - MaxTemporalContextDepth; excess > 0 {
		development.History = slices.Delete(development.History, 0, excess)
	}
	var tokens []uint64

	for _, context := range development.History {
		tokens = append(tokens, uint64(1)<<63|2) // Structural boundary between observation contexts.
		tokens = append(tokens, context.Conditions...)
	}

	return tokens
}

func (development *Development) Advance(at time.Time, space *grid.Space) error {
	regions, _, err := space.Regions(development.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[development] failed to get regions",
			err,
		))
	}

	development.Regions = regions
	conditions := make([]uint64, 0, len(regions))

	for _, region := range regions {
		conditions = append(conditions, region.Condition)
	}

	development.At, development.Updates = at, development.Updates+1

	if len(development.History) > 0 && slices.Equal(
		development.History[len(development.History)-1].Conditions, conditions,
	) {

		return nil
	}

	development.History = append(development.History, Context{At: at, Conditions: conditions})

	// Retention is bounded here as well so the history cannot grow past the
	// model's addressable depth regardless of how the owner interleaves calls.
	if excess := len(development.History) - MaxTemporalContextDepth; excess > 0 {
		development.History = slices.Delete(development.History, 0, excess)
	}
	return nil
}
