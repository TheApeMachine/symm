package causal

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
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

	rows := make([]float64, 0, depth*nodeCount)
	intervention := flow[0]

	for index := range depth {
		rows = append(rows, flow[index], ret[index])

		// do(flow): peak observed aggression — the strongest flow this window
		// actually produced, derived from data rather than chosen.
		if flow[index] > intervention {
			intervention = flow[index]
		}
	}

	// Config carries the structural-model knobs; the table carries the rows. The
	// FlipFlop writes the table through the abduction stage and lands the
	// counterfactual outputs back on the table artifact (nomagique causal API).
	config := datura.Acquire("causal-counterfactual", datura.APPJSON)
	defer config.Release()

	config.Poke(float64(nodeReturn), "target").
		Poke(float64(nodeFlow), "treatment").
		Poke(intervention, "intervention").
		Poke(float64(minHistory), "minHistory").
		Poke([]float64{float64(nodeFlow)}, "features").
		Poke(float64(1), "linear")

	table := datura.Acquire("causal-counterfactual-table", datura.APPJSON)
	defer table.Release()

	table.Poke(float64(depth), "table", "rowCount").
		Poke(float64(nodeCount), "table", "nodeCount").
		Poke(rows, "table", "rows")

	if err := transport.NewFlipFlop(table, ncausal.NewAbduction(config)); err != nil {
		return 0, 0, false
	}

	return datura.Peek[float64](table, "output", "uplift"),
		datura.Peek[float64](table, "output", "noise"),
		true
}
