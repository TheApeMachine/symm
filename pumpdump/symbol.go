package pumpdump

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
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
	pair                 asset.Pair
	fastVolumeWindow     *adaptive.Window
	mediumVolumeWindow   *adaptive.Window
	hourlyVolumeWindow   *adaptive.Window
	fastVolumeBaseline   *adaptive.EMA
	mediumVolumeBaseline *adaptive.EMA
	fastVolumeSpike      *adaptive.Ratio
	mediumVolumeSpike    *adaptive.Ratio
	medianHourlyVolume   float64
	hourlyBaselineReady  atomic.Bool
	score                *numeric.Scored
	forecast             *learned.Forecast
	spreadCompression    *adaptive.Compression
	bookGate             *adaptive.Product
	precursorMove        *adaptive.PositiveMove
	lastPrice            float64
	bid                  float64
	ask                  float64
	dailyQuoteVol        float64
	buyPressure          float64
	imbalance            float64
	spreadBPS            float64
}

func NewPumpSymbol(pair asset.Pair) *PumpSymbol {
	return &PumpSymbol{
		pair:                 pair,
		fastVolumeWindow:     adaptive.NewWindow(config.System.FastPumpWindow),
		mediumVolumeWindow:   adaptive.NewWindow(config.System.MediumPumpWindow),
		hourlyVolumeWindow:   adaptive.NewWindow(time.Hour),
		fastVolumeBaseline:   adaptive.NewEMA(0),
		mediumVolumeBaseline: adaptive.NewEMA(0),
		fastVolumeSpike:      adaptive.NewRatio(0),
		mediumVolumeSpike:    adaptive.NewRatio(0),
		forecast:             learned.NewForecast(0.35),
		spreadCompression:    adaptive.NewCompression(0),
		bookGate:             adaptive.NewProduct(),
		precursorMove:        adaptive.NewPositiveMove(0.001),
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

func (state *PumpSymbol) Measure(peakSpike float64, regime string) (engine.Measurement, bool) {
	raw, err := state.score.Push(
		peakSpike,
		math.Min(state.imbalance, 1),
		(state.buyPressure+1)/2,
		state.spreadBPS,
		state.lastPrice,
		state.mediumVolumeWindow.Anchor(),
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
	reason := moveClassifier.Label(classCode)

	if regime != "" {
		reason = regime
	}

	return engine.Measurement{
		Type: logic.Or(
			engine.Pump,
			engine.Dump,
			classCode == 0,
		),
		Source:     pumpdumpSource,
		Regime:     "microstructure",
		Reason:     reason,
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
	return state.precursorMove.Next(0, state.lastPrice, state.mediumVolumeWindow.Anchor())
}

func (state *PumpSymbol) FeedTradeVolume(at time.Time, qty float64, anchorPrice float64) {
	nanos := float64(at.UnixNano())

	for _, pair := range []struct {
		window   *adaptive.Window
		baseline *adaptive.EMA
	}{
		{state.fastVolumeWindow, state.fastVolumeBaseline},
		{state.mediumVolumeWindow, state.mediumVolumeBaseline},
		{state.hourlyVolumeWindow, nil},
	} {
		closed, err := pair.window.Next(0, nanos, qty, anchorPrice)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if pair.baseline == nil {
			continue
		}

		// Window rotation: closed is the volume evicted on this tick; when it
		// differs from the current rolling sum the window has closed a slice,
		// so advance the baseline EMA by that closed count via Next(0, closed).
		if closed != pair.window.Sum() {
			if _, err := pair.baseline.Next(0, closed); err != nil {
				errnie.Error(err)
			}
		}
	}
}

func (state *PumpSymbol) SetMedianHourlyVolume(volume float64) {
	if volume > 0 {
		state.medianHourlyVolume = volume
		state.hourlyBaselineReady.Store(true)
	}
}

func (state *PumpSymbol) HourlyBaselineReady() bool {
	return state.hourlyBaselineReady.Load()
}

func (state *PumpSymbol) SlowRVOL() float64 {
	if !state.hourlyBaselineReady.Load() || state.medianHourlyVolume <= 0 {
		return 0
	}

	return state.hourlyVolumeWindow.Sum() / state.medianHourlyVolume
}

func (state *PumpSymbol) BestVolumeSpike() (spike float64, regime string, err error) {
	fastBaseline := state.fastVolumeBaseline.Value()
	mediumBaseline := state.mediumVolumeBaseline.Value()

	if fastBaseline <= 0 || mediumBaseline <= 0 {
		return 0, "", nil
	}

	fastSpike, err := state.fastVolumeSpike.Next(
		0,
		state.fastVolumeWindow.Sum(),
		fastBaseline,
	)

	if err != nil {
		return 0, "", err
	}

	if fastSpike >= config.System.FastPumpVolumeRatio {
		return fastSpike, "fast_pump", nil
	}

	mediumSpike, err := state.mediumVolumeSpike.Next(
		0,
		state.mediumVolumeWindow.Sum(),
		mediumBaseline,
	)

	if err != nil {
		return 0, "", err
	}

	if mediumSpike <= 1 {
		return mediumSpike, "", nil
	}

	return mediumSpike, "actual_pump", nil
}
