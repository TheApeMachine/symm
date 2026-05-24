package depthflow

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const depthflowSource = "depthflow"

/*
DepthFlow detects multi-level order-book imbalance and depth-weighted flow pressure.
*/
type DepthFlow struct {
	market *engine.MarketRelay
	watch  *engine.SymbolWatch
	pairs  map[string]asset.Pair
	track  *TrackStore
	pool   *qpool.Q
}

var _ engine.Signal = (*DepthFlow)(nil)

var _ engine.MeanConfidenceReader = (*DepthFlow)(nil)

/*
NewDepthFlow wires the market relay into the depth-flow signal.
*/
func NewDepthFlow(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	watch *engine.SymbolWatch,
) (*DepthFlow, error) {
	depthflow := &DepthFlow{
		market: marketRelay,
		watch:  watch,
		pairs:  pairs,
		track:  NewTrackStore(),
		pool:   pool,
	}

	return depthflow, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  depthflow.track,
	})
}

func (depthflow *DepthFlow) Source() string {
	return depthflowSource
}

func (depthflow *DepthFlow) MeanConfidence() float64 {
	return depthflow.track.MeanGaugeConfidence()
}

func (depthflow *DepthFlow) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != depthflowSource {
		return
	}

	depthflow.track.ApplyPredictionFeedback(feedback)
}

func (depthflow *DepthFlow) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		depthflow.track.BeginScan()

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source: depthflow.Source(),
				Market: depthflow.market,
				Watch:  depthflow.watch,
				Pairs:  depthflow.pairs,
				Pool:   depthflow.pool,
			},
			now,
			depthflow.evaluate,
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (depthflow *DepthFlow) evaluate(
	symbol string,
	snapshot engine.Snapshot,
) (engine.Measurement, bool, error) {
	if !snapshot.DepthOK {
		return engine.Measurement{}, false, nil
	}

	imbalance := depthImbalanceAtLevels(snapshot.BidLevels, snapshot.AskLevels)

	if imbalance == 0 {
		return engine.Measurement{}, false, nil
	}

	track := depthflow.track.ensure(symbol)
	track.recordDepthImbalance(imbalance)

	scale := track.calibrator.Scale()

	if scale <= 0 {
		return engine.Measurement{}, false, nil
	}

	raw := absFloat(imbalance) * scale

	if snapshot.BuyPressure > 0 && imbalance > 0 {
		raw *= (snapshot.BuyPressure + 1) / 2
	}

	if snapshot.BuyPressure < 0 && imbalance < 0 {
		raw *= (1 - snapshot.BuyPressure) / 2
	}

	confidence := depthflow.track.recordScore(symbol, raw)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	pair, ok := depthflow.pairs[symbol]

	if !ok {
		return engine.Measurement{}, false, nil
	}

	measurementType := engine.DepthFlow

	if imbalance < 0 {
		measurementType = engine.Dump
	}

	return engine.Measurement{
		Type:       measurementType,
		Source:     depthflowSource,
		Regime:     "depth",
		Reason:     "depth_imbalance",
		Pairs:      []asset.Pair{pair},
		Confidence: confidence,
	}, true, nil
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
