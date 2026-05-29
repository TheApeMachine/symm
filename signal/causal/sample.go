package causal

const (
	causalHistoryCap = 64
	minCausalHistory = 12
)

const (
	macroMomentumNode = iota
	liquidityNode
	localFlowNode
	priceVelocityNode
	causalNodeCount
)

/*
causalSample is one indexed DAG observation supplied by the financial adapter.
*/
type causalSample struct {
	nodes [causalNodeCount]float64
}

func newCausalSample(
	macroMomentum, liquidity, localFlow, priceVelocity float64,
) causalSample {
	return causalSample{
		nodes: [causalNodeCount]float64{
			macroMomentumNode: macroMomentum,
			liquidityNode:     liquidity,
			localFlowNode:     localFlow,
			priceVelocityNode: priceVelocity,
		},
	}
}

func (sample causalSample) value(node int) float64 {
	return sample.nodes[node]
}

// minSpreadBPSFloor caps the effective denominator in bookLiquidity. Without
// it, a 1e-4 bps spread on a tight pair generates a feature value four orders
// of magnitude above the typical sample and dominates the normal matrix's
// trace, which then drives ridgeLambda for the whole fit.
const minSpreadBPSFloor = 0.5

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	if spreadBPS < minSpreadBPSFloor {
		spreadBPS = minSpreadBPSFloor
	}

	return batchVolume / spreadBPS
}
