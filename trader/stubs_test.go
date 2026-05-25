package trader

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type stubSignal struct {
	measurements []engine.Measurement
}

func (stub *stubSignal) Measure(_ context.Context, _ time.Time) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		for _, measurement := range stub.measurements {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (stub *stubSignal) Source() string { return "stub" }

func (stub *stubSignal) Feedback(_ engine.PredictionFeedback) {}

type sourceStubSignal struct {
	stubSignal
	source string
}

func (signal *sourceStubSignal) Source() string {
	return signal.source
}

type feedbackSignal struct {
	stubSignal
	feedback []engine.PredictionFeedback
}

func (signal *feedbackSignal) Feedback(feedback engine.PredictionFeedback) {
	signal.feedback = append(signal.feedback, feedback)
}

type measureSignal struct {
	stubSignal
	onMeasure func()
}

func (signal *measureSignal) Measure(ctx context.Context, now time.Time) iter.Seq[engine.Measurement] {
	if signal.onMeasure != nil {
		signal.onMeasure()
	}

	return signal.stubSignal.Measure(ctx, now)
}

func testMeasurement(confidence float64) engine.Measurement {
	return engine.Measurement{
		Source:     "hawkes",
		Type:       engine.Momentum,
		Regime:     "momentum",
		Reason:     "cluster_buy",
		Pairs:      []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence: confidence,
	}
}

func testForecast(expectedReturn float64, runway time.Duration) SignalForecast {
	return SignalForecast{
		ExpectedReturn: expectedReturn,
		Runway:         runway,
	}
}

func seedScorerReturnModel(scorer *Scorer, source, regime string) {
	if scorer == nil || scorer.returnModel == nil {
		return
	}

	samples := 12

	for range samples {
		scorer.returnModel.Apply(engine.PredictionFeedback{
			Source:          source,
			Regime:          regime,
			PredictedReturn: 0.01,
			ActualReturn:    0.02,
		})
	}
}

func testScorer(
	t *testing.T,
	prices QuoteReader,
	signals ...engine.Signal,
) *Scorer {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	scorer, err := NewScorer(ctx, pool, nil, candidatesGroup, prices, nil, signals...)

	if err != nil {
		t.Fatalf("new scorer: %v", err)
	}

	seedScorerReturnModel(scorer, "hawkes", "momentum")
	seedScorerReturnModel(scorer, "fluid", "flow")
	seedScorerReturnModel(scorer, "stub", "momentum")

	return scorer
}
