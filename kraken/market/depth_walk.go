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

func walkQuoteNotional(levels []BookLevel, quoteNotional float64) (costSum, volumeSum, remaining float64) {
	if quoteNotional <= 0 || len(levels) == 0 {
		return 0, 0, quoteNotional
	}

	remaining = quoteNotional

	for _, level := range levels {
		if level.Price <= 0 || level.Qty <= 0 {
			continue
		}

		levelNotional := level.Price * level.Qty

		if levelNotional >= remaining {
			takeVolume := remaining / level.Price
			costSum += remaining
			volumeSum += takeVolume
			remaining = 0

			break
		}

		costSum += levelNotional
		volumeSum += level.Qty
		remaining -= levelNotional
	}

	return costSum, volumeSum, remaining
}

func walkBaseQuantity(levels []BookLevel, baseQty float64) (costSum, volumeSum, remaining float64) {
	if baseQty <= 0 || len(levels) == 0 {
		return 0, 0, baseQty
	}

	remaining = baseQty

	for _, level := range levels {
		if level.Price <= 0 || level.Qty <= 0 {
			continue
		}

		if level.Qty >= remaining {
			costSum += remaining * level.Price
			volumeSum += remaining
			remaining = 0

			break
		}

		costSum += level.Qty * level.Price
		volumeSum += level.Qty
		remaining -= level.Qty
	}

	return costSum, volumeSum, remaining
}
