package market

/*
DepthFillResult is one depth-walk fill outcome.
*/
type DepthFillResult struct {
	VWAP          float64
	QuoteProceeds float64
	BaseQty       float64
	Complete      bool
}

/*
DepthQuoteFill walks ask/bid levels until quoteNotional is spent or depth runs out.
*/
func DepthQuoteFill(levels []BookLevel, quoteNotional float64) DepthFillResult {
	if quoteNotional <= 0 || len(levels) == 0 {
		return DepthFillResult{}
	}

	remaining := quoteNotional
	costSum := 0.0
	volumeSum := 0.0

	for _, level := range levels {
		if level.Price <= 0 || level.Volume <= 0 {
			continue
		}

		levelNotional := level.Price * level.Volume

		if levelNotional >= remaining {
			takeVolume := remaining / level.Price
			costSum += remaining
			volumeSum += takeVolume
			remaining = 0

			break
		}

		costSum += levelNotional
		volumeSum += level.Volume
		remaining -= levelNotional
	}

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
	if baseQty <= 0 || len(levels) == 0 {
		return DepthFillResult{}
	}

	remaining := baseQty
	costSum := 0.0
	volumeSum := 0.0

	for _, level := range levels {
		if level.Price <= 0 || level.Volume <= 0 {
			continue
		}

		if level.Volume >= remaining {
			costSum += remaining * level.Price
			volumeSum += remaining
			remaining = 0

			break
		}

		costSum += level.Volume * level.Price
		volumeSum += level.Volume
		remaining -= level.Volume
	}

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
