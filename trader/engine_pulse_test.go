package trader

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoEmitEnginePulse(t *testing.T) {
	Convey("Given crypto with active predictions and perspectives", t, func() {
		crypto := newEnginePulseTestCrypto(t)
		now := time.Now()
		crypto.predictions = append(
			crypto.predictions,
			enginePulseTestPrediction("BTC/EUR", 0.02, now.Add(time.Minute)),
			enginePulseTestPrediction("ETH/EUR", 0.04, now.Add(time.Minute)),
			enginePulseTestPrediction("DOGE/EUR", 1, now.Add(-time.Second)),
		)
		crypto.perspectives[bucketKey{
			symbol: "BTC/EUR",
			ptype:  engine.PerspectiveMicrostructure,
		}] = NewPerspective([]engine.Measurement{
			enginePulseTestMeasurement("BTC/EUR"),
		})
		subscriber := crypto.broadcasts["ui"].Subscribe("engine-pulse-test", 8)

		crypto.emitEnginePulse("scan")

		Convey("It should publish aggregate chart data", func() {
			select {
			case value := <-subscriber.Incoming:
				pulse, ok := value.Value.(map[string]any)
				So(ok, ShouldBeTrue)
				requirement := crypto.entryReturnRequirement(
					"BTC/EUR",
					enginePulseTestMeasurement("BTC/EUR"),
				)
				expectedMultiple := 0.03 / requirement.requiredReturn

				So(pulse["event"], ShouldEqual, "engine_pulse")
				So(pulse["phase"], ShouldEqual, "scan")
				So(pulse["measurements"], ShouldEqual, 1)
				So(pulse["candidates"], ShouldEqual, 1)
				So(pulse["forecast_symbols"], ShouldEqual, 2)
				So(pulse["scaled_forecast_symbols"], ShouldEqual, 2)
				So(pulse["avg_prediction"], ShouldAlmostEqual, 0.03)
				So(pulse["avg_prediction_multiple"], ShouldAlmostEqual, expectedMultiple)
				So(pulse["avg_required_return"], ShouldAlmostEqual, requirement.requiredReturn)
				So(pulse["avg_error"], ShouldAlmostEqual, 0)
				So(pulse["avg_error_multiple"], ShouldAlmostEqual, 0)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for engine pulse")
			}
		})
	})
}

func BenchmarkCryptoEnginePulse(b *testing.B) {
	crypto := newEnginePulseBenchCrypto(b)
	now := time.Now()

	for index := 0; index < 128; index++ {
		symbol := fmt.Sprintf("TEST%d/EUR", index)

		crypto.predictions = append(
			crypto.predictions,
			enginePulseTestPrediction(symbol, float64(index)/10000, now.Add(time.Minute)),
		)
	}

	for index := 0; index < 64; index++ {
		symbol := fmt.Sprintf("TEST%d/EUR", index)
		crypto.perspectives[bucketKey{
			symbol: symbol,
			ptype:  engine.PerspectiveMicrostructure,
		}] = NewPerspective([]engine.Measurement{
			enginePulseTestMeasurement(symbol),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = crypto.enginePulse("scan")
	}
}

func newEnginePulseTestCrypto(t *testing.T) *Crypto {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	t.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	t.Cleanup(func() { _ = crypto.Close() })

	return crypto
}

func newEnginePulseBenchCrypto(b *testing.B) *Crypto {
	b.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	forecasts := price.NewPrediction(ctx, pool)
	b.Cleanup(func() { _ = forecasts.Close() })

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	crypto := NewCrypto(ctx, pool, tradingWallet, forecasts)
	b.Cleanup(func() { _ = crypto.Close() })

	return crypto
}

func enginePulseTestMeasurement(symbol string) engine.Measurement {
	return engine.Measurement{
		Type:       engine.Momentum,
		Source:     "hawkes",
		Regime:     "cluster",
		Reason:     "burst",
		Pairs:      []asset.Pair{{Wsname: symbol}},
		Confidence: 0.9,
		Last:       100,
		Bid:        99.9,
		Ask:        100.1,
	}
}

func enginePulseTestPrediction(
	symbol string,
	expectedReturn float64,
	dueAt time.Time,
) *engine.Prediction {
	return &engine.Prediction{
		Perspective: engine.Perspective{
			Type: engine.PerspectiveMicrostructure,
			Measurements: []engine.Measurement{
				enginePulseTestMeasurement(symbol),
			},
		},
		ExpectedReturn: expectedReturn,
		DueAt:          dueAt,
	}
}
