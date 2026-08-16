package regulator

import (
	"math"
	"slices"
	"strconv"
	"time"
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
	Status           string            `json:"status"`
	Surprise         float64           `json:"surprise"`
	Energy           float64           `json:"energy"`
	PnL              float64           `json:"pnl"`
	PredictedReturn  float64           `json:"predictedReturn"`
	PredictionScale  float64           `json:"predictionScale"`
	PredictedActive  float64           `json:"predictedActive"`
	ActivityScale    float64           `json:"activityScale"`
	Samples          int               `json:"samples"`
	MarkSamples      uint64            `json:"markSamples"`
	IntervalMarks    int               `json:"intervalMarks"`
	LastMarkSymbol   string            `json:"lastMarkSymbol,omitempty"`
	LastMarkAt       string            `json:"lastMarkAt,omitempty"`
	LastMarkReturn   float64           `json:"lastMarkReturn"`
	LastMarkDrawdown float64           `json:"lastMarkDrawdown"`
	LastMarkFloor    float64           `json:"lastMarkFloorDistance"`
	LastMarkSurge    bool              `json:"lastMarkSurgeArmed"`
	Summary          string            `json:"summary"`
	Subsystems       []SubsystemStatus `json:"subsystems"`
	Sparkline        []float64         `json:"sparkline"`
}

/*
controlPresentation describes the truthful wire label and formatting for one
live control coordinate.
*/
type controlPresentation struct {
	index        int
	name         string
	label        string
	suffix       string
	displayScale float64
	changed      string
	explanation  string
	integer      bool
}

var controlPresentations = [...]controlPresentation{
	{
		index: controlAllocation, name: "allocation",
		label: "Capital Allocation Ceiling", suffix: "% max", displayScale: 100,
		changed:     "reduced",
		explanation: "Maximum quote capital one admitted entry may allocate.",
	},
	{
		index: controlConfidence, name: "confidence",
		label: "Forecast Support Confidence", suffix: "% min", displayScale: 100,
		changed:     "tightened",
		explanation: "Posterior direction probability required for each retained forecast horizon.",
	},
	{
		index: controlGraphThreshold, name: "graph",
		label: "Evidence Admission Boundary", suffix: " graph", displayScale: 1,
		changed:     "changed",
		explanation: "Minimum signed MCTS evidence reward admitted for executable evaluation.",
	},
	{
		index: controlCausalAlpha, name: "causal",
		label: "Causal Search Bias", suffix: "x bias", displayScale: 1,
		changed:     "reduced",
		explanation: "Weight of interventional evidence in MCTS trajectory selection.",
	},
	{
		index: controlIterations, name: "iterations",
		label: "MCTS Search Budget", suffix: "iterations", integer: true,
		changed:     "reduced",
		explanation: "Number of causal trajectory simulations evaluated for each decision.",
	},
	{
		index: controlExploration, name: "exploration",
		label: "MCTS Exploration", suffix: " UCT", displayScale: 1,
		changed:     "reduced",
		explanation: "Exploration weight used when balancing visited and uncertain graph branches.",
	},
	{
		index: controlRelaxation, name: "manifold",
		label: "Manifold Relaxation", suffix: "steps", integer: true,
		changed:     "changed",
		explanation: "Physics relaxation work completed before downstream evidence is compiled.",
	},
}

func (solver *Solver) buildPayload(
	periodReturn float64,
	result optimizationResult,
) RegulatorPayload {
	status, summary := result.presentation()
	subsystems := solver.subsystemStatuses(status, result)

	predictedReturn := 0.0
	predictionScale := 0.0
	predictedActive := 0.0
	activityScale := 0.0

	if result.forecastReady {
		predictedReturn = math.Expm1(result.forecast.Value)
		predictionScale = result.forecast.Scale
	}

	if result.activityReady {
		predictedActive = result.activity.Value
		activityScale = result.activity.Scale
	}

	lastMarkAt := ""

	if !solver.lastMarkAt.IsZero() {
		lastMarkAt = solver.lastMarkAt.Format(time.RFC3339Nano)
	}

	return RegulatorPayload{
		Status:           status,
		Surprise:         result.surprise,
		Energy:           result.energy,
		PnL:              math.Expm1(periodReturn) * 100,
		PredictedReturn:  predictedReturn,
		PredictionScale:  predictionScale,
		PredictedActive:  predictedActive,
		ActivityScale:    activityScale,
		Samples:          solver.optimizer.resolved,
		MarkSamples:      solver.markSamples,
		IntervalMarks:    solver.lastMarkContext.samples,
		LastMarkSymbol:   solver.lastMarkSymbol,
		LastMarkAt:       lastMarkAt,
		LastMarkReturn:   math.Expm1(solver.lastMarkReturn),
		LastMarkDrawdown: math.Expm1(solver.lastMarkDrawdown),
		LastMarkFloor:    solver.lastMarkFloor,
		LastMarkSurge:    solver.lastMarkSurge,
		Summary:          summary,
		Subsystems:       subsystems,
		Sparkline:        slices.Clone(solver.history),
	}
}

func (result optimizationResult) presentation() (string, string) {
	if result.forecastReady && result.forecast.Value < 0 {
		return "strained", "The selected local candidate still has adverse expected wallet return."
	}

	if result.exploring {
		return "adapting", "Applying a shrinking bounded intervention to identify one live control's response."
	}

	if !result.skillReady || !result.forecastReady {
		return "observing", "Collecting resolved control and equity outcomes before trusting parameter forecasts."
	}

	return "healthy", "Selected the bounded control vector with the strongest posterior wallet-return prospect."
}

func (solver *Solver) subsystemStatuses(
	health string,
	result optimizationResult,
) []SubsystemStatus {
	modelValue := 0.0
	modelText := "learning"
	modelDirection := "resolving"

	if result.skillReady {
		modelValue = result.skill
		modelText = formatFloat(result.skill, 3) + "x skill"
		modelDirection = "validated"
	}

	statuses := []SubsystemStatus{{
		Name: "model", Label: "Predictive Account Model", Health: health,
		Direction: modelDirection, ValueText: modelText, Value: modelValue,
		Explanation: "Prequential skill compares prior next-equity forecasts with a zero-return baseline.",
	}}

	for _, presentation := range controlPresentations {
		statuses = append(statuses, solver.controlStatus(
			health,
			presentation,
			result.controls,
		))
	}

	return statuses
}

func (solver *Solver) controlStatus(
	health string,
	presentation controlPresentation,
	controls controlVector,
) SubsystemStatus {
	space := solver.optimizer.space
	value := space.value(presentation.index, controls)
	baseline := space.value(presentation.index, solver.optimizer.baseline)
	direction := "configured"

	if value != baseline {
		direction = presentation.changed
	}

	valueText := formatFloat(value*presentation.displayScale, 2) + presentation.suffix

	if presentation.integer {
		valueText = strconv.Itoa(int(value)) + " " + presentation.suffix
	}

	return SubsystemStatus{
		Name:        presentation.name,
		Label:       presentation.label,
		Health:      health,
		Direction:   direction,
		ValueText:   valueText,
		Explanation: presentation.explanation,
		Value:       value,
	}
}

func formatFloat(value float64, decimals int) string {
	return strconv.FormatFloat(value, 'f', decimals, 64)
}
