package leadlag

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const leadlagSource = "leadlag"

/*
LeadLag detects when one symbol leads the cross-section and a laggard has not caught up.
*/
type LeadLag struct {
	market  *engine.MarketRelay
	watch   *engine.SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
	track   *TrackStore
	pool    *qpool.Q
}

var _ engine.Signal = (*LeadLag)(nil)

var _ engine.MeanConfidenceReader = (*LeadLag)(nil)

/*
NewLeadLag wires the market relay into the lead-lag signal.
*/
func NewLeadLag(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*LeadLag, error) {
	leadlag := &LeadLag{
		market:  marketRelay,
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		track:   NewTrackStore(),
		pool:    pool,
	}

	return leadlag, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  leadlag.track,
	})
}

func (leadlag *LeadLag) Source() string {
	return leadlagSource
}

func (leadlag *LeadLag) MeanConfidence() float64 {
	return leadlag.track.MeanGaugeConfidence()
}

func (leadlag *LeadLag) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != leadlagSource {
		return
	}

	leadlag.track.ApplyPredictionFeedback(feedback)
}

func (leadlag *LeadLag) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		leadlag.track.BeginScan()
		leadlag.refreshLeader()

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  leadlag.Source(),
				Market:  leadlag.market,
				Watch:   leadlag.watch,
				Pairs:   leadlag.pairs,
				Symbols: leadlag.symbols,
				Pool:    leadlag.pool,
			},
			now,
			leadlag.evaluate,
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (leadlag *LeadLag) refreshLeader() {
	volumes := make(map[string]float64, len(leadlag.symbols))

	for _, symbol := range leadlag.symbols {
		snapshot := leadlag.market.Read(symbol)

		if !snapshot.BatchOK {
			continue
		}

		volumes[symbol] = snapshot.BatchVolume
	}

	leader := pickLeader(volumes)

	if leader == "" {
		return
	}

	leadlag.track.setLeader(leader)
}

func (leadlag *LeadLag) evaluate(
	symbol string,
	snapshot engine.Snapshot,
) (engine.Measurement, bool, error) {
	if !snapshot.LastOK || snapshot.Last <= 0 {
		return engine.Measurement{}, false, nil
	}

	followerReturn := leadlag.track.recordReturn(symbol, snapshot.Last)
	leaderReturn := leadlag.track.leaderReturn()

	if leaderReturn == 0 {
		return engine.Measurement{}, false, nil
	}

	if symbol == leadlag.track.Leader() {
		return engine.Measurement{}, false, nil
	}

	raw := leadLagScore(leaderReturn, followerReturn)

	if raw <= 0 {
		return engine.Measurement{}, false, nil
	}

	track := leadlag.track.ensure(symbol)
	scale := track.calibrator.Scale()

	if scale <= 0 {
		return engine.Measurement{}, false, nil
	}

	confidence := leadlag.track.recordScore(symbol, raw*scale)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	pair, ok := leadlag.pairs[symbol]

	if !ok {
		return engine.Measurement{}, false, nil
	}

	measurementType := engine.LeadLag

	if leaderReturn < 0 {
		measurementType = engine.Dump
	}

	return engine.Measurement{
		Type:       measurementType,
		Source:     leadlagSource,
		Regime:     "cross",
		Reason:     "lead_lag",
		Pairs:      []asset.Pair{pair},
		Confidence: confidence,
	}, true, nil
}
