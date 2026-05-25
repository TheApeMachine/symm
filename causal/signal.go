package causal

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
Causal scores flow/liquidity intervention heuristics with bounded effect composition.
DAG sketch: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as control.
*/
type Causal struct {
	engine.Passive
	market  *engine.MarketRelay
	watch   *engine.SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
	track   *TrackStore
	pool    *qpool.Q
}

var _ engine.Signal = (*Causal)(nil)

var _ engine.LiveScoreReader = (*Causal)(nil)

var _ engine.MeanConfidenceReader = (*Causal)(nil)

/*
LiveScore returns the current causal gauge reading from track state.
*/
func (causal *Causal) LiveScore() float64 {
	return causal.track.PeakLiveConfidence()
}

func (causal *Causal) PeakReading() engine.LiveReading {
	symbol, score := causal.track.PeakSymbolScore()

	return engine.LiveReading{
		Symbol: symbol,
		Score:  score,
	}
}

/*
MeanConfidence returns the mean normalized confidence across the latest scan set.
*/
func (causal *Causal) MeanConfidence() float64 {
	return causal.track.MeanGaugeConfidence()
}

/*
NewCausal wires the shared market broadcast relay into the engine signal.
*/
func NewCausal(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
	calibrationParams engine.CalibrationParams,
) (*Causal, error) {
	causal := &Causal{
		market:  marketRelay,
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		track:   NewTrackStore(calibrationParams),
		pool:    pool,
	}

	return causal, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  causal.track,
	})
}

func (causal *Causal) Source() string {
	return "causal"
}

/*
Feedback nudges intervention calibration from settled prediction error.
*/
func (causal *Causal) Feedback(feedback engine.PredictionFeedback) {
	engine.ForwardSourceFeedback(causal.Source(), feedback, causal.track.ApplyPredictionFeedback)
}

/*
Measure advances the causal model and yields non-zero uplift readings.
*/
func (causal *Causal) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		causal.track.BeginScan()
		engine.DrainTicks(ctx)

		macro := causal.track.MacroMomentum(causal.symbols, func(symbol string) (float64, bool) {
			snapshot := causal.market.Read(symbol)

			return snapshot.ChangePct, snapshot.ChangeOK
		})

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  causal.Source(),
				Market:  causal.market,
				Watch:   causal.watch,
				Pairs:   causal.pairs,
				Symbols: causal.symbols,
				Pool:    causal.pool,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				confidence, reason := causal.evaluate(
					symbol, snapshot, macro, now,
				)

				causal.track.ObserveGaugeScore(confidence)

				if confidence <= 0 {
					return engine.Measurement{}, false, nil
				}

				return engine.Measurement{
					Type:       engine.Causal,
					Regime:     "causal",
					Reason:     reason,
					Confidence: confidence,
				}, true, nil
			},
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (causal *Causal) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	macroMomentum float64,
	now time.Time,
) (float64, string) {
	if !snapshot.LastOK || !snapshot.VolumeOK || !snapshot.BatchOK ||
		!snapshot.PressureOK || !snapshot.SpreadOK || !snapshot.ImbalanceOK {
		return 0, ""
	}

	if snapshot.Last <= 0 || snapshot.BatchVolume <= 0 || snapshot.SpreadBPS <= 0 ||
		snapshot.Imbalance <= 0 || snapshot.BuyPressure <= 0 {
		return 0, ""
	}

	causal.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)

	if !causal.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	localFlow := snapshot.BatchVolume * (snapshot.BuyPressure + 1) / 2
	liquidity := bookLiquidity(snapshot.SpreadBPS, snapshot.BatchVolume)

	sample, ready := causal.track.BuildSample(
		symbol, macroMomentum, liquidity, localFlow, snapshot.Last, now,
	)

	if !ready {
		causal.track.CommitSample(symbol, sample, snapshot.Last, now)

		return 0, ""
	}

	confidence, reason := causal.track.Evaluate(symbol, sample)
	causal.track.CommitSample(symbol, sample, snapshot.Last, now)

	return confidence, reason
}

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
