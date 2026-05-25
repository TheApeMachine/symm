package causal

const (
	causalHistoryCap = 64
	minCausalHistory = 12
)

/*
causalSample is one DAG observation:
MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as a backdoor node.
*/
type causalSample struct {
	macroMomentum float64
	liquidity     float64
	localFlow     float64
	priceVelocity float64
}

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
