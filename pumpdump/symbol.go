package pumpdump

import (
	"math"

	"github.com/theapemachine/errnie"
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
	pair              asset.Pair
	volumeWindow      *adaptive.Window
	volumeBaseline    *adaptive.EMA
	volumeSpike       *adaptive.Ratio
	score             *numeric.Scored
	forecast          *learned.Forecast
	spreadCompression *adaptive.Compression
	bookGate          *adaptive.Product
	precursorMove     *adaptive.PositiveMove
	lastPrice         float64
	bid               float64
	ask               float64
	dailyQuoteVol     float64
	buyPressure       float64
	imbalance         float64
	spreadBPS         float64
}

func NewPumpSymbol(pair asset.Pair) *PumpSymbol {
	return &PumpSymbol{
		pair:              pair,
		volumeWindow:      adaptive.NewWindow(tradeWindow),
		volumeBaseline:    adaptive.NewEMA(0),
		volumeSpike:       adaptive.NewRatio(0),
		forecast:          learned.NewForecast(0.35),
		spreadCompression: adaptive.NewCompression(0),
		bookGate:          adaptive.NewProduct(),
		precursorMove:     adaptive.NewPositiveMove(0.001),
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

func (state *PumpSymbol) Measure(peakSpike float64) (engine.Measurement, bool) {
	raw, err := state.score.Push(
		peakSpike,
		math.Min(state.imbalance, 1),
		(state.buyPressure+1)/2,
		state.spreadBPS,
		state.lastPrice,
		state.volumeWindow.Anchor(),
		state.forecast.Scale(),
	)

	if err != nil {
		errnie.Error(err)
		return engine.Measurement{}, false
	}

	if raw <= 0 {
		return engine.Measurement{}, false
	}

	confidence, err := state.measureAlignment(peakSpike)

	if err != nil {
		errnie.Error(err)
		return engine.Measurement{}, false
	}

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	classCode := state.score.ClassCode()

	return engine.Measurement{
		Type: logic.Or(
			engine.Pump,
			engine.Dump,
			classCode == 0,
		),
		Source:     pumpdumpSource,
		Regime:     "microstructure",
		Reason:     moveClassifier.Label(classCode),
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.lastPrice,
		Bid:        state.bid,
		Ask:        state.ask,
	}, true
}

/*
measureAlignment scores how completely the current microstructure matches a pump setup.
Each factor is derived only from present observations, not symbol history.
*/
func (state *PumpSymbol) measureAlignment(peakSpike float64) (float64, error) {
	spreadScore, err := state.spreadCompression.Next(0, state.spreadBPS)

	if err != nil {
		return 0, err
	}

	bookSide, err := state.bookSideStrength()

	if err != nil {
		return 0, err
	}

	moveStrength, err := state.precursorMoveStrength()

	if err != nil {
		return 0, err
	}

	return engine.AlignConfidence(
		engine.ExcessRatio(peakSpike),
		bookSide,
		engine.ExcessRatio(spreadScore),
		moveStrength,
	), nil
}

func (state *PumpSymbol) bookSideStrength() (float64, error) {
	return state.bookGate.Next(
		0,
		math.Min(math.Abs(state.imbalance), 1),
		(state.buyPressure+1)/2,
	)
}

func (state *PumpSymbol) precursorMoveStrength() (float64, error) {
	return state.precursorMove.Next(0, state.lastPrice, state.volumeWindow.Anchor())
}
