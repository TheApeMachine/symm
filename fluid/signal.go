package fluid

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	ingest        *engine.Ingest
	watch         *engine.SymbolWatch
	pairs         map[string]asset.Pair
	symbols       []string
	track         *TrackStore
	fieldSink     FieldSink
	displayParams *DisplayParams
	gridBuilder   *GridBuilder
}

var _ engine.Signal = (*Fluid)(nil)

var _ engine.LiveScoreReader = (*Fluid)(nil)

var _ engine.MeanConfidenceReader = (*Fluid)(nil)

var _ engine.RiskExporter = (*Fluid)(nil)

/*
NewFluid wires live Kraken websocket observers into the engine signal.
*/
func NewFluid(
	_ context.Context,
	_ *qpool.Q,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Fluid, error) {
	fluid := &Fluid{
		ingest:        engine.NewIngest(book, tradesObserver, tickerObserver),
		watch:         watch,
		pairs:         pairs,
		symbols:       append([]string(nil), symbols...),
		track:         NewTrackStore(),
		displayParams: NewDisplayParams(),
	}

	fluid.gridBuilder = NewGridBuilder(fluid.displayParams)

	return fluid, errnie.Require(map[string]any{
		"ingest": fluid.ingest,
		"track":  fluid.track,
	})
}

func (fluid *Fluid) Source() string {
	return "fluid"
}

func (fluid *Fluid) Symbols() []string {
	return append([]string(nil), fluid.symbols...)
}

func (fluid *Fluid) Ingest() *engine.Ingest {
	return fluid.ingest
}

/*
ApplyDisplayPatch updates server-side fluid terrain presentation parameters.
*/
func (fluid *Fluid) ApplyDisplayPatch(patch DisplayPatch) (DisplayParamsSnapshot, error) {
	gridSizeChanged, err := fluid.displayParams.Apply(patch)

	if err != nil {
		return DisplayParamsSnapshot{}, err
	}

	if patch.ResetSmoothing != nil && *patch.ResetSmoothing {
		fluid.gridBuilder.ResetSmoothing()
	}

	if gridSizeChanged {
		fluid.gridBuilder.ResetSmoothing()
	}

	return fluid.displayParams.Snapshot(), nil
}

/*
DisplayParams returns the active fluid terrain presentation snapshot.
*/
func (fluid *Fluid) DisplayParams() DisplayParamsSnapshot {
	return fluid.displayParams.Snapshot()
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
LiveScore returns the current fluid gauge reading from track state.
*/
func (fluid *Fluid) LiveScore() float64 {
	return fluid.track.PeakLiveConfidence()
}

func (fluid *Fluid) PeakReading() engine.LiveReading {
	symbol, score := fluid.track.PeakSymbolScore()

	return engine.LiveReading{
		Symbol: symbol,
		Score:  score,
	}
}

/*
MeanConfidence returns the peak normalized confidence across the latest scan set.
*/
func (fluid *Fluid) MeanConfidence() float64 {
	return fluid.track.PeakLiveConfidence()
}

/*
SymbolRisk exposes fluid turbulence metrics for dynamic execution.
*/
func (fluid *Fluid) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	return fluid.track.SymbolRisk(symbol)
}

/*
Feedback nudges fluid source/shock calibration from settled prediction error.
*/
func (fluid *Fluid) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != fluid.Source() {
		return
	}

	fluid.track.ApplyPredictionFeedback(feedback)
}

/*
Tick is a no-op until Fluid subscribes to market broadcasts.
*/
func (fluid *Fluid) Tick() bool {
	return false
}

/*
Measure advances the fluid field and yields non-zero flow readings.
*/
func (fluid *Fluid) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		fluid.track.BeginScan()
		engine.DrainTicks(ctx)

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  fluid.Source(),
				Ingest:  fluid.ingest,
				Watch:   fluid.watch,
				Pairs:   fluid.pairs,
				Symbols: fluid.symbols,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				if snapshot.LastOK && snapshot.VolumeOK && snapshot.Last > 0 {
					fluid.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
				}

				confidence, reason := fluid.evaluate(symbol, snapshot, now)

				fluid.track.ObserveGaugeScore(confidence)

				if confidence <= 0 {
					return engine.Measurement{}, false, nil
				}

				return engine.Measurement{
					Type:       engine.Flow,
					Regime:     "flow",
					Reason:     reason,
					Confidence: confidence,
				}, true, nil
			},
		) {
			if !yield(measurement) {
				return
			}
		}

		if fluid.fieldSink != nil {
			fluid.fieldSink(fluid.FieldSnapshot())
		}
	}
}

func (fluid *Fluid) evaluate(
	symbol string, snapshot engine.Snapshot, now time.Time,
) (float64, string) {
	if !fluid.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	if !snapshot.DensityOK || !snapshot.SpreadOK || !snapshot.LastOK ||
		!snapshot.BatchOK || !snapshot.PressureOK {
		return 0, ""
	}

	if snapshot.Density <= 0 || snapshot.SpreadBPS <= 0 || snapshot.Last <= 0 || snapshot.BatchVolume <= 0 {
		return 0, ""
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
