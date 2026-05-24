package hawkes

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/market"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Hawkes detects buy-side trade clustering via a bivariate self-exciting Hawkes model.
*/
type Hawkes struct {
	*engine.SignalBase
	trades *trades.Trades
	track  *TrackStore
}

var _ engine.Signal = (*Hawkes)(nil)

var _ engine.FeedbackReceiver = (*Hawkes)(nil)

var _ engine.LiveScoreReader = (*Hawkes)(nil)

/*
NewHawkes wires live Kraken websocket observers into the engine signal.
*/
func NewHawkes(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Hawkes, error) {
	base, err := engine.NewSignalBase(
		ctx,
		"hawkes",
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

	hawkes := &Hawkes{
		SignalBase: base,
		trades:     tradesObserver,
		track:      NewTrackStore(),
	}

	return hawkes, errnie.Require(map[string]any{
		"base":   base,
		"trades": tradesObserver,
		"track":  hawkes.track,
	})
}

/*
Scan recalibrates Hawkes intensity for the current scheduler tick and enqueues
unit-scale measurements for Measure to drain.
*/
func (hawkes *Hawkes) Scan(now time.Time) error {
	hawkes.track.BeginScan()
	hawkes.refreshTracks()

	return hawkes.ScanSymbols(now, func(
		symbol string, snapshot engine.Snapshot,
	) (engine.Measurement, bool, error) {
		confidence, expectedReturn, runway := hawkes.evaluate(symbol, snapshot, now)

		if confidence <= 0 || expectedReturn <= 0 {
			return engine.Measurement{}, false, nil
		}

		return engine.Measurement{
			Type:           engine.Momentum,
			Regime:         "momentum",
			Reason:         "cluster_buy",
			Confidence:     confidence,
			ExpectedReturn: expectedReturn,
			Runway:         runway,
		}, true, nil
	})
}

/*
LiveScore returns the current Hawkes gauge reading from track state.
*/
func (hawkes *Hawkes) LiveScore() float64 {
	return hawkes.track.PeakLiveConfidence()
}

func (hawkes *Hawkes) PeakReading() engine.LiveReading {
	symbol, score := hawkes.track.PeakSymbolScore()

	return engine.LiveReading{
		Symbol: symbol,
		Score:  score,
	}
}

/*
ApplyFeedback nudges Hawkes excitation parameters from settled prediction error.
*/
func (hawkes *Hawkes) ApplyFeedback(feedback engine.PredictionFeedback) {
	if feedback.Source != hawkes.Source() {
		return
	}

	hawkes.track.ApplyPredictionFeedback(feedback)
}

func (hawkes *Hawkes) refreshTracks() {
	for _, symbol := range hawkes.Symbols() {
		snapshot := hawkes.Ingest().Read(symbol)

		if snapshot.LastOK && snapshot.VolumeOK {
			hawkes.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
		}
	}
}

func (hawkes *Hawkes) evaluate(
	symbol string, snapshot engine.Snapshot, now time.Time,
) (float64, float64, time.Duration) {
	if !hawkes.track.PassesLiquidity(symbol) {
		return 0, 0, 0
	}

	allTicks, ok := hawkes.trades.RecentTicks(symbol, time.Time{})

	if !ok || len(allTicks) == 0 {
		return 0, 0, 0
	}

	context, buyTimes, sellTimes, ok := fitContextFromTicks(allTicks, time.Time{}, now)

	if !ok || !context.enoughEvents(buyTimes, sellTimes) {
		return 0, 0, 0
	}

	fit := hawkes.track.FitBivariate(symbol, buyTimes, sellTimes, now)

	if fit.MuBuy <= 0 {
		return 0, 0, 0
	}

	asymmetry := buySellAsymmetry(fit)
	baselineFence := hawkes.track.BaselineIntensityFence(symbol)
	rawConfidence := excitationConfidence(fit, asymmetry, baselineFence)
	runway := excitationRunway(fit)

	if rawConfidence <= 0 || runway <= 0 {
		return 0, 0, 0
	}

	if !snapshot.ImbalanceOK || snapshot.Imbalance <= 0 {
		return 0, 0, 0
	}

	if !snapshot.SpreadOK || snapshot.SpreadBPS <= 0 {
		return 0, 0, 0
	}

	bookSide := snapshot.Imbalance

	if bookSide > 1 {
		bookSide = 1
	}

	score := rawConfidence * bookSide
	confidence := hawkes.track.RecordScore(symbol, score)
	expectedReturn := asymmetry * (snapshot.SpreadBPS / 10000)

	return confidence, expectedReturn, runway
}

func splitSideEvents(
	ticks []market.TradeTick,
	windowStart, windowEnd time.Time,
) ([]time.Time, []time.Time) {
	buyTimes := make([]time.Time, 0, len(ticks))
	sellTimes := make([]time.Time, 0, len(ticks))

	for _, tick := range ticks {
		if tick.Timestamp.Before(windowStart) {
			continue
		}

		if tick.Timestamp.After(windowEnd) {
			continue
		}

		switch tick.Side {
		case "buy":
			buyTimes = append(buyTimes, tick.Timestamp)
		case "sell":
			sellTimes = append(sellTimes, tick.Timestamp)
		}
	}

	return buyTimes, sellTimes
}
