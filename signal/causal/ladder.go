package causal

import (
	"math"

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
		return 0, 0, false
	}

	flow := flowHistory[len(flowHistory)-depth:]
	ret := velocityHistory[len(velocityHistory)-depth:]

	if !hasFiniteVariation(flow) || !hasFiniteVariation(ret) {
		return 0, 0, false
	}

	rows := make([][]float64, 0, depth)
	intervention := flow[0]

	for index := range depth {
		rows = append(rows, []float64{flow[index], ret[index]})

		// do(flow): peak observed aggression — the strongest flow this window
		// actually produced, derived from data rather than chosen.
		if flow[index] > intervention {
			intervention = flow[index]
		}
	}

	table, err := ncausal.NewNodeTableWrapper(rows, nodeReturn, minHistory)

	if err != nil {
		return 0, 0, false
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
		return 0, 0, false
	}

	return uplift, noise, true
}

func hasFiniteVariation(values []float64) bool {
	if len(values) < 2 {
		return false
	}

	var (
		minValue float64
		maxValue float64
		seeded   bool
	)

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
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

	return seeded && minValue != maxValue
}
