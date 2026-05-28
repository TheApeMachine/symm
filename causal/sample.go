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

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
