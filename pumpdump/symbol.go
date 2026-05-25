package pumpdump

import (
	"math"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
	"github.com/theapemachine/symm/numeric/logic"
)

const (
	obsPeakSpike = iota
	obsBookSide
	obsBuySide
	obsSpreadBPS
	obsLastPrice
	obsAnchor
	obsForecast
)

type PumpSymbol struct {
	pair           asset.Pair
	volumeWindow   *adaptive.Window
	volumeBaseline *adaptive.EMA
	volumeSpike    *adaptive.Ratio
	score          *numeric.Scored
	forecast       *learned.Forecast
	lastPrice      float64
	dailyQuoteVol  float64
	buyPressure    float64
	imbalance      float64
	spreadBPS      float64
}

func NewPumpSymbol(pair asset.Pair) *PumpSymbol {
	return &PumpSymbol{
		pair:           pair,
		volumeWindow:   adaptive.NewWindow(tradeWindow),
		volumeBaseline: adaptive.NewEMA(0),
		volumeSpike:    adaptive.NewRatio(0),
		forecast:       learned.NewForecast(0.35),
		score: numeric.NewScored(
			moveClassifier,
			numeric.NewAccumulate(
				numeric.NewDerived(numeric.WithDynamics(
					adaptive.NewProduct(),
					adaptive.NewEMA(0),
				)),
				func(values []float64) []float64 {
					return values[obsBookSide : obsBuySide+1]
				},
			),
			numeric.NewAccumulate(
				numeric.NewDerived(numeric.WithDynamics(
					adaptive.NewEMA(0),
					adaptive.NewCompression(0),
				)),
				func(values []float64) []float64 {
					return values[obsSpreadBPS : obsSpreadBPS+1]
				},
			),
			numeric.NewScaleIndex(obsPeakSpike),
			numeric.NewAccumulate(
				numeric.NewDerived(numeric.WithDynamics(adaptive.NewProduct())),
				func(values []float64) []float64 {
					return values[obsForecast : obsForecast+1]
				},
			),
		),
	}
}

func (symbolState *PumpSymbol) observations(peakSpike float64) []float64 {
	return []float64{
		peakSpike,
		math.Min(symbolState.imbalance, 1),
		(symbolState.buyPressure + 1) / 2,
		symbolState.spreadBPS,
		symbolState.lastPrice,
		symbolState.volumeWindow.Anchor(),
		symbolState.forecast.Scale(),
	}
}

func (symbolState *PumpSymbol) Measure(peakSpike float64) (engine.Measurement, bool) {
	confidence, err := symbolState.score.Push(symbolState.observations(peakSpike)...)

	if err != nil || confidence <= 0 {
		return engine.Measurement{}, false
	}

	classCode := symbolState.score.ClassCode()

	return engine.Measurement{
		Type: logic.Or(
			engine.Pump,
			engine.Dump,
			classCode == 0,
		),
		Source:     pumpdumpSource,
		Regime:     "microstructure",
		Reason:     moveClassifier.Label(classCode),
		Pairs:      []asset.Pair{symbolState.pair},
		Confidence: confidence,
	}, true
}

func (symbolState *PumpSymbol) spike() (float64, bool) {
	spike, err := symbolState.volumeSpike.Next(
		0,
		symbolState.volumeWindow.Sum(),
		symbolState.volumeBaseline.Value(),
	)

	if err != nil || spike <= 1 {
		return 0, false
	}

	return spike, true
}

func (symbolState *PumpSymbol) passesLiquidity(
	positiveQuotes map[string]float64,
	symbol string,
) bool {
	if symbolState.dailyQuoteVol <= 0 {
		return true
	}

	if len(positiveQuotes) < 1 {
		return false
	}

	liquid, err := liquidityGate.Next(
		symbolState.dailyQuoteVol,
		adaptive.PeerValues(positiveQuotes, symbol)...,
	)

	if err != nil || liquid <= 0 {
		return false
	}

	return true
}

/*
ApplyFeedback updates the per-symbol forecast learner from one settled prediction.
*/
func (symbolState *PumpSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	if feedback.PredictedReturn <= 0 {
		return
	}

	_, _ = symbolState.forecast.Next(0, feedback.PredictedReturn, feedback.ActualReturn)
}

func (symbolState *PumpSymbol) FeedTick(update TickUpdate) {
	symbolState.lastPrice = update.Last
	symbolState.dailyQuoteVol = update.VolumeBase * update.Last
}

func (symbolState *PumpSymbol) FeedTrade(update TradeUpdate) {
	closed, err := symbolState.volumeWindow.Next(
		0,
		float64(update.UpdatedAt.UnixNano()),
		update.BatchVolume,
		symbolState.lastPrice,
	)

	if err != nil {
		return
	}

	if closed != symbolState.volumeWindow.Sum() {
		_, _ = symbolState.volumeBaseline.Next(0, closed)
	}

	if update.BuyPressure > 0 {
		symbolState.buyPressure = update.BuyPressure
	}
}

func (symbolState *PumpSymbol) FeedBook(update BookUpdate) {
	symbolState.spreadBPS = update.SpreadBPS
	symbolState.imbalance = update.Imbalance
}
