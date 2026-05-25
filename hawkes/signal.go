package hawkes

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
Hawkes detects buy-side trade clustering via a bivariate self-exciting Hawkes model.
*/
type Hawkes struct {
	engine.Passive
	engine.GaugeScan
	market            *engine.MarketRelay
	watch             *engine.SymbolWatch
	pairs             map[string]asset.Pair
	symbols           []string
	states            map[string]*HawkesSymbol
	calibrationParams engine.CalibrationParams
	pool              *qpool.Q
}

var _ engine.Signal = (*Hawkes)(nil)

var _ engine.LiveScoreReader = (*Hawkes)(nil)

var _ engine.MeanConfidenceReader = (*Hawkes)(nil)

var _ engine.RiskExporter = (*Hawkes)(nil)

/*
NewHawkes wires the shared market broadcast relay into the engine signal.
*/
func NewHawkes(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
	calibrationParams engine.CalibrationParams,
) *Hawkes {
	return &Hawkes{
		market:            marketRelay,
		watch:             watch,
		pairs:             pairs,
		symbols:           append([]string(nil), symbols...),
		states:            make(map[string]*HawkesSymbol),
		calibrationParams: calibrationParams,
		pool:              pool,
	}
}

func (hawkes *Hawkes) Source() string {
	return "hawkes"
}

func (hawkes *Hawkes) state(symbol string) *HawkesSymbol {
	sym := hawkes.states[symbol]

	if sym == nil {
		sym = NewHawkesSymbol(hawkes.calibrationParams)
		hawkes.states[symbol] = sym
	}

	return sym
}

/*
Measure recalibrates Hawkes intensity and yields non-zero cluster readings.
*/
func (hawkes *Hawkes) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		hawkes.beginScan()
		engine.DrainTicks(ctx)
		hawkes.refreshStates(ctx)

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  hawkes.Source(),
				Market:  hawkes.market,
				Watch:   hawkes.watch,
				Pairs:   hawkes.pairs,
				Symbols: hawkes.symbols,
				Pool:    hawkes.pool,
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

func (hawkes *Hawkes) LiveScore() float64 {
	return engine.PeakLiveFromMap(hawkes.states, func(sym *HawkesSymbol) float64 {
		return sym.liveScore
	}).Score
}

func (hawkes *Hawkes) PeakReading() engine.LiveReading {
	return engine.PeakLiveFromMap(hawkes.states, func(sym *HawkesSymbol) float64 {
		return sym.liveScore
	})
}

func (hawkes *Hawkes) MeanConfidence() float64 {
	return hawkes.MeanGaugeConfidence()
}

func (hawkes *Hawkes) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	return hawkes.state(symbol).SymbolRisk()
}

func (hawkes *Hawkes) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != hawkes.Source() {
		return
	}

	hawkes.state(feedback.Symbol).ApplyFeedback(feedback)
}

func (hawkes *Hawkes) beginScan() {
	hawkes.ResetGaugeScan()

	for _, sym := range hawkes.states {
		sym.liveScore = 0
	}
}

func (hawkes *Hawkes) refreshStates(ctx context.Context) {
	_ = engine.RunSymbolJobs(ctx, hawkes.pool, hawkes.symbols, func(symbol string) error {
		snapshot := hawkes.market.Read(symbol)

		if snapshot.LastOK && snapshot.VolumeOK {
			hawkes.state(symbol).FeedTicker(snapshot.Last, snapshot.VolumeBase)
		}

		return nil
	})
}

func (hawkes *Hawkes) passesLiquidity(symbol string) bool {
	sym := hawkes.state(symbol)

	quotes := make(map[string]float64, len(hawkes.states))

	for name, state := range hawkes.states {
		if state.dailyQuoteVol > 0 {
			quotes[name] = state.dailyQuoteVol
		}
	}

	return engine.PassesBelowMedianLiquidity(
		sym.dailyQuoteVol,
		quotes,
		symbol,
		1,
	)
}

func (hawkes *Hawkes) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	now time.Time,
) (engine.Measurement, bool, error) {
	if !hawkes.passesLiquidity(symbol) {
		return engine.Measurement{}, false, nil
	}

	ticks, ok := hawkes.market.RecentTicks(symbol, time.Time{})

	if !ok || len(ticks) == 0 {
		return engine.Measurement{}, false, nil
	}

	measurement, ok := hawkes.state(symbol).Measure(
		ticks,
		snapshot,
		now,
		hawkes.pairs[symbol],
	)

	hawkes.ObserveGaugeScore(measurement.Confidence)

	return measurement, ok, nil
}
