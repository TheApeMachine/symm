package causal

import (
	"math"

	"github.com/theapemachine/errnie"
	ncausal "github.com/theapemachine/nomagique/causal"
)

/*
counterfactual runs Pearl's abductive counterfactual on the symbol's own trade
history: given the observed (flow → return) structure, it abducts the residual
noise that reproduces reality, intervenes with do(flow = peak aggression), and
reads the counterfactual return. The uplift is the causal market-impact answer
to "if aggression surged to its recent peak, how much more would price move?" —
the real Endogenous Alpha, not a polynomial of multipliers.

It returns the uplift, the abducted residual noise (how much of the observed
move is idiosyncratic, feeding Causal Noise), and ok=false when there is not yet
enough aligned history to fit the structural model.
*/
func (signal *Signal) counterfactual(
	flowHistory, velocityHistory []float64,
) (uplift, noise float64, ok bool) {
	uplift, noise, ok, _ = signal.counterfactualWithFault(
		flowHistory,
		velocityHistory,
	)

	return uplift, noise, ok
}

func (signal *Signal) counterfactualWithFault(
	flowHistory, velocityHistory []float64,
) (uplift, noise float64, ok bool, fault string) {
	// Columns must be aligned row-for-row; trim both to the shorter tail so each
	// row is a coherent (flow, return) observation from the same trade window.
	depth := min(len(velocityHistory), len(flowHistory))

	const (
		nodeFlow   = 0 // treatment: order-flow aggression magnitude
		nodeReturn = 1 // target: realized return magnitude
		nodeCount  = 2
	)

	// Degrees of freedom: a two-node structural fit plus a residual needs more
	// rows than nodes. This is the structural minimum, not a tuned warmup gate.
	minHistory := nodeCount + 1

	if depth < minHistory {
		return 0, 0, false, ""
	}

	flow := flowHistory[len(flowHistory)-depth:]
	ret := velocityHistory[len(velocityHistory)-depth:]

	flowFinite, flowVaries := finiteVariation(flow)
	returnFinite, returnVaries := finiteVariation(ret)
	if !flowFinite || !returnFinite {
		return 0, 0, false, "non_finite_history"
	}
	if !flowVaries || !returnVaries {
		return 0, 0, false, ""
	}

	standardFlow, flowOK := standardizeSeries(flow)
	standardReturn, returnOK := standardizeSeries(ret)

	if !flowOK || !returnOK {
		return 0, 0, false, "standardization_failed"
	}

	rows := make([][]float64, 0, depth)
	intervention := flow[0]

	for index := range depth {
		rows = append(rows, []float64{
			standardFlow.values[index],
			standardReturn.values[index],
		})

		// do(flow): peak observed aggression — the strongest flow this window
		// actually produced, derived from data rather than chosen.
		if flow[index] > intervention {
			intervention = flow[index]
		}
	}

	intervention = (intervention - standardFlow.mean) / standardFlow.scale
	table, err := ncausal.NewNodeTableWrapper(rows, nodeReturn, minHistory)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: standardized counterfactual table failed",
			err,
		))
		return 0, 0, false, "counterfactual_table_failed"
	}

	uplift, _, noise, err = table.AbductiveCounterfactual(
		[]int{nodeFlow},
		true,
		rows[len(rows)-1],
		nodeReturn,
		nodeFlow,
		intervention,
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: standardized counterfactual fit failed",
			err,
		))
		return 0, 0, false, "counterfactual_fit_failed"
	}

	uplift *= standardReturn.scale
	noise *= standardReturn.scale

	if !isFinite(uplift) || !isFinite(noise) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"causal: standardized counterfactual emitted non-finite output",
			nil,
		))
		return 0, 0, false, "counterfactual_non_finite"
	}

	return uplift, noise, true, ""
}

type standardizedSeries struct {
	values []float64
	mean   float64
	scale  float64
}

func standardizeSeries(values []float64) (standardizedSeries, bool) {
	if len(values) < 2 {
		return standardizedSeries{}, false
	}

	mean := 0.0

	for _, value := range values {
		if !isFinite(value) {
			return standardizedSeries{}, false
		}

		mean += value
	}

	mean /= float64(len(values))

	variance := 0.0

	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}

	scale := math.Sqrt(variance / float64(len(values)))

	if scale <= 0 || !isFinite(scale) {
		return standardizedSeries{}, false
	}

	standardized := make([]float64, len(values))

	for index, value := range values {
		standardized[index] = (value - mean) / scale
	}

	return standardizedSeries{
		values: standardized,
		mean:   mean,
		scale:  scale,
	}, true
}

func hasFiniteVariation(values []float64) bool {
	finite, varies := finiteVariation(values)

	return finite && varies
}

func finiteVariation(values []float64) (finite bool, varies bool) {
	if len(values) < 2 {
		return true, false
	}

	var (
		minValue float64
		maxValue float64
		seeded   bool
	)

	for _, value := range values {
		if !isFinite(value) {
			return false, false
		}

		if !seeded {
			minValue = value
			maxValue = value
			seeded = true
			continue
		}

		if value < minValue {
			minValue = value
		}

		if value > maxValue {
			maxValue = value
		}
	}

	return true, seeded && minValue != maxValue
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
