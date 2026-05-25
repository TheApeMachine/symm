package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestUpdatePairStatesRecordsTraderForecast(t *testing.T) {
	scorer := testScorer(t, stubPrices{"PUMP/EUR": 100}, &stubSignal{})
	now := time.Unix(1_700_000_000, 0)

	scorer.updatePairStates(testMeasurement(0.5), now)

	state := scorer.pairState(asset.Pair{Wsname: "PUMP/EUR"})

	if state == nil || !state.HasPendingPredictions() {
		t.Fatal("expected trader forecast to be recorded")
	}
}

func TestProcessSignalsSequential(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	start := time.Unix(1_700_000_000, 0)
	release := make(chan struct{})

	var activeMu sync.Mutex
	active := 0
	peak := 0

	blockingMeasure := func() {
		activeMu.Lock()
		active++
		if active > peak {
			peak = active
		}
		activeMu.Unlock()

		<-release

		activeMu.Lock()
		active--
		activeMu.Unlock()
	}

	scorer, err := NewScorer(
		ctx,
		pool,
		nil,
		candidatesGroup,
		stubPrices{"PUMP/EUR": 100},
		nil,
		&measureSignal{onMeasure: blockingMeasure},
		&measureSignal{onMeasure: blockingMeasure},
	)

	if err != nil {
		t.Fatalf("new scorer: %v", err)
	}

	seedScorerReturnModel(scorer, "hawkes", "momentum")
	seedScorerReturnModel(scorer, "fluid", "flow")

	convey.Convey("Given a qpool-backed scorer", t, func() {
		done := make(chan struct{}, 1)

		go func() {
			scorer.processSignals(start)
			done <- struct{}{}
		}()

		time.Sleep(20 * time.Millisecond)
		close(release)

		convey.Convey("It should process signals sequentially on the orchestrator thread", func() {
			<-done
			convey.So(peak, convey.ShouldEqual, 1)
		})
	})
}

func TestProcessSignalsDrainsEverySignal(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	start := time.Unix(1_700_000_000, 0)

	scorer, err := NewScorer(
		ctx,
		pool,
		nil,
		candidatesGroup,
		stubPrices{"PUMP/EUR": 100, "DUMP/EUR": 50},
		nil,
		&sourceStubSignal{
			source: "hawkes",
			stubSignal: stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.5),
			}},
		},
		&sourceStubSignal{
			source: "fluid",
			stubSignal: stubSignal{measurements: []engine.Measurement{
				{
					Source:     "fluid",
					Type:       engine.Flow,
					Regime:     "flow",
					Reason:     "shock",
					Pairs:      []asset.Pair{{Wsname: "DUMP/EUR"}},
					Confidence: 0.4,
				},
			}},
		},
	)

	if err != nil {
		t.Fatalf("new scorer: %v", err)
	}

	seedScorerReturnModel(scorer, "hawkes", "momentum")
	seedScorerReturnModel(scorer, "fluid", "flow")

	convey.Convey("Given multiple signals on a qpool", t, func() {
		scorer.processSignals(start)

		convey.Convey("It should update pair state for every signal", func() {
			pumpState := scorer.pairState(asset.Pair{Wsname: "PUMP/EUR"})
			dumpState := scorer.pairState(asset.Pair{Wsname: "DUMP/EUR"})

			convey.So(pumpState.HasPendingPredictions(), convey.ShouldBeTrue)
			convey.So(dumpState.HasPendingPredictions(), convey.ShouldBeTrue)
		})
	})
}

func TestProcessSignals(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	measured := false

	scorer := testScorer(
		t,
		stubPrices{"PUMP/EUR": 100},
		&measureSignal{
			stubSignal: stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.5),
			}},
			onMeasure: func() { measured = true },
		},
	)

	convey.Convey("Given registered signals", t, func() {
		scorer.processSignals(start)

		convey.Convey("It should measure every signal", func() {
			convey.So(measured, convey.ShouldBeTrue)
		})
	})
}

func TestProcessSignal(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)

	convey.Convey("Given no live quote", t, func() {
		signal := &stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}}

		scorer := testScorer(t, stubPrices{}, signal)

		if err := scorer.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}
		state := scorer.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should not record a forecast without a quote", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given repeated measurements from one source", t, func() {
		signal := &stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}}

		scorer := testScorer(t, stubPrices{"PUMP/EUR": 100}, signal)

		if err := scorer.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		if err := scorer.processSignal(signal, start.Add(100*time.Millisecond)); err != nil {
			t.Fatalf("process signal: %v", err)
		}
		state := scorer.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should keep one open forecast per source", func() {
			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given feedback receivers", t, func() {
		signal := &feedbackSignal{stubSignal: stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}}}

		scorer := testScorer(t, stubPrices{"PUMP/EUR": 100}, signal)

		if err := scorer.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should not apply feedback before the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 0)
		})

		if err := scorer.processSignal(signal, start.Add(16*time.Second)); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should apply feedback once the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 1)
			convey.So(signal.feedback[0].PredictedReturn, convey.ShouldAlmostEqual, 0.01, 1e-9)
		})
	})
}

func TestRescoreCollectsFeedback(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	feedback := make([]engine.PredictionFeedback, 0)

	scorer := testScorer(
		t,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	scorer.BindFeedbackSink(func(item engine.PredictionFeedback) {
		feedback = append(feedback, item)
	})

	if err := scorer.Rescore(start); err != nil {
		t.Fatalf("rescore: %v", err)
	}

	if scorer.PendingPredictionCount() != 1 {
		t.Fatalf("expected one pending forecast, got %d", scorer.PendingPredictionCount())
	}

	if err := scorer.Rescore(start.Add(16 * time.Second)); err != nil {
		t.Fatalf("settle rescore: %v", err)
	}

	if len(feedback) != 1 {
		t.Fatalf("expected one settled feedback, got %d", len(feedback))
	}

	if feedback[0].Confidence != 0.5 {
		t.Fatalf("expected confidence on feedback, got %v", feedback[0].Confidence)
	}

	if math.Abs(feedback[0].PredictedReturn-0.01) > 1e-9 {
		t.Fatalf("expected trader forecast return, got %v", feedback[0].PredictedReturn)
	}
}

func BenchmarkProcessSignals(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	start := time.Unix(1_700_000_000, 0)

	scorer, err := NewScorer(
		ctx,
		pool,
		nil,
		candidatesGroup,
		stubPrices{"PUMP/EUR": 100},
		nil,
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	if err != nil {
		b.Fatalf("new scorer: %v", err)
	}

	seedScorerReturnModel(scorer, "hawkes", "momentum")

	b.ReportAllocs()

	for b.Loop() {
		scorer.processSignals(start)
	}
}

func BenchmarkProcessSignalsSequential(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	candidatesGroup := pool.CreateBroadcastGroup("candidates", 10*time.Millisecond)
	start := time.Unix(1_700_000_000, 0)

	scorer, err := NewScorer(
		ctx,
		pool,
		nil,
		candidatesGroup,
		stubPrices{"PUMP/EUR": 100},
		nil,
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	if err != nil {
		b.Fatalf("new scorer: %v", err)
	}

	seedScorerReturnModel(scorer, "hawkes", "momentum")

	b.ReportAllocs()

	for b.Loop() {
		scorer.processSignals(start)
	}
}
