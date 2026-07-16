package liquidity

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func liquidityRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       krakendecimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       krakendecimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      krakendecimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

func TestSignal_MeasureRequiresTwoExecutablePeers(testingTB *testing.T) {
	Convey("Given a cross-section with only one observed symbol", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing rather than dividing by an unsupported median", func() {
			So(result.Measurements, ShouldBeEmpty)
		})
	})
}

func TestSignal_MeasureUsesExecutableValueNotRawVolume(testingTB *testing.T) {
	Convey("Given two penny-priced peers with huge raw volume but tiny quote notional", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
					liquidityRow("PENNY1/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
					liquidityRow("PENNY2/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then current executable depth, not raw units, determines scarcity", func() {
			relative, ok := measureField(result.Measurements, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldBeGreaterThan, 1)
			So(relative.Subject, ShouldEqual, types.SubjectPeerLiquidity)
			So(relative.Maturity, ShouldBeGreaterThan, 0)

			strength, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureAtPeerMedianIsBalanced(testingTB *testing.T) {
	Convey("Given a subject whose notional and depth match its peers exactly", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					liquidityRow("BTC/USD", 99, 101, 5, 5, 100, 100),
					liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
					liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then relative touch depth is one and scarcity is zero", func() {
			relative, ok := measureField(result.Measurements, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 1, 1e-9)

			strength, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureDoesNotMixTurnoverIntoTouchDepth(testingTB *testing.T) {
	Convey("Given high reported turnover but below-median executable touch depth", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 1_000_000, 100),
					liquidityRow("ETH/USD", 99, 101, 1, 1, 100, 100),
					liquidityRow("SOL/USD", 99, 101, 1, 1, 100, 100),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then turnover cannot inflate the current depth ratio", func() {
			relative, ok := measureField(result.Measurements, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			strength, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestSignal_MeasureDoesNotRequireReportedTurnover(testingTB *testing.T) {
	Convey("Given executable peers without reported turnover", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 0, 0),
					liquidityRow("ETH/USD", 99, 101, 1, 1, 0, 0),
					liquidityRow("SOL/USD", 99, 101, 1, 1, 0, 0),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then touch scarcity remains measurable without invented turnover", func() {
			strength, ok := measureField(result.Measurements, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			_, hasNotional := measureField(result.Measurements, "BTC/USD", types.MetricReportedVolumeNotional)
			So(hasNotional, ShouldBeFalse)
		})
	})
}

func TestSignal_MeasureSkipsNonExecutableSubject(testingTB *testing.T) {
	Convey("Given a subject with a one-sided quote among executable peers", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			ticker: &Ticker{
				cache: tickerCache(
					kraken.TickerData{
						Symbol:    "BTC/USD",
						Bid:       krakendecimal.NewFromFloat64(0),
						BidQty:    0,
						Ask:       krakendecimal.NewFromFloat64(101),
						AskQty:    5,
						Last:      krakendecimal.NewFromFloat64(101),
						Volume:    100,
						Vwap:      100,
						Timestamp: time.Now(),
					},
					liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
					liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
				),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing for the unexecutable subject", func() {
			_, hasSubject := measureField(result.Measurements, "BTC/USD", types.MetricRelativeTouchDepth)
			So(hasSubject, ShouldBeFalse)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx: context.Background(),
		ticker: &Ticker{
			cache: tickerCache(
				liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
				liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
				liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
			),
		},
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.ticker.cache = tickerCache(
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		)
		_ = signal.Measure(types.NewThesis(nil))
	}
}
