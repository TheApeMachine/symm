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
func (analyzer *Analyzer) observeStates(
	thesis *types.Thesis,
	cutID types.CutID,
	tick int64,
) []manifold.State {
	analyzer.stateRows = analyzer.stateRows[:0]
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
		if !stateReplay(state) && analyzer.observed[state.Symbol] >= state.Epoch {
			state = withStateReplay(state, true)
			thesis.Manifold.Store(state.Symbol, state)
		}

		analyzer.stateRows = append(analyzer.stateRows, state)

		if stateReplay(state) {
			return true
		}

		if analyzer.observe(thesis, state) {
			analyzer.observed[state.Symbol] = state.Epoch
		}

		return true
	})

	payload := map[string]any{
		"states": len(analyzer.stateRows),
		"ns":     time.Since(observeStarted).Nanoseconds(),
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "observe", payload))

	return analyzer.stateRows
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
		thesis.ReplaceMeasurements(state.Symbol, measurements)
	}
	if resonanceOutcome != nil {
		thesis.Resonance.Store(state.Symbol, resonanceOutcome)
	}
	if causalOutcome != nil {
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		thesis.Causal.Store(state.Symbol, causalOutcome)
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
	forecasts := thesis.Forecasts[:0]

	for _, row := range thesis.Forecasts {
		if row.Symbol == symbol {
			continue
		}

		forecasts = append(forecasts, row)
	}

	for index := len(forecasts); index < len(thesis.Forecasts); index++ {
		thesis.Forecasts[index] = types.Forecasts{}
	}

	thesis.Forecasts = forecasts
	hypotheses := thesis.Hypotheses[:0]

	for _, row := range thesis.Hypotheses {
		if row.Symbol == symbol {
			continue
		}

		hypotheses = append(hypotheses, row)
	}

	for index := len(hypotheses); index < len(thesis.Hypotheses); index++ {
		thesis.Hypotheses[index] = types.Hypothesis{}
	}

	thesis.Hypotheses = hypotheses
	thesis.Resonance.Delete(symbol)
	thesis.Causal.Delete(symbol)
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
		ExpectedAdverseSelection: analyzer.forecastAdverse(
			thesis, state, causalOutcome, resonanceOutcome.ExpectedReturn,
		),
		Uncertainty: resonanceOutcome.Uncertainty,
		Confidence: math.Min(
			causalOutcome.Reading.Confidence,
			math.Exp(-math.Abs(resonanceOutcome.Surprise)),
		),
	}
	thesis.Forecasts = append(thesis.Forecasts, forecast)
}

/*
forecastAdverse prices adverse selection as Glosten-Milgrom informed-flow times
touch spread, plus the trap-attributable share of the return claim derived from
live signal masses on Thesis. Trap tax uses trapShare × max(0, ExpectedReturn)
so a dominant trap cannot leave positive executable return.
*/
func (analyzer *Analyzer) forecastAdverse(
	thesis *types.Thesis,
	state manifold.State,
	causalOutcome *CausalOutcome,
	expectedReturn float64,
) float64 {
	adverse := 0.0
	if state.Spread > 0 && finite(causalOutcome.InformedFlow) {
		adverse = causalOutcome.InformedFlow * state.Spread
	}
	return adverse + TrapShare(thesis, state.Symbol).Tax(expectedReturn)
}
