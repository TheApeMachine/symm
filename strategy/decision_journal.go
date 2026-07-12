package strategy

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
forecastIdentity is the stable identity of one symbol's model epoch.
Observation time remains part of the retained provenance, while evaluation
identity follows the producer's explicit epoch contract.
*/
type forecastIdentity struct {
	symbol       string
	source       string
	sourceEpoch  uint64
	target       string
	modelVersion string
}

/*
DecisionJournal owns the append-only history of strategy evaluations.
Planner is its sole writer and calls it synchronously, so a plain slice and
lookup index preserve chronological order without adding coordination machinery.
*/
type DecisionJournal struct {
	entries   map[string][]Decision
	evaluated map[forecastIdentity]struct{}
	last      map[string]time.Time
}

/*
NewDecisionJournal creates an empty strategy evaluation history.
*/
func NewDecisionJournal() *DecisionJournal {
	return &DecisionJournal{
		entries:   map[string][]Decision{},
		evaluated: map[forecastIdentity]struct{}{},
		last:      map[string]time.Time{},
	}
}

/*
Record validates and appends a decision unless its forecast epoch was already
evaluated. A duplicate returns false without error; malformed or regressing
evaluations return an explicit error. Slice-backed data is copied on ingress.
*/
func (journal *DecisionJournal) Record(decision Decision) (bool, error) {
	if err := decision.Validate(); err != nil {
		return false, err
	}

	identity := decision.identity()

	if _, ok := journal.evaluated[identity]; ok {
		return false, nil
	}

	if last := journal.last[decision.Symbol]; decision.At.Before(last) {
		return false, errnie.Err(
			errnie.Validation,
			"strategy decision journal: evaluation time moved backward",
			nil,
		)
	}

	journal.entries[decision.Symbol] = append(
		journal.entries[decision.Symbol],
		decision.clone(),
	)
	journal.evaluated[identity] = struct{}{}
	journal.last[decision.Symbol] = decision.At

	return true, nil
}

/*
Evaluated reports whether strategy already considered one forecast epoch.
The constant-time index keeps repeated live-loop observations independent of
the length of the retained decision history.
*/
func (journal *DecisionJournal) Evaluated(
	symbol string,
	forecast types.Forecasts,
) bool {
	_, ok := journal.evaluated[newForecastIdentity(symbol, forecast)]

	return ok
}

/*
Decisions returns one symbol's decisions in their original evaluation order.
Copies protect the append-only record from mutations by postmortem readers.
*/
func (journal *DecisionJournal) Decisions(symbol string) []Decision {
	entries := journal.entries[symbol]
	decisions := make([]Decision, len(entries))

	for index, decision := range entries {
		decisions[index] = decision.clone()
	}

	return decisions
}

func (decision Decision) identity() forecastIdentity {
	return newForecastIdentity(decision.Symbol, decision.Forecast)
}

func newForecastIdentity(
	symbol string,
	forecast types.Forecasts,
) forecastIdentity {
	return forecastIdentity{
		symbol:       symbol,
		source:       forecast.Source,
		sourceEpoch:  forecast.SourceEpoch,
		target:       forecast.Target,
		modelVersion: forecast.ModelVersion,
	}
}
