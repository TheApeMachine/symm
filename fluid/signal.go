package fluid

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Fluid models order-book liquidity as a compressible field with source-sink continuity.
*/
type Fluid struct {
	*engine.SignalBase
	track       *TrackStore
	fieldSink   FieldSink
	gridBuilder *GridBuilder
}

var _ engine.Signal = (*Fluid)(nil)

var _ engine.FeedbackReceiver = (*Fluid)(nil)

var _ engine.LiveScoreReader = (*Fluid)(nil)

var _ engine.RiskExporter = (*Fluid)(nil)

/*
NewFluid wires live Kraken websocket observers into the engine signal.
*/
func NewFluid(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Fluid, error) {
	base, err := engine.NewSignalBase(
		ctx,
		"fluid",
		book,
		tradesObserver,
		tickerObserver,
		pairs,
		symbols,
		watch,
	)

	if err != nil {
		return nil, err
	}

	fluid := &Fluid{
		SignalBase:  base,
		track:       NewTrackStore(),
		gridBuilder: NewGridBuilder(),
	}

	return fluid, errnie.Require(map[string]any{
		"base":  base,
		"track": fluid.track,
	})
}

/*
SetFieldSink wires immediate field telemetry after every scan.
*/
func (fluid *Fluid) SetFieldSink(sink FieldSink) {
	fluid.fieldSink = sink
}

/*
SampledCount returns symbols with at least one fluid sample.
*/
func (fluid *Fluid) SampledCount() int {
	return fluid.track.SampledCount()
}

/*
WarmingCount returns symbols ingesting ticker volume but not yet sampled.
*/
func (fluid *Fluid) WarmingCount() int {
	return fluid.track.WarmingCount()
}

/*
LiveScore returns the current fluid gauge reading from track and field state.
*/
func (fluid *Fluid) LiveScore() float64 {
	peak := fluid.track.PeakLiveConfidence()

	if peak > 0 {
		return peak
	}

	return fieldGaugeScore(fluid.FieldSnapshot().Field)
}

func (fluid *Fluid) PeakReading() engine.LiveReading {
	symbol, score := fluid.track.PeakSymbolScore()

	if score <= 0 {
		return engine.LiveReading{Score: fluid.LiveScore()}
	}

	return engine.LiveReading{
		Symbol: symbol,
		Score:  score,
	}
}

/*
SymbolRisk exposes fluid turbulence metrics for dynamic execution.
*/
func (fluid *Fluid) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	return fluid.track.SymbolRisk(symbol)
}

/*
ApplyFeedback nudges fluid source/shock calibration from settled prediction error.
*/
func (fluid *Fluid) ApplyFeedback(feedback engine.PredictionFeedback) {
	if feedback.Source != fluid.Source() {
		return
	}

	fluid.track.ApplyPredictionFeedback(feedback)
}

/*
Scan advances the fluid field for the current scheduler tick.
*/
func (fluid *Fluid) Scan(now time.Time) error {
	fluid.track.BeginScan()

	err := fluid.ScanSymbols(now, func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
		if snapshot.LastOK && snapshot.VolumeOK && snapshot.Last > 0 {
			fluid.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
		}

		confidence, expectedReturn, runway, reason := fluid.evaluate(symbol, snapshot, now)

		if confidence <= 0 || expectedReturn <= 0 || runway <= 0 {
			return engine.Measurement{}, false, nil
		}

		return engine.Measurement{
			Type:           engine.Flow,
			Regime:         "flow",
			Reason:         reason,
			Confidence:     confidence,
			ExpectedReturn: expectedReturn,
			Runway:         runway,
		}, true, nil
	})

	if err != nil {
		return err
	}

	if fluid.fieldSink != nil {
		fluid.fieldSink(fluid.FieldSnapshot())
	}

	return nil
}

func (fluid *Fluid) evaluate(
	symbol string, snapshot engine.Snapshot, now time.Time,
) (float64, float64, time.Duration, string) {
	if !fluid.track.PassesLiquidity(symbol) {
		return 0, 0, 0, ""
	}

	if !snapshot.DensityOK || !snapshot.SpreadOK || !snapshot.LastOK ||
		!snapshot.BatchOK || !snapshot.PressureOK {
		return 0, 0, 0, ""
	}

	if snapshot.Density <= 0 || snapshot.SpreadBPS <= 0 || snapshot.Last <= 0 || snapshot.BatchVolume <= 0 {
		return 0, 0, 0, ""
	}

	flow := snapshot.BatchVolume

	if snapshot.BuyPressure > 0 {
		flow = snapshot.BatchVolume * (snapshot.BuyPressure + 1) / 2
	}

	depthSlope := 0.0

	if snapshot.DepthSlopeOK {
		depthSlope = snapshot.DepthSlope
	}

	return fluid.track.Sample(
		symbol,
		snapshot.Density,
		snapshot.Last,
		snapshot.SpreadBPS,
		depthSlope,
		flow,
		snapshot.BuyPressure,
		now,
	)
}
