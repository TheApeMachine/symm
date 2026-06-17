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
