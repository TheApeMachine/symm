package fluid

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
Fluid models order-book liquidity as a compressible field with source-sink continuity.
*/
type Fluid struct {
	market        engine.MarketReader
	watch         *engine.SymbolWatch
	pairs         map[string]asset.Pair
	symbols       []string
	track         *TrackStore
	ui            *qpool.BroadcastGroup
	displayParams *DisplayParams
	gridBuilder   *GridBuilder
	pool          *qpool.Q
}

var _ engine.Signal = (*Fluid)(nil)

var _ engine.LiveScoreReader = (*Fluid)(nil)

var _ engine.MeanConfidenceReader = (*Fluid)(nil)

var _ engine.RiskExporter = (*Fluid)(nil)

/*
NewFluid wires the shared market broadcast relay into the engine signal.
*/
func NewFluid(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
	calibrationParams engine.CalibrationParams,
) (*Fluid, error) {
	fluid := &Fluid{
		market:        marketRelay,
		watch:         watch,
		pairs:         pairs,
		symbols:       append([]string(nil), symbols...),
		track:         NewTrackStore(calibrationParams),
		displayParams: NewDisplayParams(),
		pool:          pool,
	}

	fluid.gridBuilder = NewGridBuilder(fluid.displayParams)

	return fluid, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  fluid.track,
	})
}

func (fluid *Fluid) Source() string {
	return "fluid"
}

func (fluid *Fluid) Symbols() []string {
	return append([]string(nil), fluid.symbols...)
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
BindUI wires incremental field telemetry to the shared dashboard group.
*/
func (fluid *Fluid) BindUI(uiGroup *qpool.BroadcastGroup) {
	fluid.ui = uiGroup
}

func (fluid *Fluid) publishSymbolField(symbol string) {
	if fluid.ui == nil || symbol == "" {
		return
	}

	row := fluid.track.SymbolRow(symbol, fluid.market)
	publishEvent(fluid.ui, "field_row", map[string]any{
		"symbol": row.Symbol,
		"row":    WireRow(row),
	})

	rows := fluid.track.SnapshotRows(fluid.symbols, fluid.market)
	aggregate, sampledCount := aggregateFieldRows(rows)
	publishEvent(fluid.ui, "field_aggregate", map[string]any{
		"symbol_count": sampledCount,
		"field":        WireAggregate(aggregate),
	})
}

func (fluid *Fluid) publishFieldGrid() {
	if fluid.ui == nil {
		return
	}

	rows := fluid.track.SnapshotRows(fluid.symbols, fluid.market)
	grid := fluid.gridBuilder.Build(rows, fluid.displayParams.activeGridSize())
	publishEvent(fluid.ui, "field_grid", map[string]any{
		"grid": WireGrid(grid),
	})
}

func publishEvent(ui *qpool.BroadcastGroup, event string, payload map[string]any) {
	if ui == nil || payload == nil {
		return
	}

	payload["event"] = event
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	ui.Send(&qpool.QValue[any]{
		Value: payload,
	})
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
MeanConfidence returns the mean normalized confidence across the latest scan set.
*/
func (fluid *Fluid) MeanConfidence() float64 {
	return fluid.track.MeanGaugeConfidence()
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
				Market:  fluid.market,
				Watch:   fluid.watch,
				Pairs:   fluid.pairs,
				Symbols: fluid.symbols,
				Pool:    fluid.pool,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				confidence, reason := fluid.sampleField(symbol, fluid.market.Read(symbol))
				fluid.publishSymbolField(symbol)

				fluid.track.ObserveGaugeScore(confidence)

				if confidence <= 0 {
					return engine.Measurement{}, false, nil
				}

				if !fieldSnapshotReady(snapshot) || fieldSampleTime(snapshot).IsZero() {
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

		fluid.publishFieldGrid()
	}
}

func (fluid *Fluid) sampleField(symbol string, snapshot engine.Snapshot) (float64, string) {
	if snapshot.LastOK && snapshot.VolumeOK && snapshot.Last > 0 {
		fluid.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
	}

	if !fieldSnapshotReady(snapshot) {
		return 0, ""
	}

	sampleAt := fieldSampleTime(snapshot)

	if sampleAt.IsZero() {
		return 0, ""
	}

	return fluid.evaluate(symbol, snapshot, sampleAt)
}

func fieldSnapshotReady(snapshot engine.Snapshot) bool {
	if !snapshot.LastOK || !snapshot.VolumeOK || !snapshot.DensityOK || !snapshot.SpreadOK ||
		!snapshot.BatchOK || !snapshot.PressureOK {
		return false
	}

	return snapshot.Last > 0 && snapshot.BatchVolume > 0 &&
		snapshot.Density > 0 && snapshot.SpreadBPS > 0
}

func fieldSampleTime(snapshot engine.Snapshot) time.Time {
	sampleAt := snapshot.LastAt

	if snapshot.TradesAt.After(sampleAt) {
		sampleAt = snapshot.TradesAt
	}

	if snapshot.BookAt.After(sampleAt) {
		sampleAt = snapshot.BookAt
	}

	return sampleAt
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
