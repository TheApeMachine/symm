package trader

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func productionPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 1, runtime.NumCPU(), &qpool.Config{
		SchedulingTimeout: time.Second,
		Regulators: []qpool.Regulator{
			qpool.NewRegulator(qpool.NewCircuitBreaker(10, 10*time.Second, 10)),
			qpool.NewRegulator(qpool.NewRateLimiter(100, time.Second)),
			qpool.NewRegulator(qpool.NewBackPressureRegulator(1000, time.Second, time.Second)),
			qpool.NewRegulator(qpool.NewResourceGovernorRegulator(90, 90, time.Second)),
		},
	})

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func tickerArtifact(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	updates := krakenmarket.TickerUpdates{{
		Symbol:    "BTC/USD",
		Last:      50000,
		Bid:       49999,
		Ask:       50001,
		Volume:    1200,
		ChangePct: 0.01,
		Timestamp: time.Now(),
	}}

	raw, err := sonic.Marshal(updates)

	if err != nil {
		testingTB.Fatal(err)
	}

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("ticker").
		WithPayload(raw)
}

func bookArtifact(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	updates := krakenmarket.BookUpdates{{
		Symbol: "BTC/USD",
		Bids:   []krakenmarket.BookLevel{{Price: 49990, Qty: 2}},
		Asks:   []krakenmarket.BookLevel{{Price: 50010, Qty: 1}},
	}}

	raw, err := sonic.Marshal(updates)

	if err != nil {
		testingTB.Fatal(err)
	}

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("book").
		WithPayload(raw)
}

func tradeArtifact(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	observedAt := time.Now()
	updates := krakenmarket.TradeUpdates{
		&krakenmarket.TradeUpdate{
			Symbol:    "BTC/USD",
			Price:     50000,
			Qty:       0.5,
			Side:      "buy",
			Timestamp: observedAt,
		},
		&krakenmarket.TradeUpdate{
			Symbol:    "BTC/USD",
			Price:     50001,
			Qty:       0.4,
			Side:      "sell",
			Timestamp: observedAt.Add(time.Second),
		},
	}

	raw, err := sonic.Marshal(updates)

	if err != nil {
		testingTB.Fatal(err)
	}

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("trade").
		WithPayload(raw)
}

func TestCryptoOnMessageRegistersScope(testingTB *testing.T) {
	Convey("Given a crypto trader receiving ticker updates", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool)
		messageErr := crypto.onMessage(tickerArtifact(testingTB))

		Convey("It should register the symbol for measurement", func() {
			So(messageErr, ShouldBeNil)

			scopes := crypto.collectMeasureScopes()

			So(scopes, ShouldContain, "BTC/USD")
		})
	})
}

func TestCryptoEvaluateAttentionGating(testingTB *testing.T) {
	Convey("Given resonance surprise statistics", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool)
		symbol := "BTC/USD"

		for range 12 {
			crypto.evaluateAttentionGating(symbol, 1)
		}

		Convey("It should withhold low-surprise probes after warmup", func() {
			So(crypto.evaluateAttentionGating(symbol, 0.01), ShouldBeFalse)
			So(crypto.evaluateAttentionGating(symbol, 100), ShouldBeTrue)
		})
	})
}

func TestCryptoMeasureDashboardSignals(testingTB *testing.T) {
	Convey("Given a closed attention gate and active scopes", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool)

		defer crypto.Close()

		So(crypto.onMessage(tickerArtifact(testingTB)), ShouldBeNil)

		for range 12 {
			crypto.evaluateAttentionGating("BTC/USD", 1)
		}

		So(crypto.evaluateAttentionGating("BTC/USD", 0.01), ShouldBeFalse)

		crypto.recordMeasurement(logic.Measurement{
			Source:     logic.SourceFluid,
			Symbol:     "BTC/USD",
			Price:      50000,
			Strength:   0.5,
			Volume:     1200,
			Spread:     0.1,
			Elapsed:    1,
			Confidence: 0.72,
			Surprise:   1.1,
			ObservedAt: time.Now(),
		}, nil)

		So(len(crypto.story.Measurements()), ShouldEqual, 1)

		Convey("When measure runs", func() {
			crypto.measure()

			measurements := crypto.story.Measurements()

			Convey("It should keep publishable dashboard measurements on the story", func() {
				So(len(measurements), ShouldBeGreaterThan, 0)

				hasFluid := false

				for _, measurement := range measurements {
					if measurement.Source == logic.SourceFluid {
						hasFluid = true
					}
				}

				So(hasFluid, ShouldBeTrue)
			})
		})
	})
}

func TestDashboardSignalNames(testingTB *testing.T) {
	Convey("Given a crypto trader", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		crypto := NewCrypto(context.Background(), pool)

		defer crypto.Close()

		Convey("It should expose every specialist gauge source", func() {
			So(crypto.dashboardSignalNames(), ShouldContain, "fluid")
			So(crypto.dashboardSignalNames(), ShouldContain, "hawkes")
			So(len(crypto.dashboardSignalNames()), ShouldEqual, 14)
		})
	})
}

func TestUpdateSignals(testingTB *testing.T) {
	Convey("Given production qpool regulators", testingTB, func() {
		pool := productionPool(testingTB)
		crypto := NewCrypto(context.Background(), pool)

		defer pool.Close()
		defer crypto.Close()

		artifact := tickerArtifact(testingTB)

		Convey("It should apply signal updates without regulator rejection under burst", func() {
			signalNames := []string{
				"causal",
				"correlation",
				"depthflow",
				"exhaust",
				"fluid",
				"leadlag",
				"liquidity",
				"manifold",
				"resonance",
				"sentiment",
				"toxicity",
			}

			for burst := 0; burst < 500; burst++ {
				updateErr := crypto.updateSignals(artifact, signalNames...)

				So(updateErr, ShouldBeNil)
			}
		})
	})
}
