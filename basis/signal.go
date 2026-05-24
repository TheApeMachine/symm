package basis

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const basisSource = "basis"

/*
Basis measures relative strength versus the cross-section as a spot premium proxy.
*/
type Basis struct {
	market  *engine.MarketRelay
	watch   *engine.SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
	track   *TrackStore
	pool    *qpool.Q
}

var _ engine.Signal = (*Basis)(nil)

var _ engine.MeanConfidenceReader = (*Basis)(nil)

/*
NewBasis wires the market relay into the basis signal.
*/
func NewBasis(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Basis, error) {
	basis := &Basis{
		market:  marketRelay,
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		track:   NewTrackStore(),
		pool:    pool,
	}

	return basis, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  basis.track,
	})
}

func (basis *Basis) Source() string {
	return basisSource
}

func (basis *Basis) MeanConfidence() float64 {
	return basis.track.MeanGaugeConfidence()
}

func (basis *Basis) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != basisSource {
		return
	}

	basis.track.ApplyPredictionFeedback(feedback)
}

func (basis *Basis) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		basis.track.BeginScan()
		changes := basis.collectChanges()

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  basis.Source(),
				Market:  basis.market,
				Watch:   basis.watch,
				Pairs:   basis.pairs,
				Symbols: basis.symbols,
				Pool:    basis.pool,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				return basis.evaluate(symbol, snapshot, changes)
			},
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (basis *Basis) collectChanges() map[string]float64 {
	changes := make(map[string]float64, len(basis.symbols))

	for _, symbol := range basis.symbols {
		snapshot := basis.market.Read(symbol)

		if !snapshot.ChangeOK {
			continue
		}

		changes[symbol] = snapshot.ChangePct
	}

	return changes
}

func (basis *Basis) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	changes map[string]float64,
) (engine.Measurement, bool, error) {
	if !snapshot.ChangeOK {
		return engine.Measurement{}, false, nil
	}

	crossMedian := crossSectionMedianChange(changes)
	relStrength := snapshot.ChangePct - crossMedian
	raw := basisScore(snapshot.ChangePct, crossMedian)

	if raw <= 0 {
		return engine.Measurement{}, false, nil
	}

	track := basis.track.ensure(symbol)
	track.recordRelativeStrength(relStrength)

	scale := track.calibrator.Scale()

	if scale <= 0 {
		return engine.Measurement{}, false, nil
	}

	confidence := basis.track.recordScore(symbol, raw*scale)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	pair, ok := basis.pairs[symbol]

	if !ok {
		return engine.Measurement{}, false, nil
	}

	measurementType := engine.Basis

	if relStrength < 0 {
		measurementType = engine.Dump
	}

	return engine.Measurement{
		Type:       measurementType,
		Source:     basisSource,
		Regime:     "basis",
		Reason:     "relative_strength",
		Pairs:      []asset.Pair{pair},
		Confidence: confidence,
	}, true, nil
}
