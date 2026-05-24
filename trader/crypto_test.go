package trader

import (
	"context"
	"iter"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

type stubPrices map[string]float64

func (stub stubPrices) Last(symbol string) (float64, bool) {
	price, ok := stub[symbol]
	return price, ok
}

func (stub stubPrices) Quote(symbol string) (float64, float64, float64, float64, bool) {
	last, ok := stub.Last(symbol)

	if !ok {
		return 0, 0, 0, 0, false
	}

	return last, last * 0.999, last * 1.001, 0, true
}

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

type sourceStubSignal struct {
	stubSignal
	source string
}

func (signal *sourceStubSignal) Source() string {
	return signal.source
}

func (stub *stubSignal) Feedback(_ engine.PredictionFeedback) {}

type scoredSignal struct {
	stubSignal
	liveScore float64
}

func (signal *scoredSignal) LiveScore() float64 {
	return signal.liveScore
}

func (signal *scoredSignal) PeakReading() engine.LiveReading {
	return engine.LiveReading{
		Symbol: "PUMP/EUR",
		Score:  signal.liveScore,
	}
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

func testCrypto(
	t *testing.T,
	prices QuoteReader,
	signals ...engine.Signal,
) *Crypto {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	crypto, err := NewCrypto(ctx, pool, nil, wallet, prices, signals...)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	return crypto
}

func TestUpdatePairStatesRecordsTraderForecast(t *testing.T) {
	crypto := testCrypto(t, stubPrices{"PUMP/EUR": 100}, &stubSignal{})
	now := time.Unix(1_700_000_000, 0)

	crypto.updatePairStates(testMeasurement(0.5), now)

	state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})

	if state == nil || !state.HasPendingPredictions() {
		t.Fatal("expected trader forecast to be recorded")
	}
}

func TestProcessSignalsSequential(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
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

	crypto, err := NewCrypto(
		ctx,
		pool,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&measureSignal{onMeasure: blockingMeasure},
		&measureSignal{onMeasure: blockingMeasure},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	convey.Convey("Given a qpool-backed trader", t, func() {
		done := make(chan struct{}, 1)

		go func() {
			crypto.processSignals(start)
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

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		ctx,
		pool,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100, "DUMP/EUR": 50},
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
		t.Fatalf("new crypto: %v", err)
	}

	convey.Convey("Given multiple signals on a qpool", t, func() {
		crypto.processSignals(start)

		convey.Convey("It should update pair state for every signal", func() {
			pumpState := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
			dumpState := crypto.pairState(asset.Pair{Wsname: "DUMP/EUR"})

			convey.So(pumpState.HasPendingPredictions(), convey.ShouldBeTrue)
			convey.So(dumpState.HasPendingPredictions(), convey.ShouldBeTrue)
		})
	})
}

func TestProcessSignals(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	measured := false

	crypto := testCrypto(
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
		crypto.processSignals(start)

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

		crypto := testCrypto(t, stubPrices{}, signal)

		if err := crypto.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}
		state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should not record a forecast without a quote", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given repeated measurements from one source", t, func() {
		signal := &stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}}

		crypto := testCrypto(t, stubPrices{"PUMP/EUR": 100}, signal)

		if err := crypto.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		if err := crypto.processSignal(signal, start.Add(100*time.Millisecond)); err != nil {
			t.Fatalf("process signal: %v", err)
		}
		state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should keep one open forecast per source", func() {
			convey.So(state.PendingCount(), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given feedback receivers", t, func() {
		signal := &feedbackSignal{stubSignal: stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}}}

		crypto := testCrypto(t, stubPrices{"PUMP/EUR": 100}, signal)

		if err := crypto.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should not apply feedback before the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 0)
		})

		if err := crypto.processSignal(signal, start.Add(16*time.Second)); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should apply feedback once the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 1)
			convey.So(signal.feedback[0].PredictedReturn, convey.ShouldAlmostEqual, 0.001, 1e-9)
		})
	})
}

func TestRescoreCollectsFeedback(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	feedback := make([]engine.PredictionFeedback, 0)

	crypto := testCrypto(
		t,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	crypto.BindFeedbackSink(func(item engine.PredictionFeedback) {
		feedback = append(feedback, item)
	})

	if err := crypto.Rescore(start); err != nil {
		t.Fatalf("rescore: %v", err)
	}

	if crypto.PendingPredictionCount() != 1 {
		t.Fatalf("expected one pending forecast, got %d", crypto.PendingPredictionCount())
	}

	if err := crypto.Rescore(start.Add(16 * time.Second)); err != nil {
		t.Fatalf("settle rescore: %v", err)
	}

	if len(feedback) != 1 {
		t.Fatalf("expected one settled feedback, got %d", len(feedback))
	}

	if feedback[0].Confidence != 0.5 {
		t.Fatalf("expected confidence on feedback, got %v", feedback[0].Confidence)
	}

	if math.Abs(feedback[0].PredictedReturn-0.001) > 1e-9 {
		t.Fatalf("expected trader forecast return, got %v", feedback[0].PredictedReturn)
	}
}

func BenchmarkProcessSignals(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		ctx,
		pool,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	if err != nil {
		b.Fatalf("new crypto: %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		crypto.processSignals(start)
	}
}

func BenchmarkProcessSignalsSequential(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		ctx,
		pool,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.5),
		}},
	)

	if err != nil {
		b.Fatalf("new crypto: %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		crypto.processSignals(start)
	}
}
