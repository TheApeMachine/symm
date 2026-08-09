package causal

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
)

/*
causalState retains resolved rows and the one strictly prior forecast waiting
for a later market price. Keeping both together avoids a second per-symbol
cache and makes the timestamp boundary explicit.
*/
type causalState struct {
	rows             [][]float64
	pendingAt        time.Time
	pendingMidpoint  float64
	pendingCondition float64
	pendingContagion float64
	pendingTreatment float64
}

/*
observe resolves a pending forecast only against a strictly later market price,
then retains the exact row the Pearl model observes. The current forecast
becomes the next pending row; an unavailable forecast clears that pending slot
after resolving the prior one so a later target cannot silently span gaps.

The capacity is the number of first- and second-moment parameters implied by
the row width, matching the model's own data-backed window without introducing
an independent history limit.
*/
func (solver *Solver) observe(
	symbol string,
	features [3]float64,
	midpoint float64,
	at time.Time,
	forecastReady bool,
) ([]float64, [][]float64, bool, error) {
	if symbol == "" || at.IsZero() || midpoint <= 0 ||
		math.IsNaN(midpoint) || math.IsInf(midpoint, 0) {
		return nil, nil, false, errnie.Err(
			errnie.Validation,
			"causal: symbol, timestamp, and positive midpoint required",
			nil,
		)
	}

	if forecastReady {
		for _, feature := range features {
			if math.IsNaN(feature) || math.IsInf(feature, 0) {
				return nil, nil, false, errnie.Err(
					errnie.Validation,
					"causal: finite pending forecast features required",
					nil,
				)
			}
		}
	}

	solver.rowsMu.Lock()
	defer solver.rowsMu.Unlock()

	stored, found := solver.rows.Load(symbol)

	if !found {
		stored, _ = solver.rows.LoadOrStore(symbol, &causalState{})
	}

	state, ok := stored.(*causalState)

	if !ok || state == nil {
		return nil, nil, false, errnie.Err(
			errnie.Validation,
			"causal: invalid retained symbol state",
			nil,
		)
	}

	if !state.pendingAt.IsZero() && !at.After(state.pendingAt) {
		return nil, nil, false, nil
	}

	var row []float64

	if !state.pendingAt.IsZero() {
		realizedReturn := math.Log(midpoint / state.pendingMidpoint)

		if math.IsNaN(realizedReturn) || math.IsInf(realizedReturn, 0) {
			return nil, nil, false, errnie.Err(
				errnie.Validation,
				"causal: finite forward return required",
				nil,
			)
		}

		row = []float64{
			state.pendingCondition,
			state.pendingContagion,
			state.pendingTreatment,
			realizedReturn,
		}
		state.rows = append(state.rows, row)
		rowWidth := len(row)
		capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2

		if len(state.rows) > capacity {
			state.rows = state.rows[len(state.rows)-capacity:]
		}
	}

	state.pendingAt = time.Time{}
	state.pendingMidpoint = 0

	if forecastReady {
		state.pendingAt = at
		state.pendingMidpoint = midpoint
		state.pendingCondition = features[0]
		state.pendingContagion = features[1]
		state.pendingTreatment = features[2]
	}

	if row == nil {
		return nil, nil, false, nil
	}

	return row, append([][]float64(nil), state.rows...), true, nil
}
