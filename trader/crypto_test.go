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

type stubSignal struct {
	measurements []engine.Measurement
}

func (stub *stubSignal) Scan(_ time.Time) error { return nil }

func (stub *stubSignal) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		for _, measurement := range stub.measurements {
			if !yield(measurement) {
				return
			}
		}
	}
}

func (stub *stubSignal) Source() string { return "stub" }

func (stub *stubSignal) Stats() engine.QueueStats { return engine.QueueStats{} }

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

func (signal *feedbackSignal) ApplyFeedback(feedback engine.PredictionFeedback) {
	signal.feedback = append(signal.feedback, feedback)
}

type scanSignal struct {
	stubSignal
	onScan func()
}

func (signal *scanSignal) Scan(now time.Time) error {
	if signal.onScan != nil {
		signal.onScan()
	}

	return signal.stubSignal.Scan(now)
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

func TestScanSignalsConcurrent(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)
	release := make(chan struct{})

	var activeMu sync.Mutex
	active := 0
	peak := 0

	blockingScan := func() {
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
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&scanSignal{onScan: blockingScan},
		&scanSignal{onScan: blockingScan},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	convey.Convey("Given a qpool-backed trader", t, func() {
		done := make(chan error, 1)

		go func() {
			done <- crypto.scanSignals(start)
		}()

		time.Sleep(20 * time.Millisecond)
		close(release)

		convey.Convey("It should scan signals concurrently", func() {
			convey.So(<-done, convey.ShouldBeNil)
			convey.So(peak, convey.ShouldEqual, 2)
		})
	})
}

func TestProcessTickConcurrent(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		ctx,
		pool,
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
		crypto.processTick(start)

		convey.Convey("It should update pair state for every signal", func() {
			pumpState := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})
			dumpState := crypto.pairState(asset.Pair{Wsname: "DUMP/EUR"})

			convey.So(pumpState.HasPendingPredictions(), convey.ShouldBeTrue)
			convey.So(dumpState.HasPendingPredictions(), convey.ShouldBeTrue)
		})
	})
}

func BenchmarkProcessTick(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	crypto, err := NewCrypto(
		ctx,
		pool,
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
		crypto.processTick(start)
	}
}

func TestScanSignals(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)
	scanned := false

	crypto, err := NewCrypto(
		context.Background(),
		nil,
		wallet,
		stubPrices{"PUMP/EUR": 100},
		&scanSignal{
			stubSignal: stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.002, time.Second),
			}},
			onScan: func() { scanned = true },
		},
	)

	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}

	convey.Convey("Given registered signals", t, func() {
		err := crypto.scanSignals(start)

		convey.Convey("It should scan every signal", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(scanned, convey.ShouldBeTrue)
		})
	})
}

func TestProcessTick(t *testing.T) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	start := time.Unix(1_700_000_000, 0)

	convey.Convey("Given no price reader", t, func() {
		crypto, err := NewCrypto(
			context.Background(),
			nil,
			wallet,
			stubPrices{},
			&stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.002, time.Second),
			}},
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

		crypto.processTick(start)
		state := crypto.pairState(asset.Pair{Wsname: "PUMP/EUR"})

		convey.Convey("It should still record the forecast", func() {
			convey.So(state.HasPendingPredictions(), convey.ShouldBeTrue)
		})
	})

	convey.Convey("Given repeated measurements from one source", t, func() {
		crypto, err := NewCrypto(
			context.Background(),
			nil,
			wallet,
			stubPrices{"PUMP/EUR": 100},
			&stubSignal{measurements: []engine.Measurement{
				testMeasurement(0.002, time.Second),
			}},
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

		crypto.processTick(start)
		crypto.processTick(start.Add(100 * time.Millisecond))
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
			wallet,
			stubPrices{"PUMP/EUR": 100},
			signal,
		)

		if err != nil {
			t.Fatalf("new crypto: %v", err)
		}

		crypto.processTick(start)

		convey.Convey("It should not apply feedback before the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 0)
		})

		crypto.processTick(start.Add(5 * time.Second))

		convey.Convey("It should apply feedback once the runway elapses", func() {
			convey.So(len(signal.feedback), convey.ShouldEqual, 1)
			convey.So(signal.feedback[0].PredictedReturn, convey.ShouldEqual, 0.002)
		})
	})
}
