package engine

/*
String returns the feedback bucket name for a market regime.
*/
func (marketRegime MarketRegime) String() string {
	switch marketRegime {
	case RegimeDead:
		return "dead"
	case RegimeChoppy:
		return "choppy"
	case RegimeTrending:
		return "trending"
	case RegimeBullish:
		return "bullish"
	case RegimeBearish:
		return "bearish"
	default:
		return ""
	}
}

/*
FeedbackRegime returns the market regime when a perspective has one, otherwise
it preserves the measurement's source-local regime label.
*/
func FeedbackRegime(perspective Perspective, measurement Measurement) string {
	if perspective.Regime != RegimeUnknown {
		return perspective.Regime.String()
	}

	return measurement.Regime
}
