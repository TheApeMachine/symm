package strategy

import (
	"math"

	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Reading is the decomposed admit state for one forecast. Margin is economic
edge after uncertainty; Lead is how far cognition has committed ahead of the
manifold basin on a shared [0,1) scale. Reserved overflow is for anticipatory
edge only — high SNR, cognition ahead of the basin by more than noise, and a
non-ambiguous short-horizon reading.
*/
type Reading struct {
	Margin      float64
	Lead        float64
	Cognitive   float64
	Basin       float64
	BasinReady  bool
	Ambiguous   bool
	Contrast    float64
	Uncertainty float64
	Horizon     uint64
	Noise       float64
}

/*
Reserved reports whether this reading may consume overflow slots. Normal-lane
enter only needs positive executable utility; reserved further demands that
margin exceed residual uncertainty (SNR > 2), cognitive lead clear the same
noise share CognitiveClears uses, the basin be ready, the horizon be the next
event, and cognition be non-ambiguous with positive winner contrast.
*/
func (reading Reading) Reserved() bool {
	if reading.Ambiguous || !reading.BasinReady {
		return false
	}

	if reading.Horizon != 1 {
		return false
	}

	if reading.Contrast <= 0 {
		return false
	}

	if reading.Margin <= reading.Uncertainty {
		return false
	}

	return reading.Lead > reading.Noise
}

/*
CognitiveClears reports whether cognition clears the forecast noise share.
Economic enter is decided by executable utility (return − uncertainty −
friction); this gate only blocks Winner=buy at ε confidence.
*/
func (reading Reading) CognitiveClears(forecast types.Forecasts) bool {
	return reading.Cognitive > noiseShare(forecast)
}

/*
measureOpportunity builds OpportunityMargin and CognitiveLead from independent
estimators. Margin is the SNR term that also enters utility; Lead ranks the
reserved lane once utility has already cleared.
*/
func measureOpportunity(
	forecast types.Forecasts,
	cognition types.Cognition,
	thesis *types.Thesis,
) Reading {
	reading := Reading{
		Margin:      forecast.ExpectedReturn - forecast.Uncertainty,
		Cognitive:   cognition.Confidence,
		Ambiguous:   cognition.Ambiguous,
		Contrast:    cognition.Contrast,
		Uncertainty: forecast.Uncertainty,
		Horizon:     forecast.HorizonEvents,
		Noise:       noiseShare(forecast),
	}

	basin, ready := basinConfidence(thesis, forecast.Symbol)
	reading.Basin = basin
	reading.BasinReady = ready

	if ready {
		reading.Lead = cognition.Confidence - basin
	}

	return reading
}

/*
noiseShare is the fraction of forecast magnitude explained by uncertainty.
Cognition must exceed this share before a buy winner is treated as actionable.
*/
func noiseShare(forecast types.Forecasts) float64 {
	if forecast.Uncertainty <= 0 {
		return 0
	}

	magnitude := math.Abs(forecast.ExpectedReturn) + forecast.Uncertainty

	return forecast.Uncertainty / magnitude
}

/*
basinConfidence maps manifold coherence onto [0,1) so Lead compares like
quantities. Missing or invalid manifold leaves BasinReady false so Lead stays
neutral rather than inventing dynamics.
*/
func basinConfidence(thesis *types.Thesis, symbol string) (float64, bool) {
	if thesis == nil || thesis.Manifold == nil {
		return 0, false
	}

	value, found := thesis.Manifold.Load(symbol)

	if !found {
		return 0, false
	}

	state, ok := value.(manifold.State)

	if !ok || !state.GasReady() {
		return 0, false
	}

	coherence := state.Reading.CoherenceMag2

	if math.IsNaN(coherence) || math.IsInf(coherence, 0) || coherence < 0 {
		return 0, false
	}

	return coherence / (1 + coherence), true
}
