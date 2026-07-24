package logic

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
observeStates walks manifold state into resonance, causal, and forecast outputs.
A symbol epoch is observed once; later Analyzer cuts that still see that GasReady
view treat it as replay so idle Hawkes republishes cannot wipe calibrated
evidence by re-running Update at the same At.
*/
func (analyzer *Analyzer) observeStates(thesis *types.Thesis) []manifold.State {
	states := make([]manifold.State, 0)
	observeStarted := time.Now()

	if analyzer.observed == nil {
		analyzer.observed = make(map[string]uint64)
	}

	thesis.Manifold.Range(func(_, value any) bool {
		state, ok := value.(manifold.State)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received invalid manifold state",
				nil,
			))

			return true
		}

		if !state.Replay && analyzer.observed[state.Symbol] >= state.Epoch {
			state.Replay = true
			thesis.Manifold.Store(state.Symbol, state)
		}

		states = append(states, state)

		if state.Replay {
			return true
		}

		if analyzer.observe(thesis, state) {
			analyzer.observed[state.Symbol] = state.Epoch
		}

		return true
	})

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "observe", map[string]any{
		"states": len(states),
		"ns":     time.Since(observeStarted).Nanoseconds(),
	}))

	return states
}

/*
observe connects one manifold state to the existing resonance, causal, and
forecast outputs through the Thesis without making those components depend on
the manifold solver. It reports whether any symbol evidence was committed.
*/
func (analyzer *Analyzer) observe(
	thesis *types.Thesis,
	state manifold.State,
) bool {
	if analyzer.resonance == nil {
		analyzer.resonance = make(map[string]*Resonance)
	}

	if analyzer.causal == nil {
		analyzer.causal = make(map[string]*Causal)
	}

	resonance := analyzer.resonance[state.Symbol]

	if resonance == nil {
		resonance = NewResonance(state.Symbol, manifold.DefaultBaselineHalflife())

		if resonance == nil {
			return false
		}

		analyzer.resonance[state.Symbol] = resonance
	}

	measurements, resonanceOutcome := resonance.Update(state)

	causal := analyzer.causal[state.Symbol]

	if causal == nil {
		causal = NewCausal(state.Symbol)
		analyzer.causal[state.Symbol] = causal
	}

	hypothesis, causalOutcome, err := causal.Update(state)

	if err != nil {
		errnie.Error(err)
		causalOutcome = nil
	}

	if resonanceOutcome == nil && causalOutcome == nil && len(measurements) == 0 {
		return false
	}

	analyzer.dropSymbolEvidence(thesis, state.Symbol)

	if len(measurements) > 0 {
		// Upsert — never AppendMeasurements. Appending every Hawkes epoch grew the
		// durable thesis without bound; NewImmutableCut then cloned gigabytes.
		thesis.Publish(types.SourceResonance, measurements)
	}

	if resonanceOutcome != nil {
		thesis.Resonance = append(thesis.Resonance, resonanceOutcome)
	}

	if causalOutcome != nil {
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		thesis.Causal = append(thesis.Causal, causalOutcome)
	}

	analyzer.forecast(thesis, state, resonanceOutcome, causalOutcome)

	// Lock the epoch only after resonance publishes — earlier retries keep
	// feeding the same physical view until time-elastic baselines are ready.
	return resonanceOutcome != nil
}

/*
dropSymbolEvidence removes one symbol's prior observe outputs so a fresh epoch
replaces them instead of appending duplicates.
*/
func (analyzer *Analyzer) dropSymbolEvidence(thesis *types.Thesis, symbol string) {
	if thesis == nil || symbol == "" {
		return
	}

	priorForecasts := append([]types.Forecasts(nil), thesis.Forecasts...)
	forecasts := thesis.Forecasts[:0]

	for _, row := range priorForecasts {
		if row.Symbol == symbol {
			continue
		}

		forecasts = append(forecasts, row)
	}

	thesis.Forecasts = forecasts
	priorHypotheses := append([]types.Hypothesis(nil), thesis.Hypotheses...)
	hypotheses := thesis.Hypotheses[:0]

	for _, row := range priorHypotheses {
		if row.Symbol == symbol {
			continue
		}

		hypotheses = append(hypotheses, row)
	}

	thesis.Hypotheses = hypotheses
	thesis.Resonance = dropSymbolAny(thesis.Resonance, symbol)
	thesis.Causal = dropSymbolAny(thesis.Causal, symbol)
}

