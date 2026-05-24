package causal

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
Causal applies Pearl's ladder: association, backdoor intervention, and counterfactual uplift.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as backdoor control.
*/
type Causal struct {
	*engine.SignalBase
	track *TrackStore
}

var _ engine.Signal = (*Causal)(nil)

var _ engine.FeedbackReceiver = (*Causal)(nil)

var _ engine.LiveScoreReader = (*Causal)(nil)

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
NewCausal wires live Kraken websocket observers into the engine signal.
*/
func NewCausal(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Causal, error) {
	base, err := engine.NewSignalBase(
		ctx,
		"causal",
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

	causal := &Causal{
		SignalBase: base,
		track:      NewTrackStore(),
	}

	return causal, errnie.Require(map[string]any{
		"base":  base,
		"track": causal.track,
	})
}

/*
ApplyFeedback nudges intervention calibration from settled prediction error.
*/
func (causal *Causal) ApplyFeedback(feedback engine.PredictionFeedback) {
	if feedback.Source != causal.Source() {
		return
	}

	causal.track.ApplyPredictionFeedback(feedback)
}

/*
Scan advances the causal model for the current scheduler tick.
*/
func (causal *Causal) Scan(now time.Time) error {
	causal.track.BeginScan()

	macro := causal.track.MacroMomentum(causal.Symbols(), func(symbol string) (float64, bool) {
		snapshot := causal.Ingest().Read(symbol)

		return snapshot.ChangePct, snapshot.ChangeOK
	})

	return causal.ScanSymbols(now, func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
		confidence, expectedReturn, runway, reason := causal.evaluate(symbol, snapshot, macro, now)

		if confidence <= 0 || expectedReturn <= 0 || runway <= 0 {
			return engine.Measurement{}, false, nil
		}

		return engine.Measurement{
			Type:           engine.Causal,
			Regime:         "causal",
			Reason:         reason,
			Confidence:     confidence,
			ExpectedReturn: expectedReturn,
			Runway:         runway,
		}, true, nil
	})
}

func (causal *Causal) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	macroMomentum float64,
	now time.Time,
) (float64, float64, time.Duration, string) {
	if !causal.track.PassesLiquidity(symbol) {
		return 0, 0, 0, ""
	}

	if !snapshot.LastOK || !snapshot.VolumeOK || !snapshot.BatchOK ||
		!snapshot.PressureOK || !snapshot.SpreadOK || !snapshot.ImbalanceOK {
		return 0, 0, 0, ""
	}

	if snapshot.Last <= 0 || snapshot.BatchVolume <= 0 || snapshot.SpreadBPS <= 0 ||
		snapshot.Imbalance <= 0 || snapshot.BuyPressure <= 0 {
		return 0, 0, 0, ""
	}

	causal.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)

	localFlow := snapshot.BatchVolume * (snapshot.BuyPressure + 1) / 2
	liquidity := bookLiquidity(snapshot.SpreadBPS, snapshot.BatchVolume)

	sample, ready := causal.track.BuildSample(
		symbol, macroMomentum, liquidity, localFlow, snapshot.Last, now,
	)

	if !ready {
		return 0, 0, 0, ""
	}

	confidence, expectedReturn, runway, reason := causal.track.Evaluate(symbol, sample)
	causal.track.CommitSample(symbol, sample, snapshot.Last, now)

	return confidence, expectedReturn, runway, reason
}

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
