package market

/*
DepthVisibleNotionalFraction is the share of quoteNotional filled from visible
book depth before adverse-impact pricing applies.
*/
func DepthVisibleNotionalFraction(levels []BookLevel, quoteNotional float64) float64 {
	if quoteNotional <= 0 {
		return 0
	}

	_, _, remaining := walkQuoteNotional(levels, quoteNotional)

	visible := quoteNotional - remaining

	if visible <= 0 {
		return 0
	}

	return visible / quoteNotional
}