func dropSymbolAny(rows []any, symbol string) []any {
	prior := append([]any(nil), rows...)
	out := rows[:0]

	for _, row := range prior {
		switch value := row.(type) {
		case *ResonanceOutcome:
			if value != nil && value.Symbol == symbol {
				continue
			}
		case ResonanceOutcome:
			if value.Symbol == symbol {
				continue
			}
		case *CausalOutcome:
			if value != nil && value.Symbol == symbol {
				continue
			}
		case CausalOutcome:
			if value.Symbol == symbol {
				continue
			}
		}

		out = append(out, row)
	}

	return out
}

/*
forecast appends a joint resonance+causal forecast when both heads are ready.
*/
func (analyzer *Analyzer) forecast(
	thesis *types.Thesis,
	state manifold.State,
	resonanceOutcome *ResonanceOutcome,
	causalOutcome *CausalOutcome,
) {
	if resonanceOutcome == nil || causalOutcome == nil ||
		!resonanceOutcome.ReturnReady ||
		resonanceOutcome.CalibrationSamples == 0 ||
		!causalOutcome.Ready {
		return
	}

	// Candidate notional is capped at the best ask, so the forecast does not
	// claim depth-crossing impact beyond the directly observed touch spread.
	// ExpectedImpact is stamped by strategy friction, where deployable cash
	// determines the fraction of the visible touch an entry actually consumes.
	forecast := types.Forecasts{
		Source:                     "resonance+causal",
		Symbol:                     state.Symbol,
		At:                         state.At,
		ObservedInterval:           state.Duration,
		SourceEpoch:                state.Epoch,
		HorizonEvents:              resonanceOutcome.HorizonEvents,
		ExpiresEpoch:               state.Epoch + resonanceOutcome.HorizonEvents,
		Target:                     resonanceOutcome.Target,
		ModelVersion:               "resonance_return_head_v2_rls",
		Ready:                      true,
		Calibrated:                 true,
		CalibrationSamples:         resonanceOutcome.CalibrationSamples,
		IncrementalMSE:             resonanceOutcome.IncrementalMSE,
		IncrementalSkillLowerBound: resonanceOutcome.IncrementalSkillLowerBound,
		ExpectedReturn:             resonanceOutcome.ExpectedReturn,
		ReferencePrice:             state.ReferencePrice,
		BuyCapacity:                state.BuyCapacity,
		SellCapacity:               state.SellCapacity,
		ExpectedSpread:             state.Spread,
		ExpectedAdverseSelection:   analyzer.forecastAdverse(state, causalOutcome),
		Uncertainty:                resonanceOutcome.Uncertainty,
		Confidence: math.Min(
			causalOutcome.Reading.Confidence,
			math.Exp(-math.Abs(resonanceOutcome.Surprise)),
		),
	}

	thesis.Forecasts = append(thesis.Forecasts, forecast)
}

/*
forecastAdverse prices adverse selection as the causal head's informed-flow
probability times the observed touch cost, the Glosten-Milgrom decomposition:
a taker's expected loss to informed counterparties is the chance the present
flow is informed times the spread that compensates the quoting side for it.
Both factors are derived — no interim heuristic remains in the utility.
*/
func (analyzer *Analyzer) forecastAdverse(
	state manifold.State,
	causalOutcome *CausalOutcome,
) float64 {
	if state.Spread <= 0 || !finite(causalOutcome.InformedFlow) {
		return 0
	}

	return causalOutcome.InformedFlow * state.Spread
}
