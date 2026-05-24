package hawkes

import (
	"context"
	"iter"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	ingest  *engine.Ingest
	watch   *engine.SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
	trades  *trades.Trades
	track   *TrackStore
}

var _ engine.Signal = (*Hawkes)(nil)

var _ engine.LiveScoreReader = (*Hawkes)(nil)

var _ engine.MeanConfidenceReader = (*Hawkes)(nil)

var _ engine.RiskExporter = (*Hawkes)(nil)

/*
NewHawkes wires live Kraken websocket observers into the engine signal.
*/
func NewHawkes(
	_ context.Context,
	_ *qpool.Q,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Hawkes, error) {
	hawkes := &Hawkes{
		ingest:  engine.NewIngest(book, tradesObserver, tickerObserver),
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		trades:  tradesObserver,
		track:   NewTrackStore(),
	}

	return hawkes, errnie.Require(map[string]any{
		"ingest": hawkes.ingest,
		"trades": tradesObserver,
		"track":  hawkes.track,
	})
}

func (hawkes *Hawkes) Source() string {
	return "hawkes"
}

/*
Tick is a no-op until Hawkes subscribes to market broadcasts.
*/
func (hawkes *Hawkes) Tick() bool {
	return false
}

/*
Measure recalibrates Hawkes intensity and yields non-zero cluster readings.
*/
func (hawkes *Hawkes) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		hawkes.track.BeginScan()
		engine.DrainTicks(ctx)
		hawkes.refreshTracks(ctx)

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  hawkes.Source(),
				Ingest:  hawkes.ingest,
				Watch:   hawkes.watch,
				Pairs:   hawkes.pairs,
				Symbols: hawkes.symbols,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				return hawkes.evaluate(symbol, snapshot, now)
			},
		) {
			if !yield(measurement) {
				return
			}
		}
	}
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
MeanConfidence returns the peak normalized confidence across the latest scan set.
*/
func (hawkes *Hawkes) MeanConfidence() float64 {
	return hawkes.track.PeakLiveConfidence()
}

/*
SymbolRisk exposes Hawkes branching metrics for dynamic execution.
*/
func (hawkes *Hawkes) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	return hawkes.track.SymbolRisk(symbol)
}

/*
Feedback nudges Hawkes excitation parameters from settled prediction error.
*/
func (hawkes *Hawkes) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != hawkes.Source() {
		return
	}

	hawkes.track.ApplyPredictionFeedback(feedback)
}

func (hawkes *Hawkes) refreshTracks(ctx context.Context) {
	for _, symbol := range hawkes.symbols {
		engine.DrainTicks(ctx)

		snapshot := hawkes.ingest.Read(symbol)

		if snapshot.LastOK && snapshot.VolumeOK {
			hawkes.track.ApplyTicker(symbol, snapshot.Last, snapshot.VolumeBase)
		}
	}
}

func (hawkes *Hawkes) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	now time.Time,
) (engine.Measurement, bool, error) {
	confidence, measurementType := hawkes.score(symbol, snapshot, now)

	hawkes.track.ObserveGaugeScore(confidence)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	regime := "momentum"
	reason := "cluster_buy"

	if measurementType == engine.Dump {
		regime = "dump"
		reason = "cluster_sell"
	}

	return engine.Measurement{
		Type:       measurementType,
		Regime:     regime,
		Reason:     reason,
		Confidence: confidence,
	}, true, nil
}

func (hawkes *Hawkes) gaugeConfidence(
	symbol string,
	snapshot engine.Snapshot,
	rawConfidence float64,
	measurementType engine.MeasurementType,
	persist bool,
) float64 {
	if rawConfidence <= 0 {
		return 0
	}

	if !snapshot.ImbalanceOK || snapshot.Imbalance <= 0 {
		return 0
	}

	if !snapshot.SpreadOK || snapshot.SpreadBPS <= 0 {
		return 0
	}

	bookSide := snapshot.Imbalance

	if measurementType == engine.Dump {
		bookSide = math.Abs(snapshot.Imbalance)
	}

	if bookSide > 1 {
		bookSide = 1
	}

	return hawkes.track.applyGaugeScore(symbol, rawConfidence*bookSide, persist)
}

func (hawkes *Hawkes) score(
	symbol string, snapshot engine.Snapshot, now time.Time,
) (float64, engine.MeasurementType) {
	if !hawkes.track.PassesLiquidity(symbol) {
		return 0, engine.Momentum
	}

	allTicks, ok := hawkes.trades.RecentTicks(symbol, time.Time{})

	if !ok || len(allTicks) == 0 {
		return 0, engine.Momentum
	}

	context, buyTimes, sellTimes, ok := fitContextFromTicks(allTicks, time.Time{}, now)

	if !ok || !context.enoughEvents(buyTimes, sellTimes) {
		return 0, engine.Momentum
	}

	fit := hawkes.track.FitBivariate(symbol, buyTimes, sellTimes, now)

	if fit.MuBuy <= 0 {
		return 0, engine.Momentum
	}

	buyAsymmetry := buySellAsymmetry(fit)
	sellAsymmetry := sellBuyAsymmetry(fit)
	baselineFence := hawkes.track.BaselineIntensityFence(symbol)
	measurementType := engine.Momentum
	asymmetry := buyAsymmetry

	if sellAsymmetry > buyAsymmetry {
		measurementType = engine.Dump
		asymmetry = sellAsymmetry
	}

	rawConfidence := excitationConfidence(
		fit, asymmetry, baselineFence, measurementType == engine.Dump,
	)

	if rawConfidence <= 0 {
		return 0, measurementType
	}

	gaugeConfidence := hawkes.gaugeConfidence(
		symbol, snapshot, rawConfidence, measurementType, true,
	)

	if gaugeConfidence <= 0 {
		return 0, measurementType
	}

	return gaugeConfidence, measurementType
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
