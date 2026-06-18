package resonance

type marketFacts struct {
	lastPrice       float64
	volume          float64
	spreadBps       float64
	elapsed         float64
	changeAbs       float64
	changePct       float64
	buyPressure     float64
	tradeRate       float64
	tradeNotional   float64
	touchImbalance  float64
	depthImbalance  float64
	spreadWideRatio float64
	tickCadence     float64
	midDriftBps     float64
}

func buildSensoryVector(
	symbol string,
	registry *senseRegistry,
) ([]float64, marketFacts, bool) {
	_ = symbol
	_ = registry

	return nil, marketFacts{}, false
}
