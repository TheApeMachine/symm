package pumpdump

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/logic"
)

const (
	obsPeakSpike = iota
	obsBookSide
	obsBuySide
	obsSpreadBPS
	obsLastPrice
	obsAnchor
)

type PumpSymbol struct {
	pair           asset.Pair
	volumeWindow   *adaptive.Window
	volumeBaseline *adaptive.EMA
	volumeSpike    *adaptive.Ratio
	score          *numeric.Scored
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
		score: newMeasureScore(
			moveClassifier,
		),
	}
}

func newMeasureScore(classifier *adaptive.Classifier) *numeric.Scored {
	return numeric.NewScored(
		classifier,
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
	)
}

func (sym *PumpSymbol) observations(peakSpike float64) []float64 {
	return []float64{
		peakSpike,
		math.Min(sym.imbalance, 1),
		(sym.buyPressure + 1) / 2,
		sym.spreadBPS,
		sym.lastPrice,
		sym.volumeWindow.Anchor(),
	}
}

func (sym *PumpSymbol) Measure(peakSpike float64) (engine.Measurement, bool) {
	confidence := errnie.Does(func() (float64, error) {
		return sym.score.Push(sym.observations(peakSpike)...)
	}).Or(func(err error) {
		errnie.Error(err)
	})

	if confidence.Value() <= 0 {
		return engine.Measurement{}, false
	}

	classCode := sym.score.ClassCode()

	return engine.Measurement{
		Type: logic.Or(
			engine.Pump,
			engine.Dump,
			classCode == 0,
		),
		Source:     "pumpdump",
		Regime:     "microstructure",
		Reason:     moveClassifier.Label(classCode),
		Pairs:      []asset.Pair{sym.pair},
		Confidence: confidence.Value(),
	}, true
}

func (sym *PumpSymbol) FeedTick(update engine.TickUpdate) {
	sym.lastPrice = update.Last
	sym.dailyQuoteVol = update.VolumeBase * update.Last
}

func (sym *PumpSymbol) FeedTrade(update engine.TradeUpdate) {
	closed := errnie.Does(func() (float64, error) {
		return sym.volumeWindow.Next(
			0,
			float64(update.UpdatedAt.UnixNano()),
			update.BatchVolume,
			sym.lastPrice,
		)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	if closed != sym.volumeWindow.Sum() {
		errnie.Does(func() (float64, error) {
			return sym.volumeBaseline.Next(0, closed)
		}).Or(func(err error) {
			errnie.Error(err)
		})
	}

	if update.BuyPressure > 0 {
		sym.buyPressure = update.BuyPressure
	}
}

func (sym *PumpSymbol) FeedBook(update engine.BookUpdate) {
	sym.spreadBPS = update.SpreadBPS
	sym.imbalance = update.Imbalance
}
