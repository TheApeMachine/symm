package sentiment

import (
	"context"
	"iter"
	"math"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func lastMeasurement(
	measurements []*types.Measurement, symbol string, metric types.MetricType,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given a sentiment signal fed by a market replay", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(),
			},
		}
		handlers := tests.Handlers{
			"ticker": signal.ticker.On,
		}
		market := tests.NewMarket().
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

		Convey("When calm and pumped ticker timelines are measured", func() {
			calm := measureField(signal, handlers, market.Frames(), types.MetricChange)
			pumped := measureField(
				signal,
				handlers,
				tests.Spike(market.Frames(), 16, 1.25, 1),
				types.MetricChange,
			)

			Convey("Then the pumped stream should amplify measured change", func() {
				So(len(tickerRows(signal.ticker.cache)), ShouldEqual, 0)
				So(math.Abs(pumped), ShouldBeGreaterThan, math.Abs(calm))
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given current ticker rows with one clear cohort leader", testingTB, func() {
		now := time.Now()
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{cache: tickerCache(
				kraken.TickerData{
					Symbol:    "BTC/USD",
					ChangePct: 5,
					Last:      krakendecimal.NewFromFloat64(105),
					Timestamp: now,
				},
				kraken.TickerData{
					Symbol:    "ETH/USD",
					ChangePct: 2,
					Last:      krakendecimal.NewFromFloat64(102),
					Timestamp: now,
				},
				kraken.TickerData{
					Symbol:    "SOL/USD",
					ChangePct: -1,
					Last:      krakendecimal.NewFromFloat64(99),
					Timestamp: now,
				},
			)},
		}

		thesis := types.NewThesis(nil)

		result := signal.Measure(thesis)

		Convey("It should publish breadth and leader scores without categories", func() {
			breadth, ok := lastMeasurement(result.Measurements, "BTC/USD", types.MetricBreadth)
			So(ok, ShouldBeTrue)
			So(breadth.Raw, ShouldAlmostEqual, 2.0/3.0, 0.0001)

			surge, ok := lastMeasurement(result.Measurements, "BTC/USD", types.MetricSurgeScore)
			So(ok, ShouldBeTrue)
			So(surge.Raw, ShouldBeGreaterThan, 0)

			strength, ok := lastMeasurement(result.Measurements, "BTC/USD", types.MetricStrength)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldBeLessThanOrEqualTo, 1)

			So(len(tickerRows(signal.ticker.cache)), ShouldEqual, 0)
		})
	})
}

func measureField(
	signal *Signal,
	handlers tests.Handlers,
	frames iter.Seq[tests.Frame],
	metric types.MetricType,
) float64 {
	signal.ticker.cache = tickerCache()
	tests.Replay(handlers, frames)

	thesis := types.NewThesis(nil)
	result := signal.Measure(thesis)

	measurement, ok := lastMeasurement(result.Measurements, "ALGO/USD", metric)

	if !ok {
		return 0
	}

	return measurement.Raw
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	now := time.Now()
	signal := &Signal{
		ctx: context.Background(),
		ticker: &Ticker{
			cache: tickerCache(
				kraken.TickerData{
					Symbol:    "BTC/USD",
					ChangePct: 5,
					Last:      krakendecimal.NewFromFloat64(105),
					Timestamp: now,
				},
				kraken.TickerData{
					Symbol:    "ETH/USD",
					ChangePct: 2,
					Last:      krakendecimal.NewFromFloat64(102),
					Timestamp: now,
				},
			),
		},
	}
	thesis := types.NewThesis(nil)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.ticker.cache = tickerCache(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				ChangePct: 5,
				Last:      krakendecimal.NewFromFloat64(105),
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				ChangePct: 2,
				Last:      krakendecimal.NewFromFloat64(102),
				Timestamp: now,
			},
		)
		_ = signal.Measure(thesis)
	}
}
