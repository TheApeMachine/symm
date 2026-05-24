package trader

import (
	"context"
	"iter"
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

func testMeasurement(expectedReturn float64, runway time.Duration) engine.Measurement {
	return engine.Measurement{
		Source:         "hawkes",
		Type:           engine.Momentum,
		Regime:         "momentum",
		Reason:         "cluster_buy",
		Pairs:          []asset.Pair{{Wsname: "PUMP/EUR"}},
		Confidence:     0.5,
		ExpectedReturn: expectedReturn,
		Runway:         runway,
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
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.002, time.Second),
		}},
		&stubSignal{measurements: []engine.Measurement{
			{
				Source:         "hawkes",
				Type:           engine.Momentum,
				Regime:         "momentum",
				Reason:         "cluster_sell",
				Pairs:          []asset.Pair{{Wsname: "DUMP/EUR"}},
				Confidence:     0.4,
				ExpectedReturn: 0.001,
				Runway:         time.Second,
			},
		}},
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
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)
	measured := false

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&measureSignal{
			stubSignal: stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.002, time.Second),
			}},
			onMeasure: func() { measured = true },
		},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	convey.Convey("Given registered signals", t, func() {
		crypto.processSignals(start)

		convey.Convey("It should measure every signal", func() {
			convey.So(measured, convey.ShouldBeTrue)
		})
	})
}

func TestProcessSignal(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	convey.Convey("Given no price reader", t, func() {
		signal := &stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.002, time.Second),
		}}

		crypto, err := NewCrypto(
			context.Background(),
			nil,
			nil,
			wallet,
			stubPrices{},
			signal,
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

		if err := crypto.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}
		state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should still record the forecast", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeTrue)
		})
	})

	convey.Convey("Given repeated measurements from one source", t, func() {
		signal := &stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.002, time.Second),
		}}

		crypto, err := NewCrypto(
			context.Background(),
			nil,
			nil,
			wallet,
			stubPrices{"PUMP/EUR": 100},
			signal,
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

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
			testMeasurement(0.002, 5*time.Second),
		}}}

		crypto, err := NewCrypto(
			context.Background(),
			nil,
			nil,
			wallet,
			stubPrices{"PUMP/EUR": 100},
			signal,
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

		if err := crypto.processSignal(signal, start); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should not apply feedback before the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 0)
		})

		if err := crypto.processSignal(signal, start.Add(5*time.Second)); err != nil {
			t.Fatalf("process signal: %v", err)
		}

		convey.Convey("It should apply feedback once the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 1)
			convey.So(signal.feedback[0].PredictedReturn, convey.ShouldEqual, 0.002)
		})
	})
}

func TestRescoreCollectsFeedback(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)
	feedback := make([]engine.PredictionFeedback, 0)

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.002, 5*time.Second),
		}},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	crypto.BindFeedbackSink(func(item engine.PredictionFeedback) {
		feedback = append(feedback, item)
	})

	if err := crypto.Rescore(start); err != nil {
		t.Fatalf("rescore: %v", err)
	}

	if crypto.PendingPredictionCount() != 1 {
		t.Fatalf("expected one pending forecast, got %d", crypto.PendingPredictionCount())
	}

	if err := crypto.Rescore(start.Add(5 * time.Second)); err != nil {
		t.Fatalf("settle rescore: %v", err)
	}

	if len(feedback) != 1 {
		t.Fatalf("expected one settled feedback, got %d", len(feedback))
	}

	if feedback[0].Confidence != 0.5 {
		t.Fatalf("expected confidence on feedback, got %v", feedback[0].Confidence)
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
			testMeasurement(0.002, time.Second),
		}},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.001, time.Second),
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
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&stubSignal{measurements: []engine.Measurement{
			testMeasurement(0.002, time.Second),
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
