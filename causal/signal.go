package causal

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
Causal applies Pearl's ladder: association, backdoor intervention, and counterfactual uplift.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as backdoor control.
*/
type Causal struct {
	ingest  *engine.Ingest
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
MeanConfidence returns the peak normalized confidence across the latest scan set.
*/
func (causal *Causal) MeanConfidence() float64 {
	return causal.track.PeakLiveConfidence()
}

/*
NewCausal wires live Kraken websocket observers into the engine signal.
*/
func NewCausal(
	_ context.Context,
	pool *qpool.Q,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Causal, error) {
	causal := &Causal{
		ingest:  engine.NewIngest(book, tradesObserver, tickerObserver),
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		track:   NewTrackStore(),
		pool:    pool,
	}

	return causal, errnie.Require(map[string]any{
		"ingest": causal.ingest,
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
	if feedback.Source != causal.Source() {
		return
	}

	causal.track.ApplyPredictionFeedback(feedback)
}

/*
Tick is a no-op until Causal subscribes to market broadcasts.
*/
func (causal *Causal) Tick() bool {
	return false
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
			snapshot := causal.ingest.Read(symbol)

			return snapshot.ChangePct, snapshot.ChangeOK
		})

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  causal.Source(),
				Ingest:  causal.ingest,
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
	if !causal.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	if !snapshot.LastOK || !snapshot.VolumeOK || !snapshot.BatchOK ||
		!snapshot.PressureOK || !snapshot.SpreadOK || !snapshot.ImbalanceOK {
		return 0, ""
	}

	if snapshot.Last <= 0 || snapshot.BatchVolume <= 0 || snapshot.SpreadBPS <= 0 ||
		snapshot.Imbalance <= 0 || snapshot.BuyPressure <= 0 {
		return 0, ""
	}

	causal.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)

	localFlow := snapshot.BatchVolume * (snapshot.BuyPressure + 1) / 2
	liquidity := bookLiquidity(snapshot.SpreadBPS, snapshot.BatchVolume)

	sample, ready := causal.track.BuildSample(
		symbol, macroMomentum, liquidity, localFlow, snapshot.Last, now,
	)

	if !ready {
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
