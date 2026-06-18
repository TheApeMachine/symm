package trader

import (
	"context"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
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

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("ticker").
		WithScope("BTC/USD").
		WithPayload([]byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":50000,"bid":49999,"ask":50001,"volume":1200,"change_pct":0.01}]}`,
		))
}

func bookArtifact(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("book").
		WithScope("BTC/USD").
		WithPayload([]byte(
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":49990,"qty":2}],"asks":[{"price":50010,"qty":1}]}]}`,
		))
}

func tradeArtifact(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	return datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("trade").
		WithScope("BTC/USD").
		WithPayload([]byte(
			`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":50000,"qty":0.5,"side":"buy","timestamp":"2026-06-18T00:00:00Z"},{"symbol":"BTC/USD","price":50001,"qty":0.4,"side":"sell","timestamp":"2026-06-18T00:00:01Z"}]}`,
		))
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
			So(len(crypto.dashboardSignalNames()), ShouldEqual, 13)
		})
	})
}
