package strategy

import (
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Planner currently selects entry intents by comparing buy utility with doing
nothing. Keeping this scope explicit prevents the entry path from masquerading
as the later position-aware hold, exit, rotation, and reversal strategy.
*/
type Planner struct {
	status types.Status
	gate   stageGate
}

/*
stageGate gives Planner the one boot-stage fact it needs without making
strategy responsible for constructing or running the system publisher.
*/
type stageGate interface {
	Ready(system.StageType) bool
}

/*
rankedIntent retains the numerical utility used by strategy until ordering is
complete. Intent exposes Decimal for execution, but decimal comparisons would
repeatedly allocate arbitrary-precision values during a cross-section sort.
*/
type rankedIntent struct {
	intent  *Intent
	utility float64
}

/*
NewPlanner creates a Planner that is ready once its dependencies are assigned.
Planning has no deferred initialization or warmup of its own.
*/
func NewPlanner(gate stageGate) *Planner {
	return &Planner{
		status: types.READY,
		gate:   gate,
	}
}

/*
Status reports whether the Planner itself is ready to evaluate evidence.
Boot-stage admission remains a separate concern enforced by Update.
*/
func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
Update evaluates the thesis for all symbols and returns intended actions.
*/
func (planner *Planner) Update(thesis *Thesis) ([]*Intent, error) {
	if !planner.gate.Ready(system.StageReady) {
		return nil, nil
	}

	candidates := make([]*rankedIntent, 0)

	for _, symbol := range thesis.Symbols() {
		candidate, err := planner.evaluate(symbol, thesis)

		if err != nil {
			return nil, err
		}

		if candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	sort.Slice(candidates, func(left int, right int) bool {
		if candidates[left].utility == candidates[right].utility {
			return candidates[left].intent.Symbol < candidates[right].intent.Symbol
		}

		return candidates[left].utility > candidates[right].utility
	})

	intents := make([]*Intent, len(candidates))

	for index, candidate := range candidates {
		intents[index] = candidate.intent
	}

	return intents, nil
}

func (planner *Planner) evaluate(
	symbol string,
	thesis *Thesis,
) (*rankedIntent, error) {
	snapshot, ok := thesis.Evidence(symbol, "manifold_forecasts")
	if !ok {
		return nil, nil
	}

	forecasts, ok := snapshot.(types.Forecasts)

	if !ok || !forecasts.Eligible() || forecasts.Symbol != symbol {
		return nil, nil
	}

	decision, err := newDecision(forecasts)

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"strategy planner: decision evaluation failed",
			err,
		)
	}

	if thesis.Evaluated(symbol, decision.Forecast) {
		return nil, nil
	}

	recorded, err := thesis.RecordDecision(decision)

	if err != nil {
		return nil, err
	}

	if !recorded {
		return nil, nil
	}

	if decision.Action == ActionHold {
		return nil, nil
	}

	return &rankedIntent{
		utility: decision.Utility,
		intent: &Intent{
			Symbol:     symbol,
			Action:     decision.Action,
			Edge:       *decimal.NewFromFloat64(decision.Utility),
			Confidence: forecasts.Confidence,
			Thesis:     thesis,
		},
	}, nil
}
