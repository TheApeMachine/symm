package pumpdump

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const pumpdumpSource = "pumpdump"

/*
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	ctx    context.Context
	cancel context.CancelFunc
	market *engine.MarketRelay
	watch  *engine.SymbolWatch
	pairs  map[string]asset.Pair
	track  *TrackStore
	filter Filter
	pool   *qpool.Q
}

var _ engine.Signal = (*PumpDump)(nil)

var _ engine.Ticker = (*PumpDump)(nil)

var _ engine.MeanConfidenceReader = (*PumpDump)(nil)

/*
NewPumpDump wires market broadcast groups into the engine signal.
*/
func NewPumpDump(
	ctx context.Context,
	pool *qpool.Q,
	tick, trade, book *qpool.BroadcastGroup,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	watch *engine.SymbolWatch,
) (*PumpDump, error) {
	ctx, cancel := context.WithCancel(ctx)

	track, err := NewTrackStore(ctx, tick, trade, book)

	if err != nil {
		cancel()

		return nil, err
	}

	pumpdump := &PumpDump{
		ctx:    ctx,
		cancel: cancel,
		market: marketRelay,
		watch:  watch,
		pairs:  pairs,
		track:  track,
		filter: &PrecursorFilter{},
		pool:   pool,
	}

	return pumpdump, errnie.Require(map[string]any{
		"ctx":    ctx,
		"market": marketRelay,
		"track":  track,
	})
}

/*
Source identifies this signal in telemetry.
*/
func (pumpdump *PumpDump) Source() string {
	return pumpdumpSource
}

/*
Tick drains one market broadcast message into track state.
*/
func (pumpdump *PumpDump) Tick() bool {
	return pumpdump.track.Tick()
}

/*
MeanConfidence returns the peak normalized confidence across the latest scan set.
*/
func (pumpdump *PumpDump) MeanConfidence() float64 {
	return pumpdump.track.PeakLiveConfidence()
}

var _ engine.OHLCWarmer = (*PumpDump)(nil)

/*
WarmFromOHLC seeds precursor volume and price-move baselines from historical candles.
*/
func (pumpdump *PumpDump) WarmFromOHLC(candles map[string][]engine.OHLCCandle) {
	pumpdump.track.WarmFromOHLC(candles)
}

/*
Feedback nudges precursor calibration from settled prediction error.
*/
func (pumpdump *PumpDump) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != pumpdumpSource {
		return
	}

	pumpdump.track.ApplyPredictionFeedback(feedback)
}

/*
Measure samples microstructure and yields unit-scale measurements for this tick.
*/
func (pumpdump *PumpDump) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		pumpdump.track.ResetLiveScores()
		pumpdump.track.RollBuckets(now)

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source: pumpdumpSource,
				Market: pumpdump.market,
				Watch:  pumpdump.watch,
				Pairs:  pumpdump.pairs,
				Pool:   pumpdump.pool,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				rawConfidence, _ := pumpdump.filter.Score(
					symbol, pumpdump.track, snapshot, now,
				)
				pumpdump.track.ObserveGaugeScore(
					GaugeConfidence(pumpdump.track, symbol, rawConfidence),
				)

				return pumpdump.evaluate(symbol, snapshot, now)
			},
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (pumpdump *PumpDump) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	now time.Time,
) (engine.Measurement, bool, error) {
	rawConfidence, reason := pumpdump.filter.Score(symbol, pumpdump.track, snapshot, now)

	if rawConfidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	if reason == "" {
		reason = "precursor"
	}

	confidence, reason := FinalizeReading(
		pumpdump.track, symbol, rawConfidence, reason,
	)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	return engine.Measurement{
		Type:       engine.Pump,
		Regime:     "pump",
		Reason:     reason,
		Confidence: confidence,
	}, true, nil
}
