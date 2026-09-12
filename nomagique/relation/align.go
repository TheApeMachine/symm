package relation

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SeriesView is one predictor series viewed as a resident ring with its own
explicit alignment lag.
*/
type SeriesView struct {
	// History is the read-locked resident ring view (chronological).
	History RingView
	// Lag is the as-of lag: the newest observation used is the newest one
	// available no later than target time minus Lag.
	Lag time.Duration
}

/*
AlignedRow is one target observation aligned with its lagged predictors.
*/
type AlignedRow struct {
	Target     Observation
	Predictors []Observation
}

/*
AlignRequest asks for one target ring aligned with its lagged predictor
series.
*/
type AlignRequest struct {
	Target RingView
	Series []SeriesView
}

/*
AlignResult is the aligned rows of one request, in chronological target order.
*/
type AlignResult struct {
	Rows []AlignedRow
}

/*
Align owns lagged alignment of target observations against predictor series,
all read in place from resident rings. For a target observation at time t and
a predictor series with lag τ, the aligned predictor is the newest
observation available no later than t - τ. Future observations never enter a
row. Only target observations with every predictor aligned are retained.

Preconditions: the target ring and every series ring must be in
non-decreasing chronological order (resident rings are by construction); the
per-series cursor alignment scans each series once across the whole request,
and a later request on the same series (or a continued scan) requires
non-decreasing cutoffs.
*/
type Align struct {
	err error
	out AlignResult
}

/*
NewAlign creates an Align primitive.
*/
func NewAlign() core.Primitive {
	return &Align{}
}

/*
Next receives *AlignRequest payloads and yields a *AlignResult with the
aligned rows for each.
*/
func (op *Align) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			request := (*AlignRequest)(arriving)
			op.out = AlignResult{Rows: alignRows(request.Target, request.Series)}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Align) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
alignRows walks one target ring against lagged predictor series, reading
every series in place with per-series cursors.
*/
func alignRows(targets RingView, series []SeriesView) []AlignedRow {
	if targets.Len() == 0 || len(series) == 0 {
		return nil
	}

	cursors := make([]int, len(series))

	for index := range cursors {
		cursors[index] = -1
	}

	rows := make([]AlignedRow, 0, targets.Len())

	for targetIndex := 0; targetIndex < targets.Len(); targetIndex++ {
		target := targets.At(targetIndex)
		predictors := make([]Observation, len(series))
		complete := true

		for index, predictorSeries := range series {
			cutoff := target.At.Add(-predictorSeries.Lag)
			predictor, found := newestAtOrBefore(predictorSeries.History, &cursors[index], cutoff)

			if !found {
				complete = false
				break
			}

			predictors[index] = predictor
		}

		if !complete {
			continue
		}

		rows = append(rows, AlignedRow{
			Target:     target,
			Predictors: predictors,
		})
	}

	return rows
}

/*
newestAtOrBefore returns the newest observation in a resident ring view at or
before cutoff. The cursor remains positioned on the last matched observation
(a negative value means no match has ever been recorded): repeated calls with
non-decreasing cutoffs re-scan only entries after the previous match, and a
call whose cutoff reaches no newer entry returns the previously matched
observation. When no observation has ever matched, the result is not-found.
The precondition is that the ring is chronological and cutoffs are
non-decreasing across calls, which the alignment paths guarantee.
*/
func newestAtOrBefore(history RingView, cursor *int, cutoff time.Time) (Observation, bool) {
	best := -1
	start := 0

	if cursor != nil && *cursor >= 0 {
		start = *cursor
	}

	for index := start; index < history.Len() && !history.TimeAt(index).After(cutoff); index++ {
		best = index
	}

	if best >= 0 {
		if cursor != nil {
			*cursor = best
		}

		return history.At(best), true
	}

	if cursor == nil || *cursor < 0 {
		return Observation{}, false
	}

	return history.At(*cursor), true
}
