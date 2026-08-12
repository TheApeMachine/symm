package regulator

import (
	"math"
	"slices"
	"strconv"
)

/*
SubsystemStatus reports one optimizer model or live control without claiming
influence over components the regulator does not actuate.
*/
type SubsystemStatus struct {
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Health      string  `json:"health"`
	Direction   string  `json:"direction"`
	ValueText   string  `json:"valueText"`
	Explanation string  `json:"explanation"`
	Value       float64 `json:"value"`
}

/*
RegulatorPayload is the wire state published for the global regulator surface.
*/
type RegulatorPayload struct {
	Status          string            `json:"status"`
	Surprise        float64           `json:"surprise"`
	Energy          float64           `json:"energy"`
	PnL             float64           `json:"pnl"`
	PredictedReturn float64           `json:"predictedReturn"`
	PredictionScale float64           `json:"predictionScale"`
	Samples         int               `json:"samples"`
	Summary         string            `json:"summary"`
	Subsystems      []SubsystemStatus `json:"subsystems"`
	Sparkline       []float64         `json:"sparkline"`
}

func (solver *Solver) buildPayload(
	periodReturn float64,
	result optimizationResult,
) RegulatorPayload {
	status := "healthy"
	summary := "The predictive account model selected the bounded control vector with the strongest posterior wallet-return prospect."

	if !result.skillReady || !result.forecastReady {
		status = "observing"
		summary = "Collecting temporally resolved control and equity outcomes before trusting counterfactual parameter forecasts."
	}

	if result.exploring {
		status = "adapting"
		summary = "Applying a shrinking bounded intervention to identify how one live control affects the next account outcome."
	}

	if result.forecastReady && result.forecast.Value < 0 {
		status = "strained"
		summary = "Every evaluated control neighborhood has adverse expected wallet return; the optimizer selected the least adverse bounded candidate."
	}

	space := solver.optimizer.space
	controls := result.controls
	baseline := solver.optimizer.baseline
	modelValue := 0.0
	modelText := "learning"
	modelDirection := "resolving"

	if result.skillReady {
		modelValue = result.skill
		modelText = formatFloat(result.skill, 3) + "x skill"
		modelDirection = "validated"
	}

	subsystems := []SubsystemStatus{{
		Name:        "model",
		Label:       "Predictive Account Model",
		Health:      status,
		Direction:   modelDirection,
		ValueText:   modelText,
		Explanation: "Prequential skill compares prior next-equity forecasts with a zero-return baseline.",
		Value:       modelValue,
	}}
	subsystems = append(subsystems,
		controlStatus(
			"allocation", "Capital Allocation Ceiling", status,
			space.value(controlAllocation, controls),
			space.value(controlAllocation, baseline),
			"% max", 100, "reduced", "configured",
			"Maximum quote capital one admitted entry may allocate.",
		),
		controlStatus(
			"confidence", "Forecast Confidence Gate", status,
			space.value(controlConfidence, controls),
			space.value(controlConfidence, baseline),
			"% min", 100, "tightened", "configured",
			"Minimum posterior direction probability required before graph search may enter.",
		),
		controlStatus(
			"causal", "Causal Search Bias", status,
			space.value(controlCausalAlpha, controls),
			space.value(controlCausalAlpha, baseline),
			"x bias", 1, "reduced", "configured",
			"Weight of interventional evidence in MCTS trajectory selection.",
		),
		integerControlStatus(
			"iterations", "MCTS Search Budget", status,
			int(space.value(controlIterations, controls)),
			int(space.value(controlIterations, baseline)),
			"iterations", "reduced", "configured",
			"Number of causal trajectory simulations evaluated for each decision.",
		),
		controlStatus(
			"exploration", "MCTS Exploration", status,
			space.value(controlExploration, controls),
			space.value(controlExploration, baseline),
			" UCT", 1, "reduced", "configured",
			"Exploration weight used when balancing visited and uncertain graph branches.",
		),
		integerControlStatus(
			"manifold", "Manifold Relaxation", status,
			int(space.value(controlRelaxation, controls)),
			int(space.value(controlRelaxation, baseline)),
			"steps", "changed", "configured",
			"Physics relaxation work completed before downstream evidence is compiled.",
		),
	)

	predictedReturn := 0.0
	predictionScale := 0.0

	if result.forecastReady {
		predictedReturn = math.Expm1(result.forecast.Value)
		predictionScale = result.forecast.Scale
	}

	return RegulatorPayload{
		Status:          status,
		Surprise:        result.surprise,
		Energy:          result.energy,
		PnL:             math.Expm1(periodReturn) * 100,
		PredictedReturn: predictedReturn,
		PredictionScale: predictionScale,
		Samples:         solver.optimizer.resolved,
		Summary:         summary,
		Subsystems:      subsystems,
		Sparkline:       slices.Clone(solver.history),
	}
}

func controlStatus(
	name string,
	label string,
	health string,
	value float64,
	baseline float64,
	suffix string,
	displayScale float64,
	changedDirection string,
	baselineDirection string,
	explanation string,
) SubsystemStatus {
	direction := baselineDirection

	if value != baseline {
		direction = changedDirection
	}

	return SubsystemStatus{
		Name:        name,
		Label:       label,
		Health:      health,
		Direction:   direction,
		ValueText:   formatFloat(value*displayScale, 2) + suffix,
		Explanation: explanation,
		Value:       value,
	}
}

func integerControlStatus(
	name string,
	label string,
	health string,
	value int,
	baseline int,
	suffix string,
	changedDirection string,
	baselineDirection string,
	explanation string,
) SubsystemStatus {
	direction := baselineDirection

	if value != baseline {
		direction = changedDirection
	}

	return SubsystemStatus{
		Name:        name,
		Label:       label,
		Health:      health,
		Direction:   direction,
		ValueText:   strconv.Itoa(value) + " " + suffix,
		Explanation: explanation,
		Value:       float64(value),
	}
}

func formatFloat(value float64, decimals int) string {
	return strconv.FormatFloat(value, 'f', decimals, 64)
}
