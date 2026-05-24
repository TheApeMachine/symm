package sentiment

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const sentimentSource = "sentiment"

/*
Sentiment aggregates cross-section trade-pressure and momentum breadth into one score.
*/
type Sentiment struct {
	market  *engine.MarketRelay
	watch   *engine.SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
	track   *TrackStore
	pool    *qpool.Q
}

var _ engine.Signal = (*Sentiment)(nil)

var _ engine.MeanConfidenceReader = (*Sentiment)(nil)

/*
NewSentiment wires the market relay into the sentiment signal.
*/
func NewSentiment(
	_ context.Context,
	pool *qpool.Q,
	marketRelay *engine.MarketRelay,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *engine.SymbolWatch,
) (*Sentiment, error) {
	sentiment := &Sentiment{
		market:  marketRelay,
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
		track:   NewTrackStore(),
		pool:    pool,
	}

	return sentiment, errnie.Require(map[string]any{
		"market": marketRelay,
		"track":  sentiment.track,
	})
}

func (sentiment *Sentiment) Source() string {
	return sentimentSource
}

func (sentiment *Sentiment) MeanConfidence() float64 {
	return sentiment.track.MeanGaugeConfidence()
}

func (sentiment *Sentiment) Feedback(feedback engine.PredictionFeedback) {
	if feedback.Source != sentimentSource {
		return
	}

	sentiment.track.ApplyPredictionFeedback(feedback)
}

func (sentiment *Sentiment) Measure(
	ctx context.Context,
	now time.Time,
) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		sentiment.track.BeginScan()
		features := sentiment.collectFeatures()

		for measurement := range engine.MeasureSymbols(
			ctx,
			engine.SymbolScanner{
				Source:  sentiment.Source(),
				Market:  sentiment.market,
				Watch:   sentiment.watch,
				Pairs:   sentiment.pairs,
				Symbols: sentiment.symbols,
				Pool:    sentiment.pool,
			},
			now,
			func(symbol string, snapshot engine.Snapshot) (engine.Measurement, bool, error) {
				return sentiment.evaluate(symbol, snapshot, features)
			},
		) {
			if !yield(measurement) {
				return
			}
		}
	}
}

type sectionFeatures struct {
	pressures []float64
	changes   []float64
}

func (sentiment *Sentiment) collectFeatures() sectionFeatures {
	features := sectionFeatures{
		pressures: make([]float64, 0, len(sentiment.symbols)),
		changes:   make([]float64, 0, len(sentiment.symbols)),
	}

	for _, symbol := range sentiment.symbols {
		snapshot := sentiment.market.Read(symbol)

		if snapshot.PressureOK {
			features.pressures = append(features.pressures, snapshot.BuyPressure)
		}

		if snapshot.ChangeOK {
			features.changes = append(features.changes, snapshot.ChangePct)
		}
	}

	return features
}

func (sentiment *Sentiment) evaluate(
	symbol string,
	snapshot engine.Snapshot,
	features sectionFeatures,
) (engine.Measurement, bool, error) {
	if !snapshot.PressureOK && !snapshot.ChangeOK {
		return engine.Measurement{}, false, nil
	}

	pressure := 0.0

	if snapshot.PressureOK {
		pressure = snapshot.BuyPressure
	}

	change := 0.0

	if snapshot.ChangeOK {
		change = snapshot.ChangePct
	}

	raw := sentimentRaw(
		crossSectionZScore(pressure, features.pressures),
		crossSectionZScore(change, features.changes),
	)

	if raw <= 0 {
		return engine.Measurement{}, false, nil
	}

	track := sentiment.track.ensure(symbol)
	track.recordSentiment(raw)

	scale := track.calibrator.Scale()

	if scale <= 0 {
		return engine.Measurement{}, false, nil
	}

	confidence := sentiment.track.recordScore(symbol, raw*scale)

	if confidence <= 0 {
		return engine.Measurement{}, false, nil
	}

	pair, ok := sentiment.pairs[symbol]

	if !ok {
		return engine.Measurement{}, false, nil
	}

	measurementType := engine.Sentiment

	if pressure+change < 0 {
		measurementType = engine.Dump
	}

	return engine.Measurement{
		Type:       measurementType,
		Source:     sentimentSource,
		Regime:     "sentiment",
		Reason:     "flow_breadth",
		Pairs:      []asset.Pair{pair},
		Confidence: confidence,
	}, true, nil
}
