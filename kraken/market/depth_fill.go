package market

/*
DepthQuoteFill walks ask/bid levels until quoteNotional is spent or depth runs out.
*/
func DepthQuoteFill(levels []BookLevel, quoteNotional float64) DepthFillResult {
	costSum, volumeSum, remaining := walkQuoteNotional(levels, quoteNotional)

	if volumeSum <= 0 || costSum <= 0 {
		return DepthFillResult{}
	}

	return DepthFillResult{
		VWAP:          costSum / volumeSum,
		QuoteProceeds: costSum,
		BaseQty:       volumeSum,
		Complete:      remaining <= 0,
	}
}

/*
DepthBaseFill walks bid/ask levels until baseQty is filled or depth runs out.
*/
func DepthBaseFill(levels []BookLevel, baseQty float64) DepthFillResult {
	costSum, volumeSum, remaining := walkBaseQuantity(levels, baseQty)

	if volumeSum <= 0 || costSum <= 0 {
		return DepthFillResult{}
	}

	return DepthFillResult{
		VWAP:          costSum / volumeSum,
		QuoteProceeds: costSum,
		BaseQty:       volumeSum,
		Complete:      remaining <= 0,
	}
}
